package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxDatasetFile is the largest .xlsx the Lab reads; maxDatasetUpload caps the request around it.
const (
	maxDatasetFile   = 10 << 20
	maxDatasetUpload = maxDatasetFile + 64<<10
)

// errStorageFull is pendingReserve's, keepWorkbook's and confirmWork's answer when a Dataset would take the Learner over their Storage
// allowance.
var errStorageFull = errors.New("over the Storage allowance")

// sizeText writes a number of bytes as people read it: 12 KB, 980 MB, 1.5 GB.
func sizeText(b int64) string {
	one := func(x float64) string { return strconv.FormatFloat(math.Round(x*10)/10, 'f', -1, 64) }
	switch {
	case b >= 1<<30:
		return one(float64(b)/(1<<30)) + " GB"
	case b >= 1<<20:
		return one(float64(b)/(1<<20)) + " MB"
	}
	return strconv.FormatInt((b+1023)>>10, 10) + " KB"
}

// dashboard is one of a Learner's Dashboards. Its Dataset is its current Parts (ADR 0022), which stay in the store until
// a handler needs their rows: Parts is how many there are, 0 until the first confirm; Columns, Rows, Key and File
// describe them.
type dashboard struct {
	ID       int64
	Name     string
	Theme    string       // the key of the Theme it draws in
	Config   *lab.Config  // a built Dashboard's blocks and filter bar; nil until its first confirm, and for a page one
	Settings lab.Settings // what it remembers for its next upload
	Columns  []lab.Column // the Dataset's columns
	Parts    int          // how many current Parts the Dataset has
	Rows     int          // their rows in all
	Key      string       // names the Parts the Dataset is made of, so it changes whenever they do; empty without Parts
	File     string       // the last Part's file name
	Updated  string       // formatted as periodTime
	Page     *htmlPage    // its current page, the HTML its Learner made (ADR 0018); nil for a built Dashboard
}

// dashboardRow is a Dashboard in the Lab list.
type dashboardRow struct {
	ID         int64
	Name       string
	Theme      string // a built Dashboard's Theme, by name
	Updated    string
	Page       bool        // it has a page: its Learner's own, not one Cave builds
	Deployment *deployment // nil until deployed
}

