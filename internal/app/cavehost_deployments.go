package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// A CaveHost Deployment (spec #536) is a row of deployments with no Dashboard, so it shares the Lab's names, Link keys and
// Username redirect (ADR 0023, ADR 0027), plus its cavehost_deployments row for the Spreadsheet, its cavehost_sheets rows and
// its cavehost_pages. Its data and page live in the local Disk store (ADR 0025), under keys that start with caveHostKeyPrefix.
// ponytail: this ticket links one Spreadsheet's first sheet and puts a page online, replaced and restored as on the Lab; ticking more
// sheets, Sync, the cap, sleep and delete are the later tickets of #536, and a deleted record leaves its stored files until then.
const caveHostKeyPrefix = "cavehost-"

// caveHostAwakeSQL is an SQL condition, true for an awake CaveHost Deployment aliased dep: it is not taken down, and its Learner
// is the Admin or has an active CaveHost Plan period.
const caveHostAwakeSQL = `(dep.taken_down_at IS NULL AND EXISTS (SELECT 1 FROM learners owner WHERE owner.id = dep.learner_id AND
	(owner.is_admin OR EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = owner.id AND ` + activePeriodSQL + `))))`

// newCaveHostKey is a new key in the Disk store for a file of kind (xlsx, rows or page) of a CaveHost Deployment.
func newCaveHostKey(kind string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	return caveHostKeyPrefix + kind + "-" + token[:22], nil
}

// caveHostDeployments lists the Learner's CaveHost Deployments, oldest first, with their times in lang.
func (a *App) caveHostDeployments(ctx context.Context, lang string, learnerID int64) ([]deployment, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+caveHostColumns+` FROM deployments dep
		JOIN cavehost_deployments ch ON ch.deployment_id = dep.id WHERE dep.learner_id = $1 ORDER BY dep.id`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []deployment
	for rows.Next() {
		dep, err := a.scanDeployment(lang, rows)
		if err != nil {
			return nil, err
		}
		list = append(list, dep)
	}
	return list, rows.Err()
}

// caveHostHome is /cavehost: the Learner's CaveHost Deployments and the form to link a Spreadsheet.
func (a *App) caveHostHome(w http.ResponseWriter, r *http.Request) {
	a.renderCaveHostHome(w, r, http.StatusOK, &caveHostPage{})
}

// renderCaveHostHome renders p, the form's values and error if it was refused, with the Learner's Deployments.
func (a *App) renderCaveHostHome(w http.ResponseWriter, r *http.Request, status int, p *caveHostPage) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Deployments, err = a.caveHostDeployments(r.Context(), lang, learner.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Prefix = a.pagesBaseURL + "/" + learner.Username + "/"
	a.render(w, r, status, "cavehost", p)
}

