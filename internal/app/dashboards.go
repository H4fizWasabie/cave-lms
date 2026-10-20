package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// maxDatasetUpload caps an upload request: a 2 MB .xlsx plus the form around it.
// ponytail: a plain cap; #42 refuses files over 2 MB with the limit stated.
const maxDatasetUpload = 2<<20 + 64<<10

// dashboard is one of a Learner's Dashboards. Dataset is nil until the first upload.
type dashboard struct {
	ID       int64
	Template *lab.Template
	Name     string
	Choices  lab.Choices
	Dataset  *lab.Dataset
	File     string // the uploaded Dataset's file name
	Updated  string // formatted as periodTime
}

// dashboardRow is a Dashboard in the Lab list.
type dashboardRow struct {
	ID       int64
	Name     string
	Template string
	Updated  string
}

// labDashboards lists a Learner's Dashboards, most recently changed first.
func (a *App) labDashboards(r *http.Request, learnerID int64) ([]dashboardRow, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id, name, template, updated_at FROM dashboards
		WHERE learner_id = $1 ORDER BY updated_at DESC, id DESC`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []dashboardRow
	for rows.Next() {
		var d dashboardRow
		var key string
		var updated time.Time
		if err := rows.Scan(&d.ID, &d.Name, &key, &updated); err != nil {
			return nil, err
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
	var choices, dataset []byte
	var updated time.Time
	err := a.db.QueryRowContext(ctx, `SELECT id, template, name, choices, dataset, dataset_file, updated_at
		FROM dashboards WHERE `+where, args...).
		Scan(&d.ID, &key, &d.Name, &choices, &dataset, &d.File, &updated)
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
	if dataset != nil {
		if err := json.Unmarshal(dataset, &d.Dataset); err != nil {
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
			serverError(w, err)
			return
		}
		if d == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r, d)
	})
}

// labOpen applies the Lab access rule to a request that builds or changes something,
// answering 403 when the Learner may not.
func (a *App) labOpen(w http.ResponseWriter, r *http.Request) bool {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return false
	}
	acc, err := a.labAccessOf(r.Context(), learner.ID)
	if err != nil {
		serverError(w, err)
		return false
	}
	if !acc.Open() {
		a.render(w, r, http.StatusForbidden, "message", page{Title: "The Lab is locked",
			Message: "Building, uploading and deploying need a Plan and a Course that includes the Lab. See the Lab page for how to unlock it."})
	}
	return acc.Open()
}

// newDashboardPage is /lab/new: pick a Template, see its blanks, download its Sample, name the Dashboard.
type newDashboardPage struct {
	pageBase
	Templates []*lab.Template
	Picked    string
	Name      string
}

func (a *App) newDashboardForm(w http.ResponseWriter, r *http.Request) {
	if !a.labOpen(w, r) {
		return
	}
	p := &newDashboardPage{pageBase: pageBase{Title: "New Dashboard"}, Templates: lab.Templates, Picked: r.URL.Query().Get("t")}
	if lab.TemplateByKey(p.Picked) == nil {
		p.Picked = lab.Templates[0].Key
	}
	a.render(w, r, http.StatusOK, "lab_new", p)
}

func (a *App) createDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.labOpen(w, r) {
		return
	}
	p := &newDashboardPage{pageBase: pageBase{Title: "New Dashboard"}, Templates: lab.Templates,
		Picked: r.FormValue("template"), Name: strings.TrimSpace(r.FormValue("name"))}
	switch {
	case lab.TemplateByKey(p.Picked) == nil:
		p.Error, p.Picked = "Choose a Template.", lab.Templates[0].Key
	default:
		p.Error = dashboardNameError(p.Name)
	}
	if p.Error != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "lab_new", p)
		return
	}
	learner, _ := a.currentLearner(r)
	var id string
	if err := a.db.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, template, name) VALUES ($1, $2, $3)
		RETURNING id`, learner.ID, p.Picked, p.Name).Scan(&id); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/lab/dashboards/"+id, http.StatusSeeOther)
}

func dashboardNameError(name string) string {
	switch {
	case name == "":
		return "Give the Dashboard a name."
	case len(name) > 80:
		return "Keep the name to 80 characters or fewer."
	}
	return ""
}

// renameDashboard changes a Dashboard's name; the rename tab is its form.
func (a *App) renameDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if !a.labOpen(w, r) {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if msg := dashboardNameError(name); msg != "" {
		p := &dashboardPage{D: d, Tab: "rename"}
		p.Error = msg
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET name = $2, updated_at = now() WHERE id = $1`, d.ID, name); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/lab", http.StatusSeeOther)
}

// deleteDashboard deletes a Dashboard and its Dataset; the delete tab asks first. Its Deployment's link
// then answers "not found", and the Deployment still counts against its period.
func (a *App) deleteDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if !a.labOpen(w, r) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM dashboards WHERE id = $1`, d.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/lab", http.StatusSeeOther)
}

// labSample downloads a Template's Sample, /lab/samples/{key}.xlsx. Samples are invented data, open to anyone.
func (a *App) labSample(w http.ResponseWriter, r *http.Request) {
	key, ok := strings.CutSuffix(r.PathValue("file"), ".xlsx")
	t := lab.TemplateByKey(key)
	if !ok || t == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+t.Key+`-sample.xlsx"`)
	_, _ = w.Write(t.Sample)
}

