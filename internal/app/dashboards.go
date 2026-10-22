package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxDatasetFile is the largest .xlsx the Lab reads; maxDatasetUpload caps the request around it.
const (
	maxDatasetFile   = 2 << 20
	maxDatasetUpload = maxDatasetFile + 64<<10
)

// dashboard is one of a Learner's Dashboards. Dataset is nil until the first upload.
type dashboard struct {
	ID       int64
	Template *lab.Template
	Name     string
	Choices  lab.Choices
	Dataset  *lab.Dataset
	Key      string // where the Dataset is in the Dataset store; empty until the first upload
	File     string // the uploaded Dataset's file name
	Updated  string // formatted as periodTime
}

// dashboardRow is a Dashboard in the Lab list.
type dashboardRow struct {
	ID        int64
	Name      string
	Template  string
	Updated   string
	HasData   bool   // a Dataset is uploaded, so it can be downloaded
	TakenDown string // when Cave took its Deployment down; empty if it was not
}

// labDashboards lists a Learner's Dashboards, most recently changed first.
func (a *App) labDashboards(r *http.Request, learnerID int64) ([]dashboardRow, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT d.id, d.name, d.template, d.updated_at, d.dataset_key IS NOT NULL, dep.taken_down_at
		FROM dashboards d LEFT JOIN deployments dep ON dep.dashboard_id = d.id
		WHERE d.learner_id = $1 ORDER BY d.updated_at DESC, d.id DESC`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []dashboardRow
	for rows.Next() {
		var d dashboardRow
		var key string
		var updated time.Time
		var takenDown sql.NullTime
		if err := rows.Scan(&d.ID, &d.Name, &key, &updated, &d.HasData, &takenDown); err != nil {
			return nil, err
		}
		if takenDown.Valid {
			d.TakenDown = takenDown.Time.UTC().Format(periodTime)
		}
		d.Template, d.Updated = templateName(key), updated.UTC().Format(periodTime)
		list = append(list, d)
	}
	return list, rows.Err()
}

func templateName(key string) string {
	if t := lab.TemplateByKey(key); t != nil {
		return t.Name
	}
	return key
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
func (a *App) loadDashboard(ctx context.Context, where string, args ...any) (*dashboard, error) {
	var d dashboard
	var key string
	var choices []byte
	var updated time.Time
	err := a.db.QueryRowContext(ctx, `SELECT id, template, name, choices, COALESCE(dataset_key, ''), dataset_file, updated_at
		FROM dashboards WHERE `+where, args...).
		Scan(&d.ID, &key, &d.Name, &choices, &d.Key, &d.File, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.Template = lab.TemplateByKey(key); d.Template == nil {
		return nil, fmt.Errorf("dashboard %d has unknown template %q", d.ID, key)
	}
	d.Updated = updated.UTC().Format(periodTime)
	if err := json.Unmarshal(choices, &d.Choices); err != nil {
		return nil, err
	}
	if d.Key != "" {
		// ponytail: reads the whole Dataset on every Dashboard page; load it only where needed if the store gets slow.
		data, err := a.datasets.Get(d.Key)
		if err != nil {
			return nil, fmt.Errorf("get Dataset %s: %w", d.Key, err)
		}
		if err := json.Unmarshal(data, &d.Dataset); err != nil {
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

// newDashboardPage is /lab/new: pick a Template, see its blanks, download its Sample, name the Dashboard.
type newDashboardPage struct {
	pageBase
	Templates []*lab.Template
	Picked    string
	Name      string
	Sample    *lab.Dataset // the picked Template's Sample, as Cave reads it
	Guess     lab.Choices  // Cave's guesses on that Sample
}

// renderNewDashboard shows /lab/new with the picked Template's Sample read, so the page can name its columns.
func (a *App) renderNewDashboard(w http.ResponseWriter, r *http.Request, status int, p *newDashboardPage) {
	t := lab.TemplateByKey(p.Picked)
	var err error
	if p.Sample, err = lab.Read(bytes.NewReader(t.Sample), t); err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Guess = t.Guess(p.Sample)
	a.render(w, r, status, "lab_new", p)
}

func (a *App) newDashboardForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	p := &newDashboardPage{Templates: lab.Templates, Picked: r.URL.Query().Get("t")}
	if lab.TemplateByKey(p.Picked) == nil {
		p.Picked = lab.Templates[0].Key
	}
	a.renderNewDashboard(w, r, http.StatusOK, p)
}

func (a *App) createDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &newDashboardPage{Templates: lab.Templates, Picked: r.FormValue("template"), Name: strings.TrimSpace(r.FormValue("name"))}
	switch {
	case lab.TemplateByKey(p.Picked) == nil:
		p.Error, p.Picked = text.T(lang, "lab.err.template"), lab.Templates[0].Key
	default:
		p.Error = dashboardNameError(lang, p.Name)
	}
	if p.Error != "" {
		a.renderNewDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var id string
	if err := a.db.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, template, name) VALUES ($1, $2, $3)
		RETURNING id`, learner.ID, p.Picked, p.Name).Scan(&id); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/lab/dashboards/"+id, http.StatusSeeOther)
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