// labDashboards lists a Learner's Dashboards, most recently changed first.
// ponytail: one query per deployed Dashboard; join the Deployment's columns in if a Learner's list gets long.
func (a *App) labDashboards(r *http.Request, learnerID int64) ([]dashboardRow, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT d.id, d.name, d.theme, d.updated_at,
			EXISTS (SELECT 1 FROM dashboard_pages pg WHERE pg.dashboard_id = d.id AND pg.current),
			EXISTS (SELECT 1 FROM deployments dep WHERE dep.dashboard_id = d.id)
		FROM dashboards d WHERE d.learner_id = $1 ORDER BY d.updated_at DESC, d.id DESC`, learnerID)
	if err != nil {
		return nil, err
	}
	var list []dashboardRow
	var deployed []int // indexes into list
	for rows.Next() {
		var d dashboardRow
		var theme string
		var updated time.Time
		var dep bool
		if err := rows.Scan(&d.ID, &d.Name, &theme, &updated, &d.Page, &dep); err != nil {
			rows.Close()
			return nil, err
		}
		if dep {
			deployed = append(deployed, len(list))
		}
		d.Theme, d.Updated = theme, updated.UTC().Format(periodTime)
		if t, ok := lab.ThemeByKey(theme); ok {
			d.Theme = t.Name
		}
		list = append(list, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, i := range deployed {
		if list[i].Deployment, err = a.deploymentOf(r, list[i].ID); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// ownDashboard loads the Dashboard in the URL if it belongs to the logged-in Learner; nil otherwise.
func (a *App) ownDashboard(r *http.Request) (*dashboard, error) {
	learner, err := a.currentLearner(r)
	if err != nil || learner == nil {
		return nil, err
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, nil
	}
	return a.loadDashboard(r.Context(), "id = $1 AND learner_id = $2", id, learner.ID)
}

// loadDashboard loads the Dashboard matching where (an SQL condition on dashboards); nil if there is none.
// Its Parts stay in the store.
func (a *App) loadDashboard(ctx context.Context, where string, args ...any) (*dashboard, error) {
	var d dashboard
	var config, settings, columns []byte
	var updated time.Time
	var pageKey sql.NullString
	var pageSize sql.NullInt64
	var pageUploaded sql.NullTime
	err := a.db.QueryRowContext(ctx, `SELECT d.id, d.name, d.theme, d.config, d.upload_settings, d.dataset_columns,
			ds.parts, ds.rows, ds.file, d.updated_at, pg.storage_key, pg.size, pg.uploaded_at
		FROM (SELECT * FROM dashboards WHERE `+where+`) d
		CROSS JOIN LATERAL (SELECT count(*) AS parts, COALESCE(sum(p.row_count), 0) AS rows,
				COALESCE((array_agg(p.file_name ORDER BY p.position DESC))[1], '') AS file
			FROM dataset_parts p WHERE p.dashboard_id = d.id AND p.state = 'current') ds
		LEFT JOIN dashboard_pages pg ON pg.dashboard_id = d.id AND pg.current`, args...).
		Scan(&d.ID, &d.Name, &d.Theme, &config, &settings, &columns, &d.Parts, &d.Rows, &d.File, &updated,
			&pageKey, &pageSize, &pageUploaded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, d.Key, err = a.dataset.Keys(ctx, d.ID); err != nil {
		return nil, err
	}
	d.Updated = updated.UTC().Format(periodTime)
	if pageKey.Valid {
		d.Page = &htmlPage{Key: pageKey.String, Size: pageSize.Int64, Uploaded: pageUploaded.Time.UTC().Format(periodTime)}
	}
	if err := json.Unmarshal(settings, &d.Settings); err != nil {
		return nil, err
	}
	if config != nil {
		if err := json.Unmarshal(config, &d.Config); err != nil {
			return nil, err
		}
	}
	if columns != nil {
		if err := json.Unmarshal(columns, &d.Columns); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// withDashboard runs h with the logged-in Learner's Dashboard from the URL, or answers 404.
func (a *App) withDashboard(h func(http.ResponseWriter, *http.Request, *dashboard)) http.HandlerFunc {
	return a.requireLearner(func(w http.ResponseWriter, r *http.Request) {
		d, err := a.ownDashboard(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if d == nil {
			a.notFound(w, r)
			return
		}
		h(w, r, d)
	})
}

// learnerAccess applies the Lab access rule to the logged-in Learner.
func (a *App) learnerAccess(r *http.Request) (labAccess, error) {
	learner, err := a.currentLearner(r)
	if err != nil {
		return labAccess{}, err
	}
	return a.labAccessOf(r.Context(), learner)
}

// labOpen applies the Lab access rule to a request that builds or changes something,
// answering 403 when the Learner may not. ok is false when it has answered.
func (a *App) labOpen(w http.ResponseWriter, r *http.Request) (acc labAccess, ok bool) {
	acc, err := a.learnerAccess(r)
	if err != nil {
		a.serverError(w, r, err)
		return acc, false
	}
	if !acc.Open() {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return acc, false
		}
		a.render(w, r, http.StatusForbidden, "message", page{Title: text.T(lang, "lab.locked.title"), Message: text.T(lang, "lab.locked")})
	}
	return acc, acc.Open()
}

// newDashboardPage is /lab/new: the two ways to a Dashboard (ADR 0019), Build from Excel and Paste your own page.
type newDashboardPage struct {
	pageBase
	Name       string
	BuildError string // why Build from Excel was refused; Error is why the pasted page was
	HTML       string // the pasted page, kept when the form is refused
}

// labScreen opens a Lab screen, a GET under /lab, only to those the Lab is open to. Visitors, Learners without a
// Plan and lapsed Plan holders go to /pricing, even for their own Dashboards (ADR 0018).
func (a *App) labScreen(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		var acc labAccess
		if learner != nil {
			if acc, err = a.labAccessOf(r.Context(), learner); err != nil {
				a.serverError(w, r, err)
				return
			}
		}
		if !acc.Open() {
			http.Redirect(w, r, "/pricing", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

// newDashboardForm is GET /lab/new.
func (a *App) newDashboardForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "lab_new", &newDashboardPage{})
}

// createDashboard makes a Dashboard from a name and the page its Learner made (createPageDashboard).
func (a *App) createDashboard(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	a.createPageDashboard(w, r, acc, parsePageForm(w, r))
}

// buildDashboard makes a Dashboard by Build from Excel (ADR 0019), POST /lab/dashboards/build: Clean-up reads the
// Learner's .xlsx and keeps it as the new Dashboard's pending Dataset, whose preview the Dashboard opens on; the
// confirm builds its blocks by the General rules. A name or file refused, or a file whose Workbook does not fit the
// Storage allowance, saves nothing; once the Workbook is kept (ADR 0021) it stays if Clean-up fails or the Dataset
// does not fit, but no Dashboard is made.
func (a *App) buildDashboard(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	file, upload, problem := xlsxOf(w, r, lang)
	p := &newDashboardPage{Name: strings.TrimSpace(r.FormValue("name"))}
	refuse := func(msg string) {
		p.BuildError = msg
		a.render(w, r, http.StatusUnprocessableEntity, "lab_new", p)
	}
	if problem != "" {
		refuse(problem)
		return
	}
	defer file.Close()
	if msg := dashboardNameError(lang, p.Name); msg != "" {
		refuse(msg)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The Workbook is kept before Clean-up reads it, so it stays whatever Clean-up finds (ADR 0021).
	workbook, used, err := a.keepWorkbook(r, learner.ID, 0, acc, uploadMode{}, upload, data)
	switch {
	case errors.Is(err, errStorageFull):
		refuse(text.T(lang, "lab.err.storage-full", sizeText(int64(len(data))), sizeText(used), sizeText(acc.Period.Storage())))
		return
	case errors.Is(err, errDatasetStore):
		refuse(text.T(lang, "lab.d.upload-failed"))
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	res, err := a.clean(r, bytes.NewReader(data), lab.Settings{})
	var unread lab.Problem
	if errors.As(err, &unread) {
		refuse(text.T(lang, "lab.d.not-read", upload) + ". " + unread.In(lang))
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := &dashboard{}
	if err := a.db.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, kind, name)
		VALUES ($1, 'work', $2) RETURNING id`, learner.ID, p.Name).Scan(&d.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	var size int64
	_, err = a.dataset.Upload(r.Context(), d.ID, dataset.Uploaded{Kind: dataset.ReplaceData, File: upload, Sum: sha256.Sum256(data), Workbook: workbook, Read: res},
		pendingReserve(d, acc, uploadMode{}, &size, &used))
	// ponytail: a compensating delete, not one transaction, as Upload runs its own; a crash between the insert and
	// here leaves an empty built Dashboard in the Lab list, which its Learner can delete. Pass a tx into Upload
	// if that turns up.
	if err != nil { // the Dashboard goes with its Dataset, which Upload has dropped
		if _, err := a.db.ExecContext(context.WithoutCancel(r.Context()), `DELETE FROM dashboards WHERE id = $1`, d.ID); err != nil {
			a.logError(r, fmt.Errorf("delete refused Dashboard %d: %w", d.ID, err))
		}
	}
	switch {
	case errors.Is(err, errStorageFull):
		refuse(text.T(lang, "lab.err.storage-full", sizeText(size), sizeText(used), sizeText(acc.Period.Storage())))
		return
	case errors.Is(err, errDatasetStore):
		if !errors.Is(err, errDatasetStore) { // the Workbook's refusal is logged where it happened
			a.logError(r, err)
		}
		refuse(text.T(lang, "lab.d.upload-failed"))
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}

func dashboardNameError(lang, name string) string {
	switch {
	case name == "":
		return text.T(lang, "lab.err.name")
	case len(name) > 80:
		return text.T(lang, "lab.err.name-long")
	}
	return ""
}

// renameDashboard changes a Dashboard's name; the rename tab is its form.
func (a *App) renameDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if msg := dashboardNameError(lang, name); msg != "" {
		p := &dashboardPage{Dashboard: d, Tab: "rename", Access: acc}
		p.Error = msg
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET name = $2, updated_at = now() WHERE id = $1`, d.ID, name); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/lab", http.StatusSeeOther)
}

// saveTheme sets the Theme the Dashboard draws in, and its Deployment with it, then shows the preview at the
// width it was picked at; a Theme Cave does not offer is refused.
func (a *App) saveTheme(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	theme := r.FormValue("theme")
	if _, ok := lab.ThemeByKey(theme); !ok {
		http.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET theme = $2, updated_at = now() WHERE id = $1`, d.ID, theme); err != nil {
		a.serverError(w, r, err)
		return
	}
	to := dashboardURL(d) + "/preview"
	if r.FormValue("phone") != "" {
		to += "?phone"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// confirmDeleteDashboard asks before deleting a Dashboard.
func (a *App) confirmDeleteDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "lab.d.delete", Question: "lab.d.delete-ask", Cancel: "/lab", Action: r.URL.Path}, d.Name)
	}
}

