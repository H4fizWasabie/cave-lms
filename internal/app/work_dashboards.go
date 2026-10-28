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
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// previewRows is how many cleaned rows a Dashboard's upload preview shows.
const previewRows = 20

// mapPick is a column of the Dataset the Learner may read an upload's column as, for a dropdown: File is the upload's
// column now read as it, if any, and Options the upload's columns to pick from.
type mapPick struct {
	Column  string
	File    string
	Options []string
}

// MapPicks are the dropdowns of the columns the Learner mapped, so a wrong choice can be changed (or cleared) when the
// file is read again; the ones Cave matched by name are not asked about.
func (p *pendingDataset) MapPicks() []mapPick {
	var picks []mapPick
	for _, m := range p.Mapped {
		if m.Chosen {
			picks = append(picks, mapPick{Column: m.Column, File: m.File, Options: append([]string{m.File}, p.Unused...)})
		}
	}
	return picks
}

// hiddenField is a field the mapping form posts back as it was.
type hiddenField struct{ Name, Value string }

// columnMapping is the form a refused Add shows for the Dataset columns the upload lacks: it posts the Workbook the
// upload was kept as, with the choices the Learner already made and the upload's other settings, to the same address.
type columnMapping struct {
	Action string
	Hidden []hiddenField
	Picks  []mapPick
}

// pendingDataset is a Dashboard's upload after Clean-up, a pending Part waiting for its Learner to confirm the preview,
// with what the preview shows of it. Until then the Dashboard and its Deployment keep the current Dataset. Its Dataset
// holds the upload's rows alone.
type pendingDataset struct{ *dataset.Pending }

// IsAdd reports whether confirming the Part adds it to the Dataset.
func (p *pendingDataset) IsAdd() bool { return p.Kind == dataset.Add }

// IsReplace reports whether confirming the Part replaces one Part of the Dataset.
func (p *pendingDataset) IsReplace() bool { return p.Kind == dataset.ReplacePart }

// Picked is the sheet Clean-up read first, the one the others joined.
func (p *pendingDataset) Picked() string { return p.PickedSheet }

// DateFromKey is the text catalog key saying where the Part's date came from, for the preview.
func (p *pendingDataset) DateFromKey() string {
	switch p.DateFrom {
	case lab.DateFromCell, lab.DateFromSheet, lab.DateFromFile:
		return "lab.w.date-" + p.DateFrom
	}
	return "lab.w.date-upload"
}

// In reports whether the sheet called name went into the Dataset.
func (p *pendingDataset) In(name string) bool { return slices.Contains(p.Used, name) }

// Tickable reports whether any sheet could be ticked to join the picked one.
func (p *pendingDataset) Tickable() bool {
	return slices.ContainsFunc(p.Sheets, func(s lab.Sheet) bool { return s.CanCombine })
}

// Grid is the month and year Clean-up dated the days-across grids in, for the preview to ask again; nil if the
// upload had no grid.
func (p *pendingDataset) Grid() *struct{ Month, Year int } {
	if p.Month == 0 {
		return nil
	}
	return &struct{ Month, Year int }{p.Month, p.Year}
}

// UsedList is the sheets that went in, for the preview.
func (p *pendingDataset) UsedList() string { return strings.Join(p.Used, ", ") }

// Rows are the first previewRows cleaned rows as the preview shows them.
func (p *pendingDataset) Rows() [][]string {
	rows := p.Dataset.Rows
	var out [][]string
	for _, row := range rows[:min(len(rows), previewRows)] {
		cells := make([]string, len(row))
		for i, v := range row {
			switch v := v.(type) {
			case nil:
			case float64:
				cells[i] = strconv.FormatFloat(v, 'f', -1, 64)
			default:
				cells[i] = fmt.Sprint(v)
			}
		}
		out = append(out, cells)
	}
	return out
}

// inTx runs f in a transaction, committed when f returns nil.
func (a *App) inTx(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	forgetLabAccess(ctx) // the save changed the storage used
	return nil
}