// deleteDashboard deletes a Dashboard and its Dataset, from the store too; the delete tab asks first. Its Deployment's link
// then answers "not found", and the Deployment still counts against its period.
func (a *App) deleteDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	var key string
	if err := a.db.QueryRowContext(r.Context(), `DELETE FROM dashboards WHERE id = $1 RETURNING COALESCE(dataset_key, '')`,
		d.ID).Scan(&key); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDataset(r, key)
	http.Redirect(w, r, "/lab", http.StatusSeeOther)
}

// labSample downloads a Template's Sample, /lab/samples/{key}.xlsx. Samples are invented data, open to anyone.
func (a *App) labSample(w http.ResponseWriter, r *http.Request) {
	key, ok := strings.CutSuffix(r.PathValue("file"), ".xlsx")
	t := lab.TemplateByKey(key)
	if !ok || t == nil {
		a.notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+t.Key+`-sample.xlsx"`)
	_, _ = w.Write(t.Sample)
}

// dashboardPage is the Dashboard page: tabs Dataset, Columns, Preview, Under the hood and Deploy,
// plus Rename and Delete.
type dashboardPage struct {
	pageBase
	Dashboard *dashboard
	Tab       string
	Errors    []string // what was wrong with an upload
	Upload    string   // the refused file's name
	Again     bool     // a re-upload did not fit the column choices, so Cave guessed them again
	Blanks    []blankChoice
	Phone     bool // preview at phone width
	ReadOnly  bool // no Lab access: view and download only
	Hood      []hoodPart
	Data      hoodPart // the start of data.json, on the Under the hood tab

	Access labAccess // the caller looks it up; read-only follows from it

	// Deploy tab
	Deployment *deployment // nil until deployed
	Confirm    bool        // the confirm step
	DeployName string      // the name typed on the confirm step
	Prefix     string      // a Deployment's link up to its name
}

// blankChoice is one blank on the Columns tab: the columns that may fill it and the current choice.
type blankChoice struct {
	lab.Blank
	Options []string
	Chosen  string
	Guess   string
	Renamed string // the Sample's column name, when Cave found no column by name and guessed by type
}

func (a *App) dashboardTab(tab string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		acc, err := a.learnerAccess(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		q := r.URL.Query()
		a.renderDashboard(w, r, http.StatusOK, &dashboardPage{Dashboard: d, Tab: tab, Access: acc, Phone: q.Has("phone"), Again: q.Has("again")})
	})
}

func (a *App) renderDashboard(w http.ResponseWriter, r *http.Request, status int, p *dashboardPage) {
	d := p.Dashboard
	p.Title, p.ReadOnly = d.Name, !p.Access.Open()
	if p.Tab == "columns" && d.Dataset != nil {
		guess := d.Template.Guess(d.Dataset)
		for _, b := range d.Template.Blanks {
			bc := blankChoice{Blank: b, Chosen: d.Choices[b.Key], Guess: guess[b.Key]}
			if bc.Guess != "" && !b.Named(bc.Guess) {
				bc.Renamed = b.SampleName()
			}
			for _, c := range d.Dataset.Columns {
				if c.Type == b.Type {
					bc.Options = append(bc.Options, c.Name)
				}
			}
			p.Blanks = append(p.Blanks, bc)
		}
	}
	if p.Tab == "hood" && d.Dataset != nil {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		index, data, err := lab.Generate(d.Template, d.Name, d.Choices, d.Dataset, dataPath(d))
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Hood = hoodParts(lang, string(index))
		p.Data = hoodPart{N: len(p.Hood) + 1, Code: dataExcerpt(lang, string(data)), Title: text.T(lang, "lab.hood.data.title"), Note: text.T(lang, "lab.hood.data")}
	}
	a.render(w, r, status, "lab_dashboard", p)
}