// deleteDashboard deletes a Dashboard and its Dataset, from the store too, once its confirmation page is posted. Its
// Deployment's link then answers "not found"; the Deployment is never awake again, so it frees a slot, and its name
// stays taken.
func (a *App) deleteDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok || unconfirmed(w, r) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM dashboards WHERE id = $1`, d.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r) // its Datasets, queued as their records went
	http.Redirect(w, r, "/lab", http.StatusSeeOther)
}

// dashboardPage is the Dashboard page: a built Dashboard's tabs Dataset, Preview and Deploy, a page Dashboard's Page,
// Data and Deploy, plus Rename.
type dashboardPage struct {
	pageBase
	Dashboard *dashboard
	Tab       string
	Problem   string          // why an upload could not be read
	Upload    string          // the refused file's name
	Same      *samePart       // the current Part the refused file is the exact same file as
	Failed    bool            // the store refused the upload's Dataset
	Pending   *pendingDataset // an upload waiting for its confirm
	PartList  []partRow       // the Dataset's current Parts in order, on the Data tab
	Workbooks []workbook      // the Learner's Workbooks, to pick from, on the Data tab
	Replacing *partRef        // the Part whose Replace screen this is, on the Data tab
	Uses      []columnUse     // after the first confirm: the columns the blocks use, as the pending Dataset has them
	AskHeader int             // most of Uses are missing: the preview asks whether this row is the header
	Mapping   *columnMapping  // an Add refused for lacking columns: the form to map them from the file's others
	Gone      string          // a page Dashboard's replacing upload: the current columns it lacks
	New       string          // and its columns the current Dataset lacks
	Pages     []htmlPage      // the earlier pages, newest first
	Phone     bool            // preview at phone width
	Themes    []lab.Theme     // Preview tab: the Themes to pick from

	Access labAccess // the caller looks it up

	// Deploy tab
	Deployment *deployment // nil until deployed
	Confirm    bool        // the confirm step
	DeployName string      // the name typed on the confirm step
	Prefix     string      // a Deployment's link up to its name
}

func (a *App) dashboardTab(tab string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		acc, err := a.learnerAccess(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		at := tab // not tab itself, which every request shares
		if at == "" {
			at = homeTab(d)
		}
		a.renderDashboard(w, r, http.StatusOK, &dashboardPage{Dashboard: d, Tab: at, Access: acc, Phone: r.URL.Query().Has("phone")})
	})
}

// renderDashboard writes the Dashboard page p with status, first filling in what its tab shows: on the Data tab the
// pending Part, the current Parts, the Learner's Workbooks and any storage warning, on the Page tab the earlier pages.
func (a *App) renderDashboard(w http.ResponseWriter, r *http.Request, status int, p *dashboardPage) {
	d := p.Dashboard
	p.Title, p.Themes = d.Name, lab.Themes
	if p.Tab == dataTab(d) && p.Pending == nil && p.Replacing == nil && p.Mapping == nil {
		var err error
		if p.Pending, err = a.pendingOf(r.Context(), d); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Pending != nil && d.Config != nil { // later confirms keep the blocks, with a pick for each column gone
		p.Uses = columnUses(d.Config, p.Pending.Dataset)
		if 2*missingUses(p.Uses) > len(p.Uses) {
			p.AskHeader = p.Pending.HeaderRow
		}
	}
	if p.Pending != nil && !p.Pending.Mode.intoDataset() && d.Page != nil && d.Parts > 0 {
		// By exact name, as data.json's keys are: a Replace serves the new names as they are, so a page reading Jumlah
		// breaks on jumlah. An Add matches columns ignoring case and spaces (lab.ColumnChanges) as it keeps the
		// current names, so data.json's keys stay.
		lacking := func(cols, others []lab.Column) string {
			var out []string
			for _, c := range cols {
				if !slices.ContainsFunc(others, func(o lab.Column) bool { return o.Name == c.Name }) {
					out = append(out, c.Name)
				}
			}
			return strings.Join(out, ", ")
		}
		p.Gone, p.New = lacking(d.Columns, p.Pending.Dataset.Columns), lacking(p.Pending.Dataset.Columns, d.Columns)
	}
	if p.Tab == dataTab(d) {
		var err error
		if p.PartList, err = a.partsOf(r.Context(), d.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		// Every Workbook is offered: whether it fits is checked when it is picked, as reading each one here would not be bounded.
		learner, err := a.currentLearner(r)
		if err == nil {
			p.Workbooks, err = a.workbooksOf(r.Context(), learner.ID)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if p.Pending != nil {
			var room storageRoom
			room, err = roomFor(r.Context(), a.db, d.ID, p.Pending.Size, p.Pending.Mode, p.Access)
			if err == nil && !room.Fits && p.Error == "" { // the confirm will be refused: say so now
				var lang string
				lang, err = a.language(r)
				p.Error = text.T(lang, "lab.err.storage-full", sizeText(p.Pending.Size), sizeText(room.Used), sizeText(p.Access.Period.Storage()))
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Tab == "page" { // a page Dashboard's Page screen lists its earlier pages
		var err error
		if p.Pages, err = a.earlierPages(r.Context(), d.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.render(w, r, status, "lab_dashboard", p)
}

// uploadDataset checks an uploaded .xlsx's name and size, then hands it to uploadWork to clean and keep pending.
func (a *App) uploadDataset(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if acc, ok := a.labOpen(w, r); ok {
		a.uploadXlsx(w, r, d, acc, nil)
	}
}

// uploadXlsx is what uploadDataset and uploadPartReplacement share, once the Lab access rule gave acc: it reads the
// upload, or the Workbook picked in its place, and hands it to uploadWork. The upload replaces the Part replacing, if
// not nil.
func (a *App) uploadXlsx(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, replacing *partRef) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if pick := pickedID(r); pick != "" {
		// A Workbook from the Folder takes the way of an upload; keepWorkbook finds it by its bytes, so no new Workbook.
		if data, name, ok := a.pickedWorkbook(w, r, pick); ok {
			a.uploadWork(w, r, d, acc, bytes.NewReader(data), name, replacing)
		}
		return
	}
	file, name, problem := xlsxOf(w, r, lang)
	if problem != "" {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Problem: problem, Upload: name})
		return
	}
	defer file.Close()
	a.uploadWork(w, r, d, acc, file, name, replacing)
}

// pickedID is the ID of the Workbook the Data tab's pick form names, or "" for an upload. The pick form is a plain
// form; an upload is multipart, so its file is never read into memory here.
func pickedID(r *http.Request) string {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return ""
	}
	return r.PostFormValue("workbook")
}

// pickedWorkbook reads the Workbook with ID id from the store with its name, when it is the logged-in Learner's own;
// anyone else's, or one that is not there, gets the Not found page and ok is false.
func (a *App) pickedWorkbook(w http.ResponseWriter, r *http.Request, id string) (data []byte, name string, ok bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return nil, "", false
	}
	n, err := strconv.ParseInt(id, 10, 64)
	var key string
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT name, storage_key FROM workbooks WHERE id = $1 AND learner_id = $2`, n, learner.ID).Scan(&name, &key)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
		a.notFound(w, r)
		return nil, "", false
	}
	if err == nil {
		data, err = a.datasets.Get(r.Context(), key)
	}
	if err != nil {
		a.serverError(w, r, fmt.Errorf("get Workbook %d: %w", n, err))
		return nil, "", false
	}
	return data, name, true
}

