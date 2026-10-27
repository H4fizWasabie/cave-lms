package app

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxPageSize is the largest page a Dashboard takes (ADR 0017); maxPageUpload caps the request around it.
const (
	maxPageSize   = 5 << 20
	maxPageUpload = maxPageSize + 64<<10
)

// htmlPage is a Dashboard's page: the HTML its Learner made, kept in the store (ADR 0018).
type htmlPage struct {
	ID       int64
	Key      string // where the page is in the store
	Size     int64  // bytes, against the Storage allowance
	Uploaded string // formatted as periodTime
}

func (p *htmlPage) SizeText() string { return sizeText(p.Size) }

// pageCDNs are the only places a Deployment's page may load scripts, styles and fonts from (ADR 0017).
const pageCDNs = "https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://unpkg.com"

// pageCSP sandboxes a Deployment's own page (ADR 0017): inline code and the CDNs, images from anywhere over https,
// and nothing sent out but reads from Cave, by its origin as deploymentCSP explains; no form posts anywhere, and no
// <base> to point its relative links, such as ./data.json, elsewhere.
func (a *App) pageCSP() string {
	return "sandbox allow-scripts; default-src 'none'; " +
		"script-src 'unsafe-inline' " + pageCDNs + "; " +
		"style-src 'unsafe-inline' " + pageCDNs + " https://fonts.googleapis.com; " +
		"font-src data: " + pageCDNs + " https://fonts.gstatic.com; " +
		"img-src data: https:; connect-src 'self' " + a.origin() + "; form-action 'none'; frame-ancestors 'none'; base-uri 'none'"
}

// servePage writes a Deployment's page under pageCSP, cached five minutes so sleep and Takedown take effect soon.
// ponytail: reads the page from the store on each uncached visit; keep pages in memory, as datasetFilesCache keeps
// data, if the store gets slow.
func (a *App) servePage(w http.ResponseWriter, r *http.Request, p *htmlPage) {
	data, err := a.datasets.Get(r.Context(), p.Key)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("get page %s: %w", p.Key, err))
		return
	}
	w.Header().Set("Content-Security-Policy", a.pageCSP())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(data)
}

// newPageKey is a new key in the store for a page.
func newPageKey() (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	return "page-" + token[:22], nil
}

// parsePageForm parses a form carrying a page, capped at maxPageUpload, and reports whether it was over the cap.
func parsePageForm(w http.ResponseWriter, r *http.Request) (tooBig bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPageUpload)
	// ParseForm first: ParseMultipartForm hides its error on a form that is not multipart, as a pasted page may be.
	err := r.ParseForm()
	if err == nil {
		err = r.ParseMultipartForm(maxPageUpload)
	}
	var over *http.MaxBytesError
	return errors.As(err, &over) || r.ContentLength > maxPageUpload
}

// pageOf reads the page a Learner gave on a page form: the picked .html file, else the pasted HTML.
// problem says why it is refused, in lang, or is "" when it is not.
func pageOf(r *http.Request, lang string, tooBig bool) (html []byte, problem string, err error) {
	if tooBig {
		return nil, text.T(lang, "lab.page.err.too-big"), nil
	}
	file, header, err := r.FormFile("file")
	switch {
	case err == nil:
		defer file.Close()
		if !strings.EqualFold(filepath.Ext(header.Filename), ".html") {
			return nil, text.T(lang, "lab.page.err.not-html", header.Filename), nil
		}
		if header.Size > maxPageSize {
			return nil, text.T(lang, "lab.page.err.too-big"), nil
		}
		if html, err = io.ReadAll(file); err != nil {
			return nil, "", err
		}
	case errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart):
		html = []byte(r.PostFormValue("html"))
	default:
		return nil, "", err
	}
	switch {
	case strings.TrimSpace(string(html)) == "":
		return nil, text.T(lang, "lab.page.err.empty"), nil
	case len(html) > maxPageSize:
		return nil, text.T(lang, "lab.page.err.too-big"), nil
	}
	return html, "", nil
}

// errPageStore is storePage's error when the store refuses the page.
var errPageStore = errors.New("the store refused the page")