// uploadDataset replaces the Dashboard's Dataset with an uploaded .xlsx. The column choices stay when
// they still fit the new Dataset; otherwise Cave guesses again and sends the Learner to choose columns.
func (a *App) uploadDataset(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	name := ""
	refuse := func(errs ...string) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Errors: errs, Upload: name})
	}
	tooBig := text.T(lang, "lab.err.too-big")
	r.Body = http.MaxBytesReader(w, r.Body, maxDatasetUpload)
	file, header, err := r.FormFile("file")
	var over *http.MaxBytesError
	switch {
	case errors.As(err, &over) || r.ContentLength > maxDatasetUpload:
		refuse(tooBig)
		return
	case err != nil:
		refuse(text.T(lang, "lab.err.choose-file"))
		return
	}
	defer file.Close()
	name = header.Filename
	switch {
	case !strings.EqualFold(filepath.Ext(name), ".xlsx"):
		refuse(text.T(lang, "lab.err.not-xlsx", name))
		return
	case header.Size > maxDatasetFile:
		refuse(tooBig)
		return
	}
	ds, err := lab.Read(file, d.Template)
	var problems lab.Errors
	if errors.As(err, &problems) {
		refuse(problems.In(lang)...)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	next := dashboardURL(d) + "/preview"
	choices := d.Choices
	if _, _, err := lab.Generate(d.Template, d.Name, choices, ds, ""); errors.As(err, &problems) {
		refuse(problems.In(lang)...) // blank cells in a column the Learner chose for a required blank
		return
	} else if err != nil {
		choices, next = d.Template.Guess(ds), dashboardURL(d)+"/columns"
		if d.Dataset != nil {
			next += "?again"
		}
	}
	dataset, err := json.Marshal(ds)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	choicesJSON, err := json.Marshal(choices)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	key, err := newToken()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	key = "dataset-" + key[:22] // a new key every upload, so a cached copy is never stale
	if err := a.datasets.Put(key, dataset); err != nil {
		a.serverError(w, r, fmt.Errorf("put Dataset %s: %w", key, err))
		return
	}
	old, err := a.replaceDataset(r.Context(), d.ID, key, name, header.Size, choicesJSON)
	if err != nil {
		a.dropDataset(r, key)
		a.serverError(w, r, err)
		return
	}
	a.dropDataset(r, old)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// replaceDataset points the Dashboard at the Dataset stored under key and returns the key it replaces, if any.
// The row lock makes two uploads take turns, so each sees the key the other wrote and none is left behind.
func (a *App) replaceDataset(ctx context.Context, id int64, key, file string, size int64, choices []byte) (old string, err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(dataset_key, '') FROM dashboards WHERE id = $1 FOR UPDATE`, id).Scan(&old); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboards SET dataset_key = $2, dataset_file = $3, dataset_size = $4, choices = $5,
		updated_at = now() WHERE id = $1`, id, key, file, size, choices); err != nil {
		return "", err
	}
	return old, tx.Commit()
}

// dropDataset deletes the Dataset stored under key, if any. A failure leaves only an unused object, so it is logged.
func (a *App) dropDataset(r *http.Request, key string) {
	if key == "" {
		return
	}
	if err := a.datasets.Delete(key); err != nil {
		a.logError(r, fmt.Errorf("delete Dataset %s: %w", key, err))
	}
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
		key, err := newToken()
		if err != nil {
			return err
		}
		key = "dataset-" + key[:22]
		if err := store.Put(key, r.data); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `UPDATE dashboards SET dataset_key = $2, dataset_size = $3, dataset = NULL WHERE id = $1`,
			r.id, key, len(r.data)); err != nil {
			return err
		}
	}
	return nil
}