// xlsxOf opens the .xlsx a Learner gave in the form's file field, the request capped at maxDatasetUpload. problem
// says why it is refused, in lang, or is "" when it is not; name is the file's name, when one was given.
func xlsxOf(w http.ResponseWriter, r *http.Request, lang string) (file multipart.File, name, problem string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDatasetUpload)
	file, header, err := r.FormFile("file")
	var over *http.MaxBytesError
	switch {
	case errors.As(err, &over) || r.ContentLength > maxDatasetUpload:
		return nil, "", text.T(lang, "lab.err.too-big")
	case err != nil:
		return nil, "", text.T(lang, "lab.err.choose-file")
	}
	name = header.Filename
	switch {
	case strings.EqualFold(filepath.Ext(name), ".xls"):
		problem = text.T(lang, "lab.err.xls")
	case !strings.EqualFold(filepath.Ext(name), ".xlsx"):
		problem = text.T(lang, "lab.err.not-xlsx", name)
	case header.Size > maxDatasetFile:
		problem = text.T(lang, "lab.err.too-big")
	default:
		return file, name, ""
	}
	file.Close()
	return nil, name, problem
}

// newDatasetKey is a new key in the Dataset store: a new one every upload, so a cached copy is never stale.
func newDatasetKey() (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	return "dataset-" + token[:22], nil
}

