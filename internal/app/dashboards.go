package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxDatasetFile is the largest .xlsx the Lab reads; maxDatasetUpload caps the request around it.
const (
	maxDatasetFile   = 10 << 20
	maxDatasetUpload = maxDatasetFile + 64<<10
)

// errStorageFull is keepPending's and confirmWork's answer when a Dataset would take the Learner over their Storage
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

// dashboard is one of a Learner's Dashboards. Dataset is nil until its first confirm: Key, File and Size describe
// its current Dataset.
type dashboard struct {
	ID       int64
	Template *lab.Template
	Name     string
	Theme    string       // the key of the Theme it draws in
	Choices  lab.Choices  // the column filling each of its Template's blanks, for Reset
	Config   *lab.Config  // its blocks and filter bar; nil until its first confirm
	Settings lab.Settings // what it remembers for its next upload
	Dataset  *lab.Dataset
	Key      string    // where the Dataset is in the Dataset store; empty until the first confirm
	File     string    // the uploaded Dataset's file name
	Size     int64     // the Dataset's saved size, against the Storage allowance
	Updated  string    // formatted as periodTime
	Page     *htmlPage // its current page, the HTML its Learner made (ADR 0018); nil for an engine Dashboard
}

// generate writes the Dashboard's index.html and data.json, its data fetched from dataURL.
func (d *dashboard) generate(dataURL string) (index, data []byte, err error) {
	return lab.Generate(d.Template, d.Name, d.Theme, d.Config, d.Dataset, dataURL)
}

// dashboardRow is a Dashboard in the Lab list.
type dashboardRow struct {
	ID         int64
	Name       string
	Template   string
	Updated    string
	HasData    bool        // a Dataset is uploaded, so it can be downloaded
	Page       bool        // it has a page: a page Dashboard, not an engine one
	Deployment *deployment // nil until deployed
}