// dashboardPage is the Dashboard workspace: tabs Dataset, Columns, Preview and Deploy.
// ponytail: Under the hood (#45) adds its tab to lab_dashboard.html.
type dashboardPage struct {
	pageBase
	D      *dashboard
	Tab    string
	Errors []string // what was wrong with an upload
	Blanks []blankChoice
	Phone  bool // preview at phone width

	// Deploy tab
	Access     labAccess
	Deployment *deployment // nil until deployed
	Confirm    bool        // the confirm step
	WouldSleep string      // when a Deployment made now would sleep
}

// blankChoice is one blank on the Columns tab: the columns that may fill it and the current choice.
type blankChoice struct {
	lab.Blank
	Options []string
	Chosen  string
	Guess   string
}

func (a *App) dashboardTab(tab string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		a.renderDashboard(w, r, http.StatusOK, &dashboardPage{D: d, Tab: tab, Phone: r.URL.Query().Has("phone")})
	})
}

func (a *App) renderDashboard(w http.ResponseWriter, r *http.Request, status int, p *dashboardPage) {
	p.Title = p.D.Name
	if p.Tab == "columns" && p.D.Dataset != nil {
		guess := p.D.Template.Guess(p.D.Dataset)
		for _, b := range p.D.Template.Blanks {
			bc := blankChoice{Blank: b, Chosen: p.D.Choices[b.Key], Guess: guess[b.Key]}
			for _, c := range p.D.Dataset.Columns {
				if c.Type == b.Type {
					bc.Options = append(bc.Options, c.Name)
				}
			}
			p.Blanks = append(p.Blanks, bc)
		}
	}
	a.render(w, r, status, "lab_dashboard", p)
}

// uploadDataset replaces the Dashboard's Dataset with an uploaded .xlsx and pre-fills the column choices.
// ponytail: always re-guesses the choices; #42 keeps them when the columns still match.
func (a *App) uploadDataset(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if !a.labOpen(w, r) {
		return
	}
	refuse := func(errs ...string) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{D: d, Tab: "dataset", Errors: errs})
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDatasetUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		refuse("Choose an Excel file (.xlsx) of up to 2 MB.")
		return
	}
	defer file.Close()
	ds, err := lab.Read(file, d.Template)
	var problems lab.Errors
	if errors.As(err, &problems) {
		refuse(problems...)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	dataset, err := json.Marshal(ds)
	if err != nil {
		serverError(w, err)
		return
	}
	choices, err := json.Marshal(d.Template.Guess(ds))
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET dataset = $2, dataset_file = $3, choices = $4,
		updated_at = now() WHERE id = $1`, d.ID, dataset, header.Filename, choices); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, dashboardURL(d)+"/columns", http.StatusSeeOther)
}

// saveColumns stores the Learner's column choice for each blank; an empty value skips an optional blank.
func (a *App) saveColumns(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if !a.labOpen(w, r) {
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
	if _, _, err := lab.Generate(d.Template, d.Name, c, d.Dataset); err != nil {
		d.Choices = c
		p := &dashboardPage{D: d, Tab: "columns"}
		p.Error = err.Error()
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	choices, err := json.Marshal(c)
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET choices = $2, updated_at = now() WHERE id = $1`,
		d.ID, choices); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, dashboardURL(d)+"/preview", http.StatusSeeOther)
}

func dashboardURL(d *dashboard) string { return "/lab/dashboards/" + strconv.FormatInt(d.ID, 10) }

// dashboardFile serves the generated Dashboard to its owner for the preview: index.html, data.json and
// charts.js side by side, so the page loads the others by relative path. Generated on each request.
// The preview runs same-origin (it needs the session cookie for data.json), so the policy keeps
// the page to Cave's own scripts and connections, and to framing by Cave. A Deployment serves the
// same files under /d/<slug>/ (deploymentFile).
func (a *App) dashboardFile(name string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		serveDashboardFile(w, r, d, name,
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'")
	})
}

// serveDashboardFile writes one of a Dashboard's three files; csp is the index page's Content-Security-Policy.
func serveDashboardFile(w http.ResponseWriter, r *http.Request, d *dashboard, name, csp string) {
	if name == "charts.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(lab.ChartScript)
		return
	}
	if d.Dataset == nil {
		http.NotFound(w, r)
		return
	}
	index, data, err := lab.Generate(d.Template, d.Name, d.Choices, d.Dataset)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if name == "data.json" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}