// dropDataset deletes the Dataset stored under key, if any, even after the request that asked is over. A failure
// leaves only an unused object, so it is logged; r is nil outside a request.
func (a *App) dropDataset(ctx context.Context, r *http.Request, key string) {
	if key == "" {
		return
	}
	if err := a.datasets.Delete(context.WithoutCancel(ctx), key); err != nil {
		a.logError(r, fmt.Errorf("delete Dataset %s: %w", key, err))
	}
}

// datasetKeepDays is how long a Learner's Datasets outlive their last Plan period.
const datasetKeepDays = 90

// pendingKeepHours is how long a Dashboard's unconfirmed Dataset waits for its Learner before the prune.
const pendingKeepHours = 24

// lapsedSQL is an SQL condition, true for a Learner aliased l with no Plan period active, upcoming, or ended within
// $1 days. A Learner who never had a Plan period counts as long lapsed; the Admin never does.
const lapsedSQL = `NOT l.is_admin AND NOT EXISTS (SELECT 1 FROM plan_periods pp
	WHERE pp.learner_id = l.id AND LEAST(pp.ends_at, COALESCE(pp.ended_early_at, pp.ends_at)) > now() - make_interval(days => $1))`

// pruneDatasets deletes the Parts of every Learner with no Plan period active, upcoming, or ended within
// datasetKeepDays, with their Workbooks, from the store and the records, and returns how many Parts it deleted. The
// Dashboards and their Deployments stay, the Deployments asleep. The Admin's Parts are never pruned. Parts left
// pending past pendingKeepHours go too, the Admin's included.
func (a *App) pruneDatasets(ctx context.Context) (int, error) {
	work, err := a.db.ExecContext(ctx, `DELETE FROM dataset_parts ds USING dashboards d JOIN learners l ON l.id = d.learner_id
		WHERE d.id = ds.dashboard_id AND (ds.state = 'pending' AND ds.uploaded_at < now() - make_interval(hours => $2) OR `+lapsedSQL+`)`,
		datasetKeepDays, pendingKeepHours)
	if err != nil {
		return 0, err
	}
	deleted, err := work.RowsAffected()
	if err != nil {
		return 0, err
	}
	// Their Workbooks go on the same day (ADR 0021); the trigger queues the files.
	if _, err = a.db.ExecContext(ctx, `DELETE FROM workbooks wb USING learners l WHERE l.id = wb.learner_id AND `+lapsedSQL,
		datasetKeepDays); err != nil {
		return 0, err
	}
	a.dropDeletedFiles(ctx, nil)
	return int(deleted), nil
}