// labDashboards lists a Learner's Dashboards, most recently changed first.
// ponytail: one query per deployed Dashboard; join the Deployment's columns in if a Learner's list gets long.
func (a *App) labDashboards(r *http.Request, learnerID int64) ([]dashboardRow, error) {
	lang, err := a.language(r)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT d.id, d.name, d.template, d.updated_at,
			EXISTS (SELECT 1 FROM datasets ds WHERE ds.dashboard_id = d.id AND ds.state = 'current'),
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
		var key string
		var updated time.Time
		var dep bool
		if err := rows.Scan(&d.ID, &d.Name, &key, &updated, &d.HasData, &d.Page, &dep); err != nil {
			rows.Close()
			return nil, err
		}
		if dep {
			deployed = append(deployed, len(list))
		}
		d.Template, d.Updated = templateName(lang, key), updated.UTC().Format(periodTime)
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

// templateName is the name of the Template with key in lang, or key itself for one Cave no longer has.
func templateName(lang, key string) string {
	if lab.TemplateByKey(key) != nil {
		return text.T(lang, "lab.tpl."+key+".name")
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
// Its Dataset stays in the store until readDataset.
func (a *App) loadDashboard(ctx context.Context, where string, args ...any) (*dashboard, error) {
	var d dashboard
	var key string
	var choices, config, settings []byte
	var updated time.Time
	var pageKey sql.NullString
	var pageSize sql.NullInt64
	var pageUploaded sql.NullTime
	err := a.db.QueryRowContext(ctx, `SELECT d.id, d.template, d.name, d.theme, d.choices, d.config, d.upload_settings,
			COALESCE(ds.storage_key, ''), COALESCE(ds.file_name, ''), COALESCE(ds.saved_size, 0), d.updated_at,
			pg.storage_key, pg.size, pg.uploaded_at
		FROM (SELECT * FROM dashboards WHERE `+where+`) d
		LEFT JOIN datasets ds ON ds.dashboard_id = d.id AND ds.state = 'current'
		LEFT JOIN dashboard_pages pg ON pg.dashboard_id = d.id AND pg.current`, args...).
		Scan(&d.ID, &key, &d.Name, &d.Theme, &choices, &config, &settings, &d.Key, &d.File, &d.Size, &updated,
			&pageKey, &pageSize, &pageUploaded)
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
	if pageKey.Valid {
		d.Page = &htmlPage{Key: pageKey.String, Size: pageSize.Int64, Uploaded: pageUploaded.Time.UTC().Format(periodTime)}
	}
	if err := errors.Join(json.Unmarshal(choices, &d.Choices), json.Unmarshal(settings, &d.Settings)); err != nil {
		return nil, err
	}
	if config != nil {
		if err := json.Unmarshal(config, &d.Config); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// readDataset fetches the Dashboard's Dataset from the store, unless it has none or it is already read.
func (a *App) readDataset(ctx context.Context, d *dashboard) error {
	if d.Key == "" || d.Dataset != nil {
		return nil
	}
	data, err := a.datasets.Get(ctx, d.Key)
	if err != nil {
		return fmt.Errorf("get Dataset %s: %w", d.Key, err)
	}
	return json.Unmarshal(data, &d.Dataset)
}

// withDashboard runs h with the logged-in Learner's Dashboard from the URL, its Dataset read, or answers 404.
// ponytail: every Dashboard page reads the whole Dataset from the store; read it only on the tabs that use it if R2 gets slow.
func (a *App) withDashboard(h func(http.ResponseWriter, *http.Request, *dashboard)) http.HandlerFunc {
	return a.withDashboardRow(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		if err := a.readDataset(r.Context(), d); err != nil {
			a.serverError(w, r, err)
			return
		}
		h(w, r, d)
	})
}

// withDashboardRow is withDashboard leaving the Dataset in the store, for handlers that read it only when needed.
func (a *App) withDashboardRow(h func(http.ResponseWriter, *http.Request, *dashboard)) http.HandlerFunc {
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

// newDashboardPage is /lab/new: name the Dashboard, then upload its Dataset or try a Sample. Cave builds the blocks
// from the data and offers the Templates that fit it at the preview (ADR 0014).
type newDashboardPage struct {
	pageBase
	Name    string
	HTML    string          // the pasted page, kept when the form is refused
	Samples []*lab.Template // the Templates whose Sample the Learner may try
}

func newDashboard(name string) *newDashboardPage {
	p := &newDashboardPage{Name: name}
	for _, t := range lab.Templates {
		if t != lab.General {
			p.Samples = append(p.Samples, t)
		}
	}
	return p
}

func (a *App) newDashboardForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	a.render(w, r, http.StatusOK, "lab_new", newDashboard(""))
}

// createDashboard makes a Dashboard from a name and the page its Learner made (createPageDashboard). With sample, a
// Template's key, it makes an engine Dashboard instead, on that Template with its Sample, cleaned at start-up, as its
// pending Dataset, until the engine goes (#376).
func (a *App) createDashboard(w http.ResponseWriter, r *http.Request) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	tooBig := parsePageForm(w, r)
	key := r.FormValue("sample")
	if key == "" {
		a.createPageDashboard(w, r, acc, tooBig)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	t, sample := lab.TemplateByKey(key), a.samples[key]
	if t == nil || t == lab.General || sample == nil {
		a.notFound(w, r)
		return
	}
	p := newDashboard(strings.TrimSpace(r.FormValue("name")))
	if p.Error = dashboardNameError(lang, p.Name); p.Error != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "lab_new", p)
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := &dashboard{}
	if err := a.db.QueryRowContext(r.Context(), `INSERT INTO dashboards (learner_id, kind, template, name) VALUES ($1, 'work', $2, $3)
		RETURNING id`, learner.ID, t.Key, p.Name).Scan(&d.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if t != lab.General {
		// ponytail: the Sample is kept unchecked, as refusing a new Dashboard would need its own message; it is one
		// small fixed Dataset per Dashboard, and it counts against the allowance for every other upload.
		if _, _, err := a.keepPending(r, d, labAccess{}, cleaned{CleanUp: *sample}, t.Key+"-sample.xlsx"); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
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
	if !slices.ContainsFunc(lab.Themes, func(t lab.Theme) bool { return t.Key == theme }) {
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

// dashboardPage is the Dashboard page: tabs Dataset, Blocks, Preview, Under the hood and Deploy,
// plus Rename.
type dashboardPage struct {
	pageBase
	Dashboard *dashboard
	Tab       string
	Problem   string          // why an upload could not be read
	Upload    string          // the refused file's name
	Failed    bool            // the store refused the upload's Dataset
	Blanks    []blankChoice   // before the first confirm: the picked Template's blanks
	Pending   *pendingDataset // an upload waiting for its confirm
	Drops     []keptDataset   // the earlier Datasets confirming Pending deletes
	Uses      []columnUse     // after the first confirm: the columns the blocks use, as the pending Dataset has them
	AskHeader int             // most of Uses are missing: the preview asks whether this row is the header
	Offers    []string        // before the first confirm: the keys of General and the Templates the pending Dataset fits
	Picked    *lab.Template   // before the first confirm: the one of Offers the preview opens
	Building  []string        // the labels of the blocks Picked opens to
	Earlier   []keptDataset   // the earlier Datasets, newest first
	Gone      string          // a page Dashboard's replacing upload: the current columns it lacks
	New       string          // and its columns the current Dataset lacks
	Pages     []htmlPage      // the earlier pages, newest first
	Open      int             // Blocks tab: the block whose form is open; -1 for none
	Saved     bool            // Blocks tab: an edit was just saved
	Blocks    *blocksView     // Blocks tab
	Phone     bool            // preview at phone width
	ReadOnly  bool            // no Lab access: view and download only
	Hood      []hoodPart
	Data      hoodPart    // the start of data.json, on the Under the hood tab
	Themes    []lab.Theme // Preview tab: the Themes to pick from

	Access labAccess // the caller looks it up; read-only follows from it

	// Deploy tab
	Deployment *deployment // nil until deployed
	Confirm    bool        // the confirm step
	DeployName string      // the name typed on the confirm step
	Prefix     string      // a Deployment's link up to its name
}

// blankChoice is one blank of the first confirm's picked Template: the columns that may fill it and Cave's guess.
type blankChoice struct {
	lab.Blank
	Options []string
	Chosen  string
}

// blankChoices is each of the Template's blanks with the Dataset's columns that may fill it.
func blankChoices(t *lab.Template, d *lab.Dataset, chosen lab.Choices) []blankChoice {
	var out []blankChoice
	for _, b := range t.Blanks {
		bc := blankChoice{Blank: b, Chosen: chosen[b.Key]}
		for _, c := range d.Columns {
			if c.Type == b.Type {
				bc.Options = append(bc.Options, c.Name)
			}
		}
		out = append(out, bc)
	}
	return out
}

// pickOffer fills the first confirm's preview: General, the Dashboard's own Template and the Templates Clean-up
// offered, with the one keyed want picked, else the Dashboard's own; then the picked one's blanks and blocks.
func (p *dashboardPage) pickOffer(d *dashboard, want string) {
	pd := p.Pending
	p.Offers = []string{lab.General.Key}
	for _, t := range lab.Templates {
		if _, ok := pd.Offers[t.Key]; (ok || t == d.Template) && t != lab.General {
			p.Offers = append(p.Offers, t.Key)
		}
	}
	p.Picked = d.Template
	if slices.Contains(p.Offers, want) {
		p.Picked = lab.TemplateByKey(want)
	}
	guess := pd.Offers[p.Picked.Key]
	if p.Picked == d.Template {
		guess = pd.Guess
	}
	p.Blanks = blankChoices(p.Picked, pd.Dataset, guess)
	for _, b := range p.Picked.Open(guess, pd.Dataset).Blocks {
		p.Building = append(p.Building, b.Label)
	}
}

func (a *App) dashboardTab(tab string) http.HandlerFunc {
	return a.withDashboard(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		acc, err := a.learnerAccess(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		q := r.URL.Query()
		open, err := strconv.Atoi(q.Get("open"))
		if err != nil {
			open = -1
		}
		a.renderDashboard(w, r, http.StatusOK, &dashboardPage{Dashboard: d, Tab: tab, Access: acc, Phone: q.Has("phone"),
			Open: open, Saved: q.Has("saved")})
	})
}

func (a *App) renderDashboard(w http.ResponseWriter, r *http.Request, status int, p *dashboardPage) {
	d := p.Dashboard
	p.Title, p.ReadOnly, p.Themes = d.Name, !p.Access.Open(), lab.Themes
	if p.Tab == "blocks" && d.Config != nil && d.Dataset != nil {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Blocks = newBlocksView(d, lang, p.Open, p.Saved)
	}
	if p.Tab == dataTab(d) && p.Pending == nil && !p.ReadOnly {
		var err error
		if p.Pending, err = a.pendingOf(r.Context(), d); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Pending != nil && d.Config == nil && d.Page == nil { // the first confirm opens the picked Template with these choices
		p.pickOffer(d, r.URL.Query().Get("template"))
	}
	if p.Pending != nil && d.Config != nil { // later confirms keep the blocks, with a pick for each column gone
		p.Uses = columnUses(d.Config, p.Pending.Dataset)
		if 2*missingUses(p.Uses) > len(p.Uses) {
			p.AskHeader = p.Pending.HeaderRow
		}
	}
	if p.Pending != nil && p.Pending.Added == nil && d.Page != nil && d.Dataset != nil {
		// By exact name, as data.json's keys are: a page reading Jumlah breaks on jumlah.
		lacking := func(cols, others []lab.Column) string {
			var out []string
			for _, c := range cols {
				if !slices.ContainsFunc(others, func(o lab.Column) bool { return o.Name == c.Name }) {
					out = append(out, c.Name)
				}
			}
			return strings.Join(out, ", ")
		}
		p.Gone, p.New = lacking(d.Dataset.Columns, p.Pending.Dataset.Columns), lacking(p.Pending.Dataset.Columns, d.Dataset.Columns)
	}
	if p.Tab == dataTab(d) && !p.ReadOnly {
		var err error
		if p.Pending != nil {
			var room storageRoom
			room, err = roomFor(r.Context(), a.db, d, p.Pending.Size, p.Access)
			if err == nil && room.Fits {
				p.Drops = room.Drop
			}
			if err == nil && !room.Fits && p.Error == "" { // the confirm will be refused: say so now
				var lang string
				lang, err = a.language(r)
				p.Error = text.T(lang, "lab.err.storage-full", sizeText(p.Pending.Size), sizeText(room.Used), sizeText(p.Access.Period.Storage()))
			}
		}
		if err == nil {
			p.Earlier, err = a.earlierOf(r.Context(), d)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if d.Page != nil && p.Tab == "dataset" && !p.ReadOnly { // a page Dashboard's Page screen lists its earlier pages
		var err error
		if p.Pages, err = a.earlierPages(r.Context(), d.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Tab == "hood" && d.Dataset != nil {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		index, data, err := d.generate(dataPath(d))
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Hood = hoodParts(lang, string(index))
		p.Data = hoodPart{N: len(p.Hood) + 1, Code: dataExcerpt(lang, string(data)), Title: text.T(lang, "lab.hood.data.title"), Note: text.T(lang, "lab.hood.data")}
	}
	a.render(w, r, status, "lab_dashboard", p)
}

// uploadDataset checks an uploaded .xlsx's name and size, then hands it to uploadWork to clean and keep pending.
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
	refuse := func(problem string) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Problem: problem, Upload: name})
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
	case strings.EqualFold(filepath.Ext(name), ".xls"):
		refuse(text.T(lang, "lab.err.xls"))
		return
	case !strings.EqualFold(filepath.Ext(name), ".xlsx"):
		refuse(text.T(lang, "lab.err.not-xlsx", name))
		return
	case header.Size > maxDatasetFile:
		refuse(tooBig)
		return
	}
	a.uploadWork(w, r, d, acc, file, name)
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

// pruneDatasets deletes the Datasets of every Learner with no Plan period active, upcoming, or ended within
// datasetKeepDays, from the store and the records, and returns how many it deleted. The Dashboards and their
// Deployments stay, the Deployments asleep. The Admin's Datasets are never pruned. Datasets left pending
// past pendingKeepHours go too, the Admin's included.
func (a *App) pruneDatasets(ctx context.Context) (int, error) {
	work, err := a.db.ExecContext(ctx, `DELETE FROM datasets ds USING dashboards d JOIN learners l ON l.id = d.learner_id
		WHERE d.id = ds.dashboard_id AND (ds.state = 'pending' AND ds.uploaded_at < now() - make_interval(hours => $2) OR `+lapsedSQL+`)`,
		datasetKeepDays, pendingKeepHours)
	if err != nil {
		return 0, err
	}
	deleted, err := work.RowsAffected()
	if err != nil {
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

// convertPracticeDashboards is a one-off step at startup (ADR 0015): every Practice Dashboard becomes a normal one.
// Its Template opens with its column choices onto its Dataset as its blocks, and the Dataset, already in the store
// in the same format, becomes its current one under the same key, so nothing is copied or deleted and its
// Deployment, drawn from the Dashboard on each visit, keeps its link and data. A converted Dashboard is not
// Practice, so starting again changes nothing. Its Theme, Template and choices stay.
// The migration that drops dashboards.kind removes this step.
func convertPracticeDashboards(ctx context.Context, db *sql.DB, store storage.Store) error {
	type row struct {
		id             int64
		template, file string
		key            sql.NullString
		choices        []byte
	}
	rows, err := db.QueryContext(ctx, `SELECT id, template, choices, dataset_key, dataset_file FROM dashboards WHERE kind = 'practice' ORDER BY id`)
	if err != nil {
		return err
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.template, &r.choices, &r.key, &r.file); err != nil {
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
		if err := convertPractice(ctx, db, store, r.id, r.template, r.choices, r.key, r.file); err != nil {
			return fmt.Errorf("convert Practice Dashboard %d: %w", r.id, err)
		}
	}
	return nil
}

// convertPractice converts one Practice Dashboard, as convertPracticeDashboards says, in one transaction.
func convertPractice(ctx context.Context, db *sql.DB, store storage.Store, id int64, template string, choices []byte, key sql.NullString, file string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !key.Valid { // never uploaded: its blocks come at its first confirm, as for any new Dashboard
		if _, err := tx.ExecContext(ctx, `UPDATE dashboards SET kind = 'work' WHERE id = $1 AND kind = 'practice'`, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	t := lab.TemplateByKey(template)
	if t == nil {
		return fmt.Errorf("unknown template %q", template)
	}
	var c lab.Choices
	if err := json.Unmarshal(choices, &c); err != nil {
		return err
	}
	data, err := store.Get(ctx, key.String)
	if err != nil {
		return fmt.Errorf("get Dataset %s: %w", key.String, err)
	}
	var ds lab.Dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return err
	}
	config, err := json.Marshal(t.Open(c, &ds))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO datasets (dashboard_id, storage_key, file_name, saved_size, row_count, state, confirmed_at)
		VALUES ($1, $2, $3, $4, $5, 'current', now())`, id, key.String, file, len(data), len(ds.Rows)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboards SET kind = 'work', config = $2, dataset_key = NULL, dataset_file = '', dataset_size = 0
		WHERE id = $1`, id, config); err != nil {
		return err
	}
	return tx.Commit()
}

func dashboardURL(d *dashboard) string { return "/lab/dashboards/" + strconv.FormatInt(d.ID, 10) }

// dashboardFile serves the generated Dashboard to its Learner for the preview: index.html, charts.js and
// the data at data/<key> under it, so the page loads the others by relative path.
// A Deployment serves the same files under /<prefix>/<name>/ (deploymentFile).
func (a *App) dashboardFile(name string) http.HandlerFunc {
	return a.withDashboardRow(func(w http.ResponseWriter, r *http.Request, d *dashboard) {
		a.serveDashboardFile(w, r, d, name, previewCSP, "private, max-age=31536000, immutable")
	})
}

// previewCSP keeps a Dashboard served same-origin (the preview and the demo) to Cave's own scripts and
// connections, to the fonts written into charts.js, and to framing by Cave. The preview needs the session
// cookie for data.json, so no sandbox.
const previewCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; font-src data:; frame-ancestors 'self'"

// dataPath is where a Dashboard's page fetches its data, relative to the page: the Dataset's key, so a
// re-upload changes the address and no cache ever serves an old Dataset. The preview's data is cached for good
// (only its Learner sees it); a Deployment's for 5 minutes, so sleep and Takedown take effect soon.
func dataPath(d *dashboard) string { return "data/" + d.Key }

// serveDashboardFile writes one of a Dashboard's files: index.html, charts.js, data.json (the demo's data,
// at a fixed address) or data (data/<key>, under cache; any other key is not found). index.html needs only the
// Dataset's columns and the data its data.json: both are kept by key (datasetFilesCache), so the Dataset is read
// from the store once, not on each request, and never for charts.js.
// csp is the index page's Content-Security-Policy; cache is the data's Cache-Control.
func (a *App) serveDashboardFile(w http.ResponseWriter, r *http.Request, d *dashboard, name, csp, cache string) {
	if name == "charts.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(lab.ChartScript)
		return
	}
	if name == "data" && r.PathValue("key") != d.Key {
		a.notFound(w, r)
		return
	}
	wantData := name == "data.json" || name == "data"
	f, ok := a.datasetFiles.get(d.Key)
	if !ok || wantData && f.data == nil {
		if err := a.readDataset(r.Context(), d); err != nil {
			a.serverError(w, r, err)
			return
		}
		if d.Dataset == nil {
			a.notFound(w, r)
			return
		}
		data, err := lab.DataJSON(d.Dataset)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		f = datasetFiles{columns: d.Dataset.Columns, data: data}
		if !ok {
			a.datasetFiles.put(d.Key, f)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	if wantData {
		if name == "data" {
			w.Header().Set("Cache-Control", cache)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.data)
		return
	}
	dataURL := "data.json"
	if d.Key != "" {
		dataURL = dataPath(d)
	}
	index, err := lab.Page(d.Template, d.Name, d.Theme, d.Config, f.columns, dataURL)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

// datasetFiles is what a Dashboard's files need of its Dataset: its columns for index.html and its data.json.
type datasetFiles struct {
	columns []lab.Column
	data    []byte // nil when too big to keep
}

// maxKeptBytes is how many bytes datasetFilesCache keeps in all; one Dataset's data.json is kept within half of it.
const maxKeptBytes = 128 << 20

// datasetFilesCache keeps datasetFiles by Dataset key. A Dataset never changes under its key (each upload gets a
// new key; a restore points back to an earlier one, unchanged), so nothing goes stale. The demo's Dataset has no
// key and is never kept.
// ponytail: FIFO within maxKeptBytes and no single-flight, so a data.json over 64 MB is built on each data request
// (its columns are still kept) and a burst of first visits each reads the Dataset; make it an LRU, or store
// data.json beside the Dataset, if that shows up.
type datasetFilesCache struct {
	mu    sync.Mutex
	items map[string]datasetFiles
	order []string // keys, oldest first
	size  int
}

func (c *datasetFilesCache) get(key string) (datasetFiles, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, ok := c.items[key]
	return f, ok
}

func (c *datasetFilesCache) put(key string, f datasetFiles) {
	if key == "" {
		return
	}
	if len(f.data) > maxKeptBytes/2 {
		f.data = nil
	}
	n := keptSize(f)
	if n > maxKeptBytes/2 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]datasetFiles{}
	}
	if old, ok := c.items[key]; ok {
		c.size -= keptSize(old)
	} else {
		c.order = append(c.order, key)
	}
	c.items[key] = f
	c.size += n
	for c.size > maxKeptBytes {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.size -= keptSize(c.items[oldest])
		delete(c.items, oldest)
	}
}

// keptSize is about how many bytes f holds: its data, and each column's name plus its slice header and Type.
func keptSize(f datasetFiles) int {
	n := len(f.data)
	for _, col := range f.columns {
		n += len(col.Name) + 32
	}
	return n
}

// downloadDashboard is the Dashboard as a zip that opens offline: index.html, data.json, charts.js and
// data.js, the same rows as a script, which the page reads when the browser blocks fetch from a file.
// Open to its Learner with or without Lab access.
func (a *App) downloadDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if d.Dataset == nil {
		a.notFound(w, r)
		return
	}
	index, data, err := d.generate("data.json")
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
	{"  <script>\n", "lab.hood.blocks"},
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