// firstSheetOf is the name of the first sheet of the xlsx file data.
func (a *App) firstSheetOf(r *http.Request, data []byte) (string, error) {
	release, err := a.waitForCleanUp(r.Context()) // opening a workbook takes as much memory as reading it
	if err != nil {
		return "", err
	}
	defer release()
	names, err := lab.SheetNames(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return names[0], nil
}

// createCaveHostDeployment links a Spreadsheet: it takes the pasted link and the name picked, and with the Learner's word that
// they may show its data to anyone with the Deployment's link, fetches the Spreadsheet and keeps it with its first sheet. The
// Deployment takes its name only once all that works; the Learner then checks how Clean-up reads the sheet, on the Deployment's
// page, and confirms it. Anything refused saves nothing.
func (a *App) createCaveHostDeployment(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	link, name := strings.TrimSpace(r.PostFormValue("link")), strings.TrimSpace(r.PostFormValue("name"))
	refuse := func(status int, msg string) {
		a.renderCaveHostHome(w, r, status, &caveHostPage{pageBase: pageBase{Error: msg}, Link: link, Name: name})
	}
	spreadsheet, gid, ok := spreadsheetOf(link)
	if !ok {
		refuse(http.StatusUnprocessableEntity, text.T(lang, "cavehost.err.link"))
		return
	}
	if key := nameError(name); key != "" {
		refuse(http.StatusUnprocessableEntity, text.T(lang, key))
		return
	}
	if r.PostFormValue("right") != "yes" {
		refuse(http.StatusUnprocessableEntity, text.T(lang, "lab.dep.err.right"))
		return
	}
	// ponytail: two links sent at once could both pass and make a 4th; fine at one Learner's pace.
	var have int
	if !learner.IsAdmin {
		if err := a.db.QueryRowContext(r.Context(), `SELECT count(*) FROM deployments dep JOIN cavehost_deployments ch
			ON ch.deployment_id = dep.id WHERE dep.learner_id = $1`, learner.ID).Scan(&have); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if have >= maxCaveHostDeployments {
		refuse(http.StatusUnprocessableEntity, text.T(lang, "cavehost.err.cap", maxCaveHostDeployments))
		return
	}
	var taken bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM deployments WHERE name = $1)`, name).Scan(&taken); err != nil {
		a.serverError(w, r, err)
		return
	}
	if taken {
		refuse(http.StatusUnprocessableEntity, text.T(lang, "lab.dep.err.name-taken"))
		return
	}

	data, err := a.fetchSpreadsheet(r.Context(), spreadsheet)
	switch {
	case errors.Is(err, errSpreadsheetClosed):
		refuse(http.StatusUnprocessableEntity, text.T(lang, "cavehost.err.closed"))
		return
	case errors.Is(err, errSpreadsheetTooBig):
		refuse(http.StatusUnprocessableEntity, text.T(lang, "cavehost.err.too-big"))
		return
	case err != nil:
		a.logError(r, err)
		refuse(http.StatusBadGateway, text.T(lang, "cavehost.err.unreachable"))
		return
	}
	sheet, err := a.firstSheetOf(r, data)
	var problem lab.Problem
	if errors.As(err, &problem) {
		refuse(http.StatusUnprocessableEntity, problem.In(lang))
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	key, err := newCaveHostKey("xlsx")
	if err == nil {
		err = a.store.Put(r.Context(), key, data)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		var id int64
		// The row outlives the Learner's use of it, so the unique name is never given to another Deployment.
		if err := tx.QueryRowContext(r.Context(), `INSERT INTO deployments (learner_id, name, link_key) VALUES ($1, $2, $3) RETURNING id`,
			learner.ID, name, newLinkKey()).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO cavehost_deployments (deployment_id, spreadsheet_id, gid, workbook_key)
			VALUES ($1, $2, $3, $4)`, id, spreadsheet, gid, key); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO cavehost_sheets (deployment_id, name, position) VALUES ($1, $2, 0)`, id, sheet)
		return err
	})
	if err != nil {
		_ = a.store.Delete(context.WithoutCancel(r.Context()), key)
	}
	if isUniqueViolation(err) { // another Deployment took the name since it was checked
		refuse(http.StatusUnprocessableEntity, text.T(lang, "lab.dep.err.name-taken"))
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost/"+name, http.StatusSeeOther)
}

// caveHostView is a CaveHost Deployment of the Learner asking, with its first sheet.
type caveHostView struct {
	deployment
	ID          int64
	Spreadsheet string
	WorkbookKey string // the fetched xlsx, until the sheet is confirmed
	SheetID     int64
	Sheet       string
	Confirmed   bool // the sheet's first Clean-up is confirmed
	Columns     []lab.Column
	HasPage     bool
	Pages       []htmlPage   // the earlier pages, newest first
	Sync        caveHostSync // the last Sync
}

// caveHostViewOf loads the CaveHost Deployment called {name} that belongs to the Learner asking; when there is none it writes the
// Not found page and returns false.
func (a *App) caveHostViewOf(w http.ResponseWriter, r *http.Request) (*caveHostView, bool) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	v := &caveHostView{}
	var columns, reason, changes []byte
	var syncedAt sql.NullTime
	var status sql.NullString
	dep, err := a.scanDeployment(lang, a.db.QueryRowContext(r.Context(), `SELECT dep.id, ch.spreadsheet_id, COALESCE(ch.workbook_key, ''),
			sh.id, sh.name, sh.rows_key IS NOT NULL, COALESCE(sh.columns, '[]'::jsonb),
			EXISTS (SELECT 1 FROM cavehost_pages pg WHERE pg.deployment_id = dep.id AND pg.current),
			ch.synced_at, ch.sync_status, ch.sync_reason, ch.sync_changes, `+caveHostColumns+`
		FROM deployments dep
		JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		JOIN cavehost_sheets sh ON sh.deployment_id = dep.id AND sh.position = 0
		WHERE dep.name = $1 AND dep.learner_id = $2`, r.PathValue("name"), learner.ID),
		&v.ID, &v.Spreadsheet, &v.WorkbookKey, &v.SheetID, &v.Sheet, &v.Confirmed, &columns, &v.HasPage, &syncedAt, &status, &reason, &changes)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return nil, false
	}
	if err == nil {
		err = json.Unmarshal(columns, &v.Columns)
	}
	if err == nil {
		v.Sync, err = caveHostSyncOf(lang, syncedAt, status, reason, changes)
	}
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	v.deployment = dep
	if v.HasPage {
		if v.Pages, err = a.earlierPages(r.Context(), caveHostPages, v.ID); err != nil {
			a.serverError(w, r, err)
			return nil, false
		}
	}
	return v, true
}

// caveHostDeploymentPage is a CaveHost Deployment's page: the preview of its first sheet's Clean-up to confirm, then its link
// and the page's form. Reading again with a header row or two-row header is a GET with the choices in the query.
type caveHostDeploymentPage struct {
	pageBase
	*caveHostView
	Preview *lab.CleanUp // the first sheet's Clean-up, until it is confirmed
	Rows    [][]string   // the first of its cleaned rows
	Header  int          // the header row the Learner typed; 0 lets Cave find it
	TwoRow  bool
	HTML    string // the pasted page, when it was refused
}

// ColumnList is the sheet's columns, as the preview reads them or as they were confirmed.
func (p *caveHostDeploymentPage) ColumnList() string {
	if p.Preview != nil {
		return columnList(p.Preview.Dataset.Columns)
	}
	return columnList(p.Columns)
}

// caveHostSettings is how Clean-up reads the first sheet of v: alone, with the header choices on the form.
func caveHostSettings(r *http.Request, v *caveHostView) lab.Settings {
	header, _ := strconv.Atoi(r.FormValue("header"))
	return lab.Settings{Sheet: v.Sheet, Alone: true, HeaderRow: max(header, 0), TwoRowHeader: r.FormValue("two-row") == "on"}
}

// caveHostPreview reads the first sheet of v from the stored Spreadsheet, as settings say.
func (a *App) caveHostPreview(r *http.Request, v *caveHostView, settings lab.Settings) (*lab.CleanUp, error) {
	data, err := a.store.Get(r.Context(), v.WorkbookKey)
	if err != nil {
		return nil, err
	}
	res, err := a.clean(r, bytes.NewReader(data), settings)
	if err == nil && res.PickedSheet != v.Sheet { // Clean-up could not read the sheet and read another instead
		return nil, lab.Problem{Key: "cavehost.err.sheet", Args: []any{v.Sheet}}
	}
	return res, err
}

// renderCaveHostDeployment renders the page of v with msg, if any, as its error. An unconfirmed sheet shows its preview, read
// with the header choices on the request; a Clean-up Problem is the error too.
func (a *App) renderCaveHostDeployment(w http.ResponseWriter, r *http.Request, status int, v *caveHostView, msg string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := caveHostSettings(r, v)
	p := &caveHostDeploymentPage{pageBase: pageBase{Error: msg}, caveHostView: v, Header: settings.HeaderRow, TwoRow: settings.TwoRowHeader}
	if r.Method == http.MethodPost {
		p.HTML = r.PostFormValue("html")
	}
	if !v.Confirmed {
		res, err := a.caveHostPreview(r, v, settings)
		var problem lab.Problem
		switch {
		case errors.As(err, &problem):
			p.Error = problem.In(lang)
			if r.Method == http.MethodPost {
				status = http.StatusUnprocessableEntity
			}
		case err != nil:
			a.serverError(w, r, err)
			return
		default:
			p.Preview, p.Rows = res, previewCells(res.Dataset)
		}
	}
	a.render(w, r, status, "cavehost_deployment", p)
}

func (a *App) caveHostDeploymentPage(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.caveHostViewOf(w, r); ok {
		a.renderCaveHostDeployment(w, r, http.StatusOK, v, "")
	}
}

// confirmCaveHostSheet makes the first sheet's Clean-up, read as the form's header choices say, the sheet's columns and Recipe,
// and keeps its rows as the last good ones. Later Syncs read the sheet with that Recipe.
func (a *App) confirmCaveHostSheet(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	if v.Confirmed {
		http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := caveHostSettings(r, v)
	res, err := a.caveHostPreview(r, v, settings)
	var problem lab.Problem
	if errors.As(err, &problem) {
		a.renderCaveHostDeployment(w, r, http.StatusUnprocessableEntity, v, problem.In(lang))
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	day := res.PartDate
	if day == "" { // the sheet names no date: the day of this read
		day = a.now().In(malaysia).Format(time.DateOnly)
	}
	rows, err := lab.PartRows(res.Dataset, day)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings.Recipe = &lab.Recipe{Sheet: res.PickedSheet, HeaderRow: res.HeaderRow, HeaderTyped: settings.HeaderRow > 0,
		MonthColumns: append([]string{}, res.MonthColumns...)}
	saved, err := json.Marshal(settings)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	columns, err := json.Marshal(res.Dataset.Columns)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	key, err := newCaveHostKey("rows")
	if err == nil {
		err = a.store.Put(r.Context(), key, rows)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	confirmed := false
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Two confirms at once: the first sets the rows, the second finds them set and changes nothing.
		set, err := tx.ExecContext(r.Context(), `UPDATE cavehost_sheets SET columns = $2, settings = $3, rows_key = $4 WHERE id = $1 AND rows_key IS NULL`,
			v.SheetID, columns, saved, key)
		if err != nil {
			return err
		}
		if n, err := set.RowsAffected(); err != nil || n == 0 {
			return err
		}
		confirmed = true
		_, err = tx.ExecContext(r.Context(), `UPDATE cavehost_deployments SET workbook_key = NULL WHERE deployment_id = $1`, v.ID)
		return err
	})
	if err != nil || !confirmed {
		_ = a.store.Delete(context.WithoutCancel(r.Context()), key)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if confirmed { // the fetched file is not kept once its rows are
		_ = a.store.Delete(context.WithoutCancel(r.Context()), v.WorkbookKey)
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// uploadCaveHostPage puts the Learner's page online at the Deployment's link: the picked .html file, else the pasted HTML, as on
// the Lab's Deploy tab, once the sheet is confirmed. A page already online turns earlier, and past keepEarlier the oldest earlier
// one goes, from the store too.
func (a *App) uploadCaveHostPage(w http.ResponseWriter, r *http.Request) {
	tooBig := parsePageForm(w, r)
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	if !v.Confirmed {
		http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	html, problem, err := pageOf(r, lang, tooBig)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if problem != "" {
		a.renderCaveHostDeployment(w, r, http.StatusUnprocessableEntity, v, problem)
		return
	}
	key, err := newCaveHostKey("page")
	if err == nil {
		err = a.store.Put(r.Context(), key, html)
	}
	if err != nil {
		a.logError(r, err)
		a.renderCaveHostDeployment(w, r, http.StatusServiceUnavailable, v, text.T(lang, "lab.d.upload-failed"))
		return
	}
	var dropped []string
	err = a.inTx(r.Context(), func(tx *sql.Tx) (err error) {
		if dropped, err = caveHostPages.replaceCurrent(r.Context(), tx, v.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cavehost_pages (deployment_id, storage_key, size, current) VALUES ($1, $2, $3, true)`, v.ID, key, len(html))
		return err
	})
	if err != nil {
		_ = a.store.Delete(context.WithoutCancel(r.Context()), key)
	}
	if err != nil && !isUniqueViolation(err) { // a unique violation is another upload getting in first
		a.serverError(w, r, err)
		return
	}
	for _, old := range dropped { // the Disk store has no queue of its own, as the Lab's has
		if err := a.store.Delete(context.WithoutCancel(r.Context()), old); err != nil {
			a.logError(r, err)
		}
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// restoreCaveHostPage makes an earlier page live again at once, POST /cavehost/{name}/pages/{pg}/restore; the live one turns
// earlier. It asks nothing first: the page it replaces is kept.
func (a *App) restoreCaveHostPage(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("pg"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error { return caveHostPages.restore(r.Context(), tx, v.ID, id) })
	if errors.Is(err, errNotEarlier) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// serveCaveHost serves the file of a CaveHost Deployment named name, with this Link key, at its link on the Deployments host: the
// page at the link and the first ticked sheet's rows at data.json. served is false when no CaveHost Deployment has that name
// and key. An asleep or taken down Deployment shows the inactive page, as a Lab one does.
func (a *App) serveCaveHost(w http.ResponseWriter, r *http.Request, name, linkKey, file string) (served bool, err error) {
	var id int64
	var awake, takenDown bool
	err = a.db.QueryRowContext(r.Context(), `SELECT dep.id, `+caveHostAwakeSQL+`, dep.taken_down_at IS NOT NULL
		FROM deployments dep JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		WHERE dep.name = $1 AND COALESCE(dep.link_key, '') = $2`, name, linkKey).Scan(&id, &awake, &takenDown)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch {
	case takenDown:
		a.pagesMessage(w, r, http.StatusOK, "lab.dep.sleeping.title", "lab.dep.sleeping-taken")
		return true, nil
	case !awake:
		a.pagesMessage(w, r, http.StatusOK, "cavehost.sleeping.title", "cavehost.sleeping")
		return true, nil
	}
	var key string
	switch file {
	case "index.html":
		err = a.db.QueryRowContext(r.Context(), `SELECT storage_key FROM cavehost_pages WHERE deployment_id = $1 AND current`, id).Scan(&key)
	case "data.json": // the first ticked sheet that has rows
		err = a.db.QueryRowContext(r.Context(), `SELECT rows_key FROM cavehost_sheets
			WHERE deployment_id = $1 AND ticked AND rows_key IS NOT NULL ORDER BY position LIMIT 1`, id).Scan(&key)
	default:
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return true, nil
	}
	if err != nil {
		return true, err
	}
	data, err := a.store.Get(r.Context(), key)
	if err != nil {
		return true, err
	}
	if file == "index.html" {
		a.writePage(w, data)
		return true, nil
	}
	// The same rows as a Lab Dataset's data.json, in the same shape. Any origin may read them, as a sandboxed page's origin is
	// null; five minutes' cache, so sleep and Takedown take effect soon.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[\n"))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n]\n"))
	return true, nil
}