// pendingOf loads the Dashboard's pending Part, its rows read from the store; nil if it has none.
func (a *App) pendingOf(ctx context.Context, d *dashboard) (*pendingDataset, error) {
	p, err := a.dataset.Pending(ctx, d.ID)
	if p == nil || err != nil {
		return nil, err
	}
	return &pendingDataset{p}, nil
}

// maxCleanUps is how many uploads Clean-up reads at once: a 1.5 MB workbook peaks near 300 MB.
const maxCleanUps = 4

// cleanUpWaitLog is how long an upload may wait for its turn before the wait goes in the System log, so we know
// when reading in the Learner's browser becomes worth building.
const cleanUpWaitLog = 3 * time.Second

// clean runs Clean-up on an upload once one of the maxCleanUps turns is free; until then it waits, unless the
// Learner gives up.
func (a *App) clean(r *http.Request, file io.Reader, s lab.Settings) (*lab.CleanUp, error) {
	start := time.Now()
	select {
	case a.cleaning <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	defer func() { <-a.cleaning }()
	if waited := time.Since(start); waited > a.cleanUpWaitLog {
		a.logInfo("An upload waited %s for its turn to be read", waited.Round(time.Millisecond))
	}
	return lab.Clean(file, s)
}

// errDatasetStore is keepWorkbook's error when the store refuses the Workbook, and the Dataset module's when it refuses
// a Part.
var errDatasetStore = dataset.ErrStore

// settingFields are the form fields that choose how Clean-up reads an upload. The mapping form posts them back, so the
// file is read again as it was; uploadSettings reads them through settingField, so none it reads can be left out.
var settingFields = []string{"header", "two-row", "sheet", "month", "year"}

// settingField is the value of the form field name, one of settingFields.
func settingField(r *http.Request, name string) string {
	if !slices.Contains(settingFields, name) {
		panic("settingField: " + name + " is not in settingFields")
	}
	return r.FormValue(name)
}

// uploadSettings is how Clean-up reads the upload called name: the Dashboard's remembered settings, with the choices
// the Learner made on the upload form. An Add or a Replace of a Part is read with the Dataset's recipe (the Dataset
// module's; nil for the first Part and Replace data, which are read as ever and set a new recipe on confirm).
func uploadSettings(r *http.Request, d *dashboard, name string, recipe *lab.Recipe) lab.Settings {
	settings := d.Settings
	settings.Recipe = recipe
	if recipe != nil {
		// A Part is read with the recipe: its sheet alone, found by name, and its header row if the Learner typed it. A
		// remembered sheet or tick belongs to the Dataset's first Part; only the Learner's pick below counts.
		settings.Sheet, settings.Alone, settings.Ticked = "", false, nil
	}
	settings.FileName = name
	// A header row that is not a number lets Cave find it, as when left empty. It counts for this upload only.
	header, _ := strconv.Atoi(settingField(r, "header"))
	settings.HeaderRow = max(header, 0)
	settings.TwoRowHeader = settingField(r, "two-row") == "on"
	if sheet := settingField(r, "sheet"); sheet != "" {
		settings.Sheet, settings.Alone, settings.Ticked = sheet, false, nil
		// Ticks were made against the sheet the preview showed picked; for another sheet, every match joins.
		if sheet == r.FormValue("shown") && r.FormValue("tickable") != "" {
			settings.Ticked = append([]string{}, r.Form["combine"]...)
			settings.Alone = len(settings.Ticked) == 0
		}
	}
	// The grid month and year hold for this upload only, so the next month's workbook is dated afresh.
	settings.Month, _ = strconv.Atoi(settingField(r, "month"))
	if settings.Year, _ = strconv.Atoi(settingField(r, "year")); settings.Year < 1900 || settings.Year > 2100 {
		settings.Year = 0 // this year
	}
	return settings
}

// mapPicks are the Learner's picks on the form: a field map:<Dataset column> holds the upload's column to read as it, or
// nothing to read none as it.
func mapPicks(form url.Values) map[string]string {
	picks := map[string]string{}
	for field, values := range form {
		if column, ok := strings.CutPrefix(field, "map:"); ok {
			picks[column] = values[0]
		}
	}
	return picks
}

// mappingForm is the form to map the columns of a refused upload that the Dataset still lacks, from the file's columns
// that are left; nil when none are left to pick from. Choices already made, for columns not asked about again, and the
// upload's other settings go in it as hidden fields, so the file is read again as it was.
func mappingForm(r *http.Request, d *dashboard, e *dataset.MissingColumnsError, workbook int64, kind dataset.Kind, replaces int64) *columnMapping {
	missing, unused := e.Missing, e.Unused
	if len(unused) == 0 {
		return nil
	}
	form := &columnMapping{Action: dashboardURL(d) + "/dataset", Hidden: []hiddenField{{"workbook", strconv.FormatInt(workbook, 10)}}}
	if kind == dataset.ReplacePart {
		form.Action = fmt.Sprintf("%s/parts/%d/replace", dashboardURL(d), replaces)
	} else {
		form.Hidden = append(form.Hidden, hiddenField{"mode", "add"})
	}
	for _, field := range settingFields {
		if v := r.FormValue(field); v != "" {
			form.Hidden = append(form.Hidden, hiddenField{field, v})
		}
	}
	for _, field := range slices.Sorted(maps.Keys(r.Form)) {
		if column, ok := strings.CutPrefix(field, "map:"); ok && !slices.Contains(missing, column) {
			form.Hidden = append(form.Hidden, hiddenField{field, r.Form.Get(field)})
		}
	}
	for _, column := range missing {
		form.Picks = append(form.Picks, mapPick{Column: column, Options: unused})
	}
	return form
}

// uploadWork keeps a Dashboard's upload as its Learner's Workbook, then runs Clean-up on it and has the Dataset module
// keep the result as its pending Part, replacing any earlier pending one, then shows the preview. A file Clean-up cannot
// read gets one message. Clean-up reads with the Dashboard's remembered settings, unless the upload comes with the
// preview's sheet choices, which the Dashboard remembers once the Learner confirms. When the Dataset has no Part, Clean-up
// made only the kinds of change it made on the latest Part, every column the blocks use is still there and the Part fits
// the Storage allowance, there is nothing new to check: the upload is confirmed at once.
// replacing is the Part the upload replaces, from its Replace link; nil for an upload to the Dataset. Replacing a Part,
// even the only one, is read like an Add and takes the Part's place; changing the columns is for Replace data.
func (a *App) uploadWork(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, file io.Reader, name string, replacing *partRef) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The form asks for Add to data or Replace data; the Dataset module makes an Add on a Dataset without Parts Replace
	// data (ADR 0022).
	kind, replaces := dataset.ReplaceData, int64(0)
	switch {
	case replacing != nil:
		kind, replaces = dataset.ReplacePart, replacing.ID
	case r.FormValue("mode") == "add":
		kind = dataset.Add
	}
	recipe, err := a.dataset.Recipe(r.Context(), d.ID, kind)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := uploadSettings(r, d, name, recipe)
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The exact same file as a current Part is refused before anything is kept (the Dataset module's Upload refuses it
	// too).
	sum := sha256.Sum256(data)
	err = a.dataset.SameFile(r.Context(), d.ID, kind, replaces, sum)
	var same *dataset.SameFileError
	if errors.As(err, &same) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name, Same: &samePart{ID: same.ID, File: same.File, Date: same.Date}})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The Workbook is kept before Clean-up reads it, so it stays whatever Clean-up finds or the Learner confirms (ADR
	// 0021). A refused upload shows the Learner the message of the refusal: the Workbook that does not fit, or the
	// store's.
	refused := func(err error) bool {
		switch msg := storageMessage(lang, err); {
		case err == nil:
			return false
		case msg != "":
			page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
			page.Error = msg
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		case errors.Is(err, errDatasetStore):
			if err != errDatasetStore { // the module's, wrapped; keepWorkbook logs its own bare one where it happened
				a.logError(r, err)
			}
			a.renderDashboard(w, r, http.StatusServiceUnavailable, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Failed: true})
		default:
			a.serverError(w, r, err)
		}
		return true
	}
	workbook, err := a.keepWorkbook(r, learner.ID, d.ID, acc, kind, replaces, name, data)
	if refused(err) {
		return
	}
	res, err := a.clean(r, bytes.NewReader(data), settings)
	var problem lab.Problem
	if errors.As(err, &problem) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Problem: problem.In(lang), Upload: name})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	_, err = a.dataset.Upload(r.Context(), d.ID, dataset.Uploaded{Kind: kind, Replaces: replaces, File: name, Sum: sum, Workbook: workbook,
		Read: res, Settings: settings, Picks: mapPicks(r.Form), FromWorkbook: pickedID(r) != ""}, partReserve(acc, learner.ID, d.ID, kind, replaces))
	forgetLabAccess(r.Context())
	var missing *dataset.MissingColumnsError
	switch {
	case errors.As(err, &same): // another upload of it got in first
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name, Same: &samePart{ID: same.ID, File: same.File, Date: same.Date}})
		return
	case errors.As(err, &missing):
		page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
		problem = missing.Problem
		if workbook != 0 {
			page.Mapping = mappingForm(r, d, missing, workbook, kind, replaces)
			if page.Mapping != nil && problem.Key == "lab.err.add-columns" {
				problem.Key = "lab.err.add-columns-map" // the refusal says the columns can be matched below
			}
		}
		page.Error = problem.In(lang)
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		return
	case errors.As(err, &problem): // the Part does not fit the Dataset
		page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
		page.Error = problem.In(lang)
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		return
	case errors.Is(err, dataset.ErrPartGone):
		a.notFound(w, r)
		return
	}
	if refused(err) {
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	// Nothing new to check: it goes live. An Add or a Replace of a Part has Parts to keep, so it is always shown: its
	// rows may repeat ones already there.
	if d.Config != nil && d.Parts == 0 && allFit(d.Config, res.Dataset) {
		usual, err := a.dataset.Routine(r.Context(), d.ID, res.Changes)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if usual {
			a.confirmWork(w, r, d)
			return
		}
	}
	http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
}