// saveColumns stores the Learner's column choice for each blank; an empty value skips an optional blank.
func (a *App) saveColumns(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	if d.Dataset == nil {
		http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
		return
	}
	c := lab.Choices{}
	for _, b := range d.Template.Blanks {
		if v := r.FormValue(b.Key); v != "" {
			c[b.Key] = v
		}
	}
	if _, _, err := lab.Generate(d.Template, d.Name, c, d.Dataset, ""); err != nil {
		lang, langErr := a.language(r)
		if langErr != nil {
			a.serverError(w, r, langErr)
			return
		}
		d.Choices = c
		p := &dashboardPage{Dashboard: d, Tab: "columns", Access: acc}
		var blanks lab.Errors
		var choice lab.Problem
		switch {
		case errors.As(err, &blanks):
			p.Errors = blanks.In(lang)
		case errors.As(err, &choice):
			p.Error = choice.In(lang)
		default:
			a.serverError(w, r, err)
			return
		}
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	choices, err := json.Marshal(c)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET choices = $2, updated_at = now() WHERE id = $1`,
		d.ID, choices); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, dashboardURL(d)+"/preview", http.StatusSeeOther)
}

func dashboardURL(d *dashboard) string { return "/lab/dashboards/" + strconv.FormatInt(d.ID, 10) }

// dashboardFile serves the generated Dashboard to its Learner for the preview: index.html, charts.js and
// the data at data/<key> under it, so the page loads the others by relative path. Generated on each request.
// A Deployment serves the same files under /<prefix>/<name>/ (deploymentFile).
func (a *App) dashboardFile(name string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		a.serveDashboardFile(w, r, d, name, previewCSP, "private, max-age=31536000, immutable")
	})
}

// previewCSP keeps a Dashboard served same-origin (the preview and the demo) to Cave's own scripts and
// connections, and to framing by Cave. The preview needs the session cookie for data.json, so no sandbox.
const previewCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'"

// demoFile serves a Template's demo, /lab/demo/{key}/: the Template drawn from its Sample with Cave's
// guesses, open to anyone. Nothing is stored, so a Visitor sees the Lab without building anything.
func (a *App) demoFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := lab.TemplateByKey(r.PathValue("key"))
		if t == nil {
			a.notFound(w, r)
			return
		}
		ds, err := lab.Read(bytes.NewReader(t.Sample), t)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.serveDashboardFile(w, r, &dashboard{Template: t, Name: t.Name + " Template", Choices: t.Guess(ds), Dataset: ds}, name, previewCSP, "")
	}
}

// dataPath is where a Dashboard's page fetches its data, relative to the page: the Dataset's key, so a
// re-upload changes the address and the data can be cached for good.
func dataPath(d *dashboard) string { return "data/" + d.Key }

// serveDashboardFile writes one of a Dashboard's files: index.html, charts.js, data.json (the demo's data,
// at a fixed address) or data (data/<key>, cached for good; any other key is not found).
// csp is the index page's Content-Security-Policy; cache is the data's Cache-Control.
func (a *App) serveDashboardFile(w http.ResponseWriter, r *http.Request, d *dashboard, name, csp, cache string) {
	if name == "charts.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(lab.ChartScript)
		return
	}
	if d.Dataset == nil {
		a.notFound(w, r)
		return
	}
	if name == "data" && r.PathValue("key") != d.Key {
		a.notFound(w, r)
		return
	}
	dataURL := "data.json"
	if d.Key != "" {
		dataURL = dataPath(d)
	}
	index, data, err := lab.Generate(d.Template, d.Name, d.Choices, d.Dataset, dataURL)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if name == "data" {
		w.Header().Set("Cache-Control", cache)
	}
	if name == "data.json" || name == "data" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

// downloadDashboard is the Dashboard as a zip that opens offline: index.html, data.json, charts.js and
// data.js, the same rows as a script, which the page reads when the browser blocks fetch from a file.
// Open to its Learner with or without Lab access.
func (a *App) downloadDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if d.Dataset == nil {
		a.notFound(w, r)
		return
	}
	index, data, err := lab.Generate(d.Template, d.Name, d.Choices, d.Dataset, "data.json")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		body []byte
	}{
		{"index.html", index},
		{"data.json", data},
		{"data.js", append(append([]byte("var dashboardRows = "), bytes.TrimSuffix(data, []byte("\n"))...), ";\n"...)},
		{"charts.js", lab.ChartScript},
	} {
		fw, err := zw.Create(f.name)
		if err == nil {
			_, err = fw.Write(f.body)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+zipName(d.Name)+`"`)
	_, _ = w.Write(buf.Bytes())
}

// zipName is the Dashboard's name as a safe file name: lower-case letters and digits joined by dashes.
func zipName(name string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = "dashboard"
	}
	return slug + ".zip"
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// hoodPart is a piece of a generated file on the Under the hood tab, with its plain-word note.
type hoodPart struct {
	N                 int
	Code, Title, Note string
}

// hoodNotes explain index.html part by part; each part starts at its marker in lab/dashboard.html.
// key names its note in the text catalog, and key.title its title.
var hoodNotes = []struct{ marker, key string }{
	{"", "lab.hood.head"},
	{"<body>", "lab.hood.layout"},
	{`  <script src="charts.js">`, "lab.hood.library"},
	{"  <script>\n", "lab.hood.choices"},
	{"    fetch(", "lab.hood.fetch"},
}

// hoodParts cuts the generated index.html at the markers; a missing marker merges its part into the one before.
func hoodParts(lang, index string) []hoodPart {
	var parts []hoodPart
	for i, n := range hoodNotes {
		if i > 0 {
			last := &parts[len(parts)-1]
			j := strings.Index(last.Code, n.marker)
			if j <= 0 {
				continue
			}
			index, last.Code = last.Code[j:], last.Code[:j]
		}
		parts = append(parts, hoodPart{N: len(parts) + 1, Code: index, Title: text.T(lang, n.key+".title"), Note: text.T(lang, n.key)})
	}
	return parts
}

// dataExcerpt is the start of data.json: its first three rows, then how many more there are.
func dataExcerpt(lang, data string) string {
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n") // "[", one line per row, "]"
	if len(lines) <= 7 {
		return data
	}
	return strings.Join(lines[:4], "\n") + "\n  ... " + text.T(lang, "lab.d.more-rows", len(lines)-5) + "\n]\n"
}