// copyDatasetsToStore is a one-off step at startup: it moves every Dataset still in the old dataset column into
// the store. Once 00069 drops the column there is nothing left to copy. The JSON's size stands in for the
// uploaded file's, which was never kept.
func copyDatasetsToStore(ctx context.Context, db *sql.DB, store storage.Store) error {
	var old bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'dashboards' AND column_name = 'dataset')`).Scan(&old); err != nil || !old {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, dataset FROM dashboards WHERE dataset IS NOT NULL AND dataset_key IS NULL`)
	if err != nil {
		return err
	}
	type row struct {
		id   int64
		data []byte
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.data); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range todo {
		key, err := newDatasetKey()
		if err != nil {
			return err
		}
		if err := store.Put(ctx, key, r.data); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `UPDATE dashboards SET dataset_key = $2, dataset_size = $3, dataset = NULL WHERE id = $1`,
			r.id, key, len(r.data)); err != nil {
			return err
		}
	}
	return nil
}

// convertDatasetsToParts is a one-off step at startup (ADR 0022): it rewrites the Datasets from before Parts that are
// current or pending, each stored as one whole table, into the Part shape (lab.PartRows) under a new key, with the
// Part date migration 00128 gave it, and gives their Dashboards the Dataset's columns and a recipe (the sheet the Part
// was read from, its header row found, not typed). Their sizes stay as saved. A pending Add held the whole combined
// table; its Part is the new rows alone. The earlier Datasets are left as they are. With nothing left to convert it
// reads one row.
func convertDatasetsToParts(ctx context.Context, db *sql.DB, store storage.Store) error {
	rows, err := db.QueryContext(ctx, `SELECT id, dashboard_id, storage_key, state, part_date::text, cleanup, upload_kind FROM dataset_parts
		WHERE legacy AND state <> 'earlier' ORDER BY state = 'pending', id`)
	if err != nil {
		return err
	}
	type old struct {
		id, dashboard          int64
		key, state, date, kind string
		cleanup                []byte
	}
	var todo []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.dashboard, &o.key, &o.state, &o.date, &o.cleanup, &o.kind); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, o := range todo {
		data, err := store.Get(ctx, o.key)
		if err != nil {
			return fmt.Errorf("get Dataset %s: %w", o.key, err)
		}
		var d lab.Dataset
		var c cleaned
		if err := json.Unmarshal(o.cleanup, &c); err != nil {
			return fmt.Errorf("read Dataset %s: %w", o.key, err)
		}
		// A whole table is rewritten as a Part. What is not one is a Part already (rows only, as after migrating down and
		// up again), and keeps its object.
		key, rows, rewrite := o.key, -1, json.Unmarshal(data, &d) == nil && len(d.Columns) > 0
		if rewrite {
			// Only a pending Add holds rows it has not added yet; a confirmed one's record still names its Add.
			if o.state == "pending" && o.kind == string(dataset.Add) && c.Added != nil && c.Added.Had <= len(d.Rows) {
				d.Rows = d.Rows[c.Added.Had:]
			}
			c.Columns, rows = d.Columns, len(d.Rows)
			part, err := lab.PartRows(&d, o.date)
			if err != nil {
				return err
			}
			if key, err = newDatasetKey(); err != nil {
				return err
			}
			if err := store.Put(ctx, key, part); err != nil {
				return err
			}
		}
		cleanup, err := json.Marshal(c)
		if err != nil {
			return err
		}
		columns, err := json.Marshal(c.Columns)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE dataset_parts SET storage_key = $2, row_count = CASE WHEN $3 < 0 THEN row_count ELSE $3 END,
			cleanup = $4, legacy = false WHERE id = $1`, o.id, key, rows, cleanup)
		if err == nil && o.state == "current" {
			_, err = tx.ExecContext(ctx, `UPDATE dashboards SET dataset_columns = $2 WHERE id = $1`, o.dashboard, columns)
		}
		if err == nil && o.state == "current" && c.PickedSheet != "" {
			// The Dataset has no recipe until its next Replace data, so every later Part would be read as a first one: its
			// recipe is the sheet this Part was picked from, its header row found, not typed.
			recipe, _ := json.Marshal(lab.Recipe{Sheet: c.PickedSheet, HeaderRow: c.HeaderRow})
			_, err = tx.ExecContext(ctx, `UPDATE dashboards SET upload_settings = jsonb_set(upload_settings, '{recipe}', $2::jsonb) WHERE id = $1 AND NOT upload_settings ? 'recipe'`,
				o.dashboard, recipe)
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			if rewrite {
				_ = store.Delete(ctx, key)
			}
			return err
		}
		if rewrite {
			_ = store.Delete(ctx, o.key) // ponytail: a failed delete leaves one unused object; nothing lists it
		}
	}
	return nil
}

func dashboardURL(d *dashboard) string { return "/lab/dashboards/" + strconv.FormatInt(d.ID, 10) }

// dashboardFile serves the generated Dashboard to its Learner for the preview: index.html, charts.js and
// the data at data/<key> under it, so the page loads the others by relative path.
// A Deployment serves the same files under /<prefix>/<name>/ (deploymentFile).
func (a *App) dashboardFile(name string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		a.serveDashboardFile(w, r, d, name, previewCSP, "private, max-age=31536000, immutable")
	})
}

// previewCSP keeps a Dashboard served same-origin (the preview) to Cave's own scripts and
// connections, to the fonts written into charts.js, and to framing by Cave. The preview needs the session
// cookie for data.json, so no sandbox.
const previewCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; font-src data:; frame-ancestors 'self'"

// dataPath is where a Dashboard's page fetches its data, relative to the page: the Dataset's key, so any change to its
// Parts changes the address and no cache ever serves an old Dataset. The preview's data is cached for good
// (only its Learner sees it); a Deployment's for 5 minutes, so sleep and Takedown take effect soon.
func dataPath(d *dashboard) string { return "data/" + d.Key }

// serveDashboardFile writes one of a built Dashboard's files: index.html, charts.js or data (data/<key>, under
// cache; any other key is not found). index.html needs only the Dataset's columns, which the Dashboard holds, so the
// store is read for data alone.
// csp is the index page's Content-Security-Policy; cache is the data's Cache-Control.
func (a *App) serveDashboardFile(w http.ResponseWriter, r *http.Request, d *dashboard, name, csp, cache string) {
	if name == "charts.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(lab.ChartScript)
		return
	}
	if d.Parts == 0 || name == "data" && r.PathValue("key") != d.Key {
		a.notFound(w, r)
		return
	}
	if name == "data" {
		a.serveParts(w, r, d, r.PathValue("key"), cache)
		return
	}
	index, err := lab.Page(d.Name, d.Theme, d.Config, d.Columns, dataPath(d))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

// serveParts writes the Dashboard's Parts as one JSON array, Part after Part in position order, each as stored, so
// no Part is parsed. The Parts are listed in one query. With a key (the preview's data/<key>) they must be the Parts it
// names, as a Part deleted or confirmed since d was loaded would make the listing another Dataset's, which the browser
// would then keep for a year under the key; any other listing is not found. The first Part is read before the headers
// go, so a store that fails answers 500.
// ponytail: one Part is in memory at a time, as the store reads whole objects; stream from the store (R2) if Parts
// get big enough to matter.
func (a *App) serveParts(w http.ResponseWriter, r *http.Request, d *dashboard, key, cache string) {
	parts, datasetKey, err := a.dataset.Keys(r.Context(), d.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if key != "" && datasetKey != key {
		a.notFound(w, r)
		return
	}
	var withRows []string
	for _, p := range parts {
		if p.HasRows {
			withRows = append(withRows, p.Key)
		}
	}
	var first []byte
	if len(withRows) > 0 {
		if first, err = a.datasets.Get(r.Context(), withRows[0]); err != nil {
			a.serverError(w, r, fmt.Errorf("get Part %s: %w", withRows[0], err))
			return
		}
	}
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[\n"))
	_, _ = w.Write(first)
	for _, k := range withRows[min(1, len(withRows)):] {
		data, err := a.datasets.Get(r.Context(), k)
		if err != nil {
			a.logError(r, fmt.Errorf("get Part %s: %w", k, err))
			panic(http.ErrAbortHandler) // the rows already sent are not the Dataset: cut the connection
		}
		_, _ = w.Write([]byte(",\n"))
		_, _ = w.Write(data)
	}
	_, _ = w.Write([]byte("\n]\n"))
}