// confirmWork has the Dataset module make the pending Part current (see dataset.Confirm for what that does to the Parts,
// the Part date and the Dataset's recipe), then goes to the Dashboard's preview. The first confirm of a built Dashboard
// builds its blocks by the General rules (ADR 0019). Later confirms keep the blocks, swapping in the Learner's pick
// (use:<column>) for each column they use that the Dataset lacks; one left out pauses its blocks. A confirm that would
// take the Learner over their Storage allowance even with the Parts it deletes gone is refused and the Part stays
// pending.
func (a *App) confirmWork(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var hook dataset.Hook
	if d.Page == nil { // a page Dashboard has no blocks: its page reads the Dataset at data.json (ADR 0018)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		hook = func(ctx context.Context, tx *sql.Tx, c *dataset.Confirmed) error {
			cfg := d.Config
			if cfg == nil {
				cfg = lab.Build(c.Dataset)
			} else {
				cfg = withPicks(cfg, c.Dataset, r.Form)
			}
			config, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE dashboards SET config = $2 WHERE id = $1`, d.ID, config)
			return err
		}
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	_, err = a.dataset.Confirm(r.Context(), d.ID, r.FormValue("part-date"), confirmReserve(acc, learner.ID, d.ID), hook)
	forgetLabAccess(r.Context())
	if errors.Is(err, dataset.ErrLostRace) { // the new upload's preview
		http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
		return
	}
	full := storageMessage(lang, err)
	if errors.Is(err, dataset.ErrPartDate) || full != "" {
		// The preview again, with the refusal.
		p, perr := a.pendingOf(r.Context(), d)
		if perr != nil {
			a.serverError(w, r, perr)
			return
		}
		if p == nil {
			http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
			return
		}
		page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Pending: p, Upload: p.File}
		if errors.Is(err, dataset.ErrPartDate) {
			page.Error = text.T(lang, "lab.err.part-date")
		} else {
			page.Error = full
		}
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	if d.Page != nil {
		http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, dashboardURL(d)+"/preview", http.StatusSeeOther)
}
