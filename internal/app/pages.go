package app

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxPageSize is the largest page a Dashboard takes (ADR 0017); maxPageUpload caps the request around it.
const (
	maxPageSize   = 5 << 20
	maxPageUpload = maxPageSize + 64<<10
)

// htmlPage is a Dashboard's page: the HTML its Learner made, kept in the store (ADR 0018).
type htmlPage struct {
	Key      string // where the page is in the store
	Size     int64  // bytes, against the Storage allowance
	Uploaded string // formatted as periodTime
}

func (p *htmlPage) SizeText() string { return sizeText(p.Size) }

// pageCDNs are the only places a Deployment's page may load scripts, styles and fonts from (ADR 0017).
const pageCDNs = "https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://unpkg.com"

// pageCSP sandboxes a Deployment's own page (ADR 0017): inline code and the CDNs, images from anywhere over https,
// and nothing sent out but reads from Cave, by its origin as deploymentCSP explains; no form posts anywhere.
func (a *App) pageCSP() string {
	return "sandbox allow-scripts; default-src 'none'; " +
		"script-src 'unsafe-inline' " + pageCDNs + "; " +
		"style-src 'unsafe-inline' " + pageCDNs + " https://fonts.googleapis.com; " +
		"font-src data: " + pageCDNs + " https://fonts.gstatic.com; " +
		"img-src data: https:; connect-src 'self' " + a.origin() + "; form-action 'none'; frame-ancestors 'none'"
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

// pageOf reads the page a Learner gave on the New Dashboard form: the picked .html file, else the pasted HTML.
// problem is the catalog key saying why it is refused, with its value, or "" when it is not.
func pageOf(r *http.Request, tooBig bool) (html []byte, problem string, arg string, err error) {
	if tooBig {
		return nil, "lab.page.err.too-big", "", nil
	}
	file, header, err := r.FormFile("file")
	switch {
	case err == nil:
		defer file.Close()
		if !strings.EqualFold(filepath.Ext(header.Filename), ".html") {
			return nil, "lab.page.err.not-html", header.Filename, nil
		}
		if header.Size > maxPageSize {
			return nil, "lab.page.err.too-big", "", nil
		}
		if html, err = io.ReadAll(file); err != nil {
			return nil, "", "", err
		}
	case errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart):
		html = []byte(r.PostFormValue("html"))
	default:
		return nil, "", "", err
	}
	switch {
	case strings.TrimSpace(string(html)) == "":
		return nil, "lab.page.err.empty", "", nil
	case len(html) > maxPageSize:
		return nil, "lab.page.err.too-big", "", nil
	}
	return html, "", "", nil
}

// createPageDashboard makes a Dashboard from a name and its page, if the page fits the Learner's Storage allowance.
// Anything refused saves nothing.
func (a *App) createPageDashboard(w http.ResponseWriter, r *http.Request, acc labAccess, tooBig bool) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := newDashboard(strings.TrimSpace(r.FormValue("name")))
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
	html, problem, arg, err := pageOf(r, tooBig)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if problem != "" {
		if arg != "" {
			refuse(text.T(lang, problem, arg))
		} else {
			refuse(text.T(lang, problem))
		}
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	key, err := newPageKey()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.datasets.Put(r.Context(), key, html); err != nil {
		a.logError(r, fmt.Errorf("put page %s: %w", key, err))
		refuse(text.T(lang, "lab.d.upload-failed"))
		return
	}
	var id, used int64
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if period := acc.CountedPeriod(); period != nil {
			// Locking the Learner makes this take turns with uploads and confirms, so none slips under the allowance.
			if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learner.ID); err != nil {
				return err
			}
			if err := tx.QueryRowContext(r.Context(), `SELECT `+storageUsedSQL("$1"), learner.ID).Scan(&used); err != nil {
				return err
			}
			if used+int64(len(html)) > period.Storage() {
				return errStorageFull
			}
		}
		// template is the engine's; General, as it reads none of a page Dashboard's, until the engine goes (#376).
		if err := tx.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, kind, template, name) VALUES ($1, 'work', 'general', $2)
			RETURNING id`, learner.ID, p.Name).Scan(&id); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO dashboard_pages (dashboard_id, storage_key, size, current) VALUES ($1, $2, $3, true)`,
			id, key, len(html))
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		if errors.Is(err, errStorageFull) {
			refuse(text.T(lang, "lab.err.storage-full", sizeText(int64(len(html))), sizeText(used), sizeText(acc.Period.Storage())))
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, dashboardURL(&dashboard{ID: id}), http.StatusSeeOther)
}