// storePage puts html in the store as a new page, then, in one transaction, runs claim, which locks what it must and
// gives the Dashboard the page is for, checks period's Storage allowance unless period is nil, and makes the page that
// Dashboard's current one. Whatever fails leaves the page out of the store; used is the bytes the Learner's files took
// when the allowance refused it with errStorageFull.
func (a *App) storePage(ctx context.Context, r *http.Request, html []byte, period *activePeriod,
	claim func(tx *sql.Tx) (dashboardID int64, err error)) (used int64, err error) {
	key, err := newPageKey()
	if err != nil {
		return 0, err
	}
	if err := a.datasets.Put(ctx, key, html); err != nil {
		a.logError(r, fmt.Errorf("put page %s: %w", key, err))
		return 0, errPageStore
	}
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		id, err := claim(tx)
		if err != nil {
			return err
		}
		if period != nil {
			if err := tx.QueryRowContext(ctx, `SELECT `+storageUsedSQL("this.learner_id")+` FROM dashboards this WHERE this.id = $1`, id).Scan(&used); err != nil {
				return err
			}
			if used+int64(len(html)) > period.Storage() {
				return errStorageFull
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO dashboard_pages (dashboard_id, storage_key, size, current) VALUES ($1, $2, $3, true)`,
			id, key, len(html))
		return err
	})
	if err != nil {
		a.dropDataset(ctx, r, key)
	}
	return used, err
}

// createPageDashboard makes a Dashboard from a name and its page, if the page fits the Learner's Storage allowance.
// Anything refused saves nothing.
func (a *App) createPageDashboard(w http.ResponseWriter, r *http.Request, acc labAccess, tooBig bool) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &newDashboardPage{Name: strings.TrimSpace(r.FormValue("name"))}
	refuse := func(msg string) {
		if !tooBig {
			p.HTML = r.PostFormValue("html")
		}
		p.Error = msg
		a.render(w, r, http.StatusUnprocessableEntity, "lab_new", p)
	}
	if msg := dashboardNameError(lang, p.Name); msg != "" && !tooBig {
		refuse(msg)
		return
	}
	html, problem, err := pageOf(r, lang, tooBig)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if problem != "" {
		refuse(problem)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var id int64
	used, err := a.storePage(r.Context(), r, html, acc.CountedPeriod(), func(tx *sql.Tx) (int64, error) {
		if acc.CountedPeriod() != nil {
			// Locking the Learner makes this take turns with uploads and confirms, so none slips under the allowance.
			if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learner.ID); err != nil {
				return 0, err
			}
		}
		err := tx.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, kind, name) VALUES ($1, 'work', $2) RETURNING id`,
			learner.ID, p.Name).Scan(&id)
		return id, err
	})
	switch {
	case errors.Is(err, errPageStore):
		refuse(text.T(lang, "lab.d.upload-failed"))
		return
	case errors.Is(err, errStorageFull):
		refuse(text.T(lang, "lab.err.storage-full", sizeText(int64(len(html))), sizeText(used), sizeText(acc.Period.Storage())))
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, dashboardURL(&dashboard{ID: id}), http.StatusSeeOther)
}

// earlierPages lists the Dashboard's earlier pages, newest first.
func (a *App) earlierPages(ctx context.Context, dashboardID int64) ([]htmlPage, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, storage_key, size, uploaded_at FROM dashboard_pages
		WHERE dashboard_id = $1 AND NOT current ORDER BY uploaded_at DESC, id DESC`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []htmlPage
	for rows.Next() {
		var p htmlPage
		var uploaded time.Time
		if err := rows.Scan(&p.ID, &p.Key, &p.Size, &uploaded); err != nil {
			return nil, err
		}
		p.Uploaded = uploaded.UTC().Format(periodTime)
		out = append(out, p)
	}
	return out, rows.Err()
}

// uploadPage makes a new page current at a page Dashboard's link, POST /lab/dashboards/{id}/page, checked as
// createPageDashboard checks the first; a built Dashboard, one without a page, takes none (ADR 0019).
// The page it replaces turns earlier; past keepEarlier, the oldest earlier one goes, from the store too, and its bytes
// no longer count against the Storage allowance.
func (a *App) uploadPage(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	tooBig := parsePageForm(w, r)
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if d.Page == nil {
		msg := text.T(lang, "lab.page.err.built", d.Name)
		a.render(w, r, http.StatusUnprocessableEntity, "message", page{Title: d.Name, Message: msg})
		return
	}
	refuse := func(msg string) {
		p := &dashboardPage{Dashboard: d, Tab: homeTab(d), Access: acc}
		p.Error = msg
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
	}
	html, problem, err := pageOf(r, lang, tooBig)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if problem != "" {
		refuse(problem)
		return
	}
	used, err := a.storePage(r.Context(), r, html, acc.CountedPeriod(), func(tx *sql.Tx) (int64, error) {
		// Locking the Learner makes this take turns with other uploads and confirms, so none slips under the allowance.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return 0, err
		}
		// The current page turns earlier, so keepEarlier-1 of the earlier ones stay; their bytes are freed before the
		// allowance is checked.
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dashboard_pages WHERE id IN (SELECT id FROM dashboard_pages
			WHERE dashboard_id = $1 AND NOT current ORDER BY uploaded_at DESC, id DESC OFFSET $2)`, d.ID, keepEarlier-1); err != nil {
			return 0, err
		}
		// Two statements, as the one-current index is checked row by row: this one, then storePage's insert.
		if _, err := tx.ExecContext(r.Context(), `UPDATE dashboard_pages SET current = false WHERE dashboard_id = $1 AND current`, d.ID); err != nil {
			return 0, err
		}
		_, err := tx.ExecContext(r.Context(), `UPDATE dashboards SET updated_at = now() WHERE id = $1`, d.ID)
		return d.ID, err
	})
	switch {
	case errors.Is(err, errPageStore):
		refuse(text.T(lang, "lab.d.upload-failed"))
		return
	case errors.Is(err, errStorageFull):
		refuse(text.T(lang, "lab.err.storage-full", sizeText(int64(len(html))), sizeText(used), sizeText(acc.Period.Storage())))
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r) // the oldest earlier page, queued as its record went
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}

// restorePage makes an earlier page current at once, POST /lab/dashboards/{id}/pages/{pg}/restore; the current one
// turns earlier.
func (a *App) restorePage(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("pg"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	gone := errors.New("not an earlier page")
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `UPDATE dashboard_pages SET current = false WHERE dashboard_id = $1 AND current AND id <> $2`, d.ID, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(r.Context(), `UPDATE dashboard_pages SET current = true WHERE id = $1 AND dashboard_id = $2 AND NOT current`, id, d.ID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return cmp.Or(err, gone)
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET updated_at = now() WHERE id = $1`, d.ID)
		return err
	})
	if errors.Is(err, gone) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}
