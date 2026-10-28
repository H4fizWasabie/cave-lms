package app

import (
	"bytes"
	"cmp"
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

// cleaned is what Clean-up found on an upload besides its data, kept with the Part as dataset_parts.cleanup: the
// preview shows it, the next upload compares its changes, and the Dashboard remembers its Settings on confirm.
type cleaned = dataset.Preview

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

// addition and replacement are what an Add and a Replace of one Part put together.
type (
	addition    = dataset.Addition
	replacement = dataset.Replacement
)

// uploadKind is what an upload does to the Dataset once confirmed (ADR 0022).
type uploadKind int

const (
	replaceData uploadKind = iota // every current Part is deleted and the upload is the Dataset
	addPart                       // the upload becomes the last Part and the others stay
	replacePart                   // the upload takes the place of one Part and the others stay
)

// dbKind is the kind as dataset_parts.upload_kind holds it.
// ponytail: two enums for one kind, mapped here and in pendingOf, until the confirm moves into the Dataset module (#460)
// and #461 deletes uploadKind.
func (k uploadKind) dbKind() dataset.Kind {
	switch k {
	case addPart:
		return dataset.Add
	case replacePart:
		return dataset.ReplacePart
	}
	return dataset.ReplaceData
}

// uploadMode is an upload's kind, with the Part it replaces for replacePart. The zero value is Replace data.
type uploadMode struct {
	Kind uploadKind
	Part int64 // the ID of the Part replaced; 0 unless Kind is replacePart, which matches no Part
}

// intoDataset reports whether the upload joins the Dataset's Parts, an Add or a Replace of one Part, so it is read with
// the Dataset's recipe and columns. Replace data starts the Dataset afresh.
func (m uploadMode) intoDataset() bool { return m.Kind != replaceData }

// otherRows is the rows the Dataset keeps besides this Part's once it is confirmed: 0 for a Part that is the whole
// Dataset.
func (p *pendingDataset) otherRows() int {
	switch {
	case p.Mode.Kind == addPart && p.Added != nil:
		return p.Added.Had
	case p.Mode.Kind == replacePart && p.Replaces != nil:
		return p.Replaces.Had
	}
	return 0
}

// pendingDataset is a Dashboard's upload after Clean-up, a pending Part waiting for its Learner to confirm the preview.
// Until then the Dashboard and its Deployment keep the current Dataset. Dataset holds the upload's rows alone.
type pendingDataset struct {
	cleaned
	ID       int64
	Key      string // where the Part's rows are stored
	File     string
	Size     int64 // saved size, which counts against the Storage allowance once confirmed
	Workbook int64 // the Workbook it was read from, 0 once that Workbook is deleted
	Dataset  *lab.Dataset
	Mode     uploadMode // what confirming it does, from the Part's columns
}

// IsAdd reports whether confirming the Part adds it to the Dataset.
func (p *pendingDataset) IsAdd() bool { return p.Mode.Kind == addPart }

// IsReplace reports whether confirming the Part replaces one Part of the Dataset.
func (p *pendingDataset) IsReplace() bool { return p.Mode.Kind == replacePart }

// ReplacedPart is the ID of the Part it replaces, 0 unless IsReplace.
func (p *pendingDataset) ReplacedPart() int64 { return p.Mode.Part }

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

// queryer is a *sql.DB or a *sql.Tx, for reads that may run inside a caller's transaction.
type queryer interface {
	rowQueryer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// storageRoom is what confirming a pending Part does to the Learner's Storage allowance.
type storageRoom struct {
	Used  int64 // the bytes the Learner's Workbooks, pages and confirmed Parts take now
	Freed int64 // the bytes the confirm deletes: the Parts a Replace removes, and any Dataset from before Parts
	Fits  bool  // false when the Part does not fit even with those gone
}

// roomFor is whether a pending Part of size bytes fits the Learner's Storage allowance, counting as free the Parts
// its confirm deletes (see mode): every current Part for Replace data, the one Part for a Replace of a Part, none for
// an Add, and for all three the earlier Datasets from before Parts. dashboardID is 0 for a Dashboard not made yet. The
// Admin has no allowance.
func roomFor(ctx context.Context, q queryer, dashboardID, size int64, mode uploadMode, acc labAccess) (storageRoom, error) {
	period := acc.CountedPeriod()
	if period == nil {
		return storageRoom{Fits: true}, nil
	}
	var room storageRoom
	if err := q.QueryRowContext(ctx, `SELECT `+storageUsedSQL("this.learner_id")+`,
			(SELECT COALESCE(sum(p.saved_size), 0) FROM dataset_parts p
				WHERE p.dashboard_id = this.id AND (p.state = 'earlier' OR p.state = 'current' AND ($2::boolean OR p.id = $3)))
		FROM dashboards this WHERE this.id = $1`, dashboardID, mode.Kind == replaceData, mode.Part).Scan(&room.Used, &room.Freed); err != nil {
		return storageRoom{}, err
	}
	room.Fits = room.Used+size-room.Freed <= period.Storage()
	return room, nil
}

// routine reports whether Clean-up's changes are all of kinds it also made on the Dashboard's latest Part, as
// when the same monthly workbook is skipped past its title rows and combined again.
func (a *App) routine(ctx context.Context, d *dashboard, changes []lab.Change) (bool, error) {
	var c []byte
	err := a.db.QueryRowContext(ctx, `SELECT cleanup FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current'
		ORDER BY position DESC LIMIT 1`, d.ID).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return len(changes) == 0, nil
	}
	var last cleaned
	if err == nil {
		err = json.Unmarshal(c, &last)
	}
	if err != nil {
		return false, err
	}
	// A kind is the change's key, one row or many alike: lab.clean.title-row is lab.clean.title-rows.
	kind := func(c lab.Change) string { return strings.TrimSuffix(c.Key, "s") }
	for _, c := range changes {
		if !slices.ContainsFunc(last.Changes, func(l lab.Change) bool { return kind(l) == kind(c) }) {
			return false, nil
		}
	}
	return true, nil
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
	return tx.Commit()
}

// pendingOf loads the Dashboard's pending Part, its rows read from the store; nil if it has none.
func (a *App) pendingOf(ctx context.Context, d *dashboard) (*pendingDataset, error) {
	var p pendingDataset
	var c []byte
	var kind dataset.Kind
	var replaces sql.NullInt64
	err := a.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, cleanup, COALESCE(workbook_id, 0), upload_kind, replaces_part_id FROM dataset_parts
		WHERE dashboard_id = $1 AND state = 'pending'`, d.ID).Scan(&p.ID, &p.Key, &p.File, &p.Size, &c, &p.Workbook, &kind, &replaces)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	switch kind {
	case dataset.Add:
		p.Mode.Kind = addPart
	case dataset.ReplacePart:
		p.Mode = uploadMode{Kind: replacePart, Part: replaces.Int64}
	}
	if err := json.Unmarshal(c, &p.cleaned); err != nil {
		return nil, err
	}
	data, err := a.datasets.Get(ctx, p.Key)
	if err != nil {
		return nil, fmt.Errorf("get Dataset %s: %w", p.Key, err)
	}
	if p.Dataset, err = lab.PartDataset(data, p.Columns); err != nil {
		return nil, fmt.Errorf("read Part %s: %w", p.Key, err)
	}
	return &p, nil
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

// pendingReserve is the Storage allowance's check for a pending Part on Dashboard d, for the Dataset module's Upload to
// call in its transaction. With acc's counted period, the Learner's pending Parts on their other Dashboards count against
// the allowance too, as the upload replaces the Dashboard's own. A Part that would not fit, even with the Parts its
// confirm deletes gone (those of mode; see roomFor), is refused with errStorageFull. size is set to the Part's saved size,
// and used to the bytes the Learner's Parts, Workbooks and pages take, pending Parts included.
func pendingReserve(d *dashboard, acc labAccess, mode uploadMode, size, used *int64) dataset.Reserve {
	return func(ctx context.Context, tx *sql.Tx, bytes int64) error {
		*size = bytes
		if acc.CountedPeriod() == nil {
			return nil
		}
		// Locking the Learner makes uploads and confirms take turns, so none slips under the allowance.
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		var pending int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(p.saved_size), 0) FROM dataset_parts p
			JOIN dashboards owner ON owner.id = p.dashboard_id JOIN dashboards this ON this.learner_id = owner.learner_id
			WHERE this.id = $1 AND p.state = 'pending' AND p.dashboard_id <> $1`, d.ID).Scan(&pending); err != nil {
			return err
		}
		// ponytail: what confirm deletes counts as free before it is, so the store can hold up to about twice the
		// allowance until the Learner confirms; refusing those uploads would refuse ones that fit once confirmed.
		room, err := roomFor(ctx, tx, d.ID, pending+bytes, mode, acc)
		if err != nil {
			return err
		}
		if *used = room.Used + pending; !room.Fits {
			return errStorageFull
		}
		return nil
	}
}

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
func mappingForm(r *http.Request, d *dashboard, e *dataset.MissingColumnsError, workbook int64, mode uploadMode) *columnMapping {
	missing, unused := e.Missing, e.Unused
	if len(unused) == 0 {
		return nil
	}
	form := &columnMapping{Action: dashboardURL(d) + "/dataset", Hidden: []hiddenField{{"workbook", strconv.FormatInt(workbook, 10)}}}
	if mode.Kind == replacePart {
		form.Action = fmt.Sprintf("%s/parts/%d/replace", dashboardURL(d), mode.Part)
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
	// Add to data on a Dashboard with a Dataset adds a Part; any other upload replaces the data (ADR 0022).
	var mode uploadMode
	switch {
	case replacing != nil:
		mode = uploadMode{Kind: replacePart, Part: replacing.ID}
	case r.FormValue("mode") == "add" && d.Parts > 0:
		mode.Kind = addPart
	}
	recipe, err := a.dataset.Recipe(r.Context(), d.ID, mode.Kind.dbKind())
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
	err = a.dataset.SameFile(r.Context(), d.ID, mode.Kind.dbKind(), mode.Part, sum)
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
	refused := func(size, used int64, err error) bool {
		switch {
		case err == nil:
			return false
		case errors.Is(err, errStorageFull):
			page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
			page.Error = text.T(lang, "lab.err.storage-full", sizeText(size), sizeText(used), sizeText(acc.Period.Storage()))
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		case errors.Is(err, errDatasetStore):
			if err != errDatasetStore { // the Workbook's refusal is logged where it happened
				a.logError(r, err)
			}
			a.renderDashboard(w, r, http.StatusServiceUnavailable, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Failed: true})
		default:
			a.serverError(w, r, err)
		}
		return true
	}
	workbook, used, err := a.keepWorkbook(r, learner.ID, d.ID, acc, mode, name, data)
	if refused(int64(len(data)), used, err) {
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
	var size int64
	pending, err := a.dataset.Upload(r.Context(), d.ID, dataset.Uploaded{Kind: mode.Kind.dbKind(), Replaces: mode.Part, File: name, Sum: sum, Workbook: workbook,
		Read: res, Settings: settings, Picks: mapPicks(r.Form), FromWorkbook: pickedID(r) != ""}, pendingReserve(d, acc, mode, &size, &used))
	var missing *dataset.MissingColumnsError
	switch {
	case errors.As(err, &same): // another upload of it got in first
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name, Same: &samePart{ID: same.ID, File: same.File, Date: same.Date}})
		return
	case errors.As(err, &missing):
		page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
		problem = missing.Problem
		if workbook != 0 {
			page.Mapping = mappingForm(r, d, missing, workbook, mode)
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
	if refused(size, used, err) {
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	// Nothing new to check: it goes live. An Add or a Replace of a Part has Parts to keep, so it is always shown: its
	// rows may repeat ones already there.
	if d.Config != nil && d.Parts == 0 && allFit(d.Config, res.Dataset) {
		usual, err := a.routine(r.Context(), d, res.Changes)
		var room storageRoom
		if err == nil && usual {
			room, err = roomFor(r.Context(), a.db, d.ID, pending.Size, uploadMode{}, acc)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if usual && room.Fits {
			a.confirmWork(w, r, d)
			return
		}
	}
	http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
}

// confirmWork makes the pending Part current. An Add to data puts it last in the Dataset's order; a Replace of the data
// makes it the only Part, deleting the others; a Replace of one Part deletes that Part and takes its place in the
// order. Any Dataset from before Parts is deleted in each case, their files queued for deletion from the store.
// The first confirm of a built Dashboard builds its blocks by the General rules (ADR 0019). Later confirms keep the
// blocks, swapping in the Learner's pick (use:<column>) for each column they use that the Dataset lacks; one left out
// pauses its blocks. A confirm that would take the Learner over their Storage allowance even with the Parts it deletes
// gone is refused and the Part stays pending.
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
	p, err := a.pendingOf(r.Context(), d)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if p == nil {
		http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
		return
	}
	// The preview's date, which the Learner may have changed; the Part's rows are rewritten with it.
	day := p.PartDate
	if v := strings.TrimSpace(r.FormValue("part-date")); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil || t.Year() < 1900 || t.Year() > 2100 {
			page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Pending: p, Upload: p.File}
			page.Error = text.T(lang, "lab.err.part-date")
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
			return
		}
		day = t.Format(time.DateOnly)
	}
	cfg := d.Config
	switch {
	case d.Page != nil: // a page Dashboard has no blocks: its page reads the Dataset at data.json (ADR 0018)
	case cfg == nil:
		cfg = lab.Build(p.Dataset)
	default:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		cfg = withPicks(cfg, p.Dataset, r.Form)
	}
	var config []byte // NULL for a page Dashboard
	if cfg != nil {
		if config, err = json.Marshal(cfg); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	mode := p.Mode
	if recipe := p.Settings.Recipe; mode.intoDataset() && recipe != nil {
		// The Dataset's values decide the month columns: with this Part's, a recorded one holding a number outside 1 to
		// 12 is dropped. A Part that is all the Dataset has is a first Part: its own values decide.
		if p.otherRows() == 0 {
			scan := lab.NewMonthScan(p.Columns)
			scan.Add(p.Dataset)
			recipe.MonthColumns = scan.Columns()
		} else if recipe.MonthColumns != nil {
			recipe.MonthColumns = lab.KeepMonthColumns(p.Columns, recipe.MonthColumns, p.Dataset)
		}
	}
	if !mode.intoDataset() { // the first Part, and Replace data, set the recipe every later Part is read with
		// empty, not nil: found, and none
		p.Settings.Recipe = &lab.Recipe{Sheet: p.PickedSheet, HeaderRow: p.HeaderRow, HeaderTyped: p.HeaderTyped, MonthColumns: append([]string{}, p.MonthColumns...)}
	}
	settings, err := json.Marshal(p.Settings)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	columns, err := json.Marshal(p.Columns)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// A changed date rewrites the Part's rows (the same length, so the saved size and the room check stand). They go under
	// a new key first and the confirm switches the Part to it, so a confirm that fails leaves the Part's object as it was.
	var newKey string
	if day != p.PartDate {
		rows, err := lab.PartRows(p.Dataset, day)
		if err == nil {
			newKey, err = newDatasetKey()
		}
		if err == nil {
			if err = a.datasets.Put(r.Context(), newKey, rows); err != nil {
				err = fmt.Errorf("put Part %s: %w", newKey, err)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	var room storageRoom
	replaced := errors.New("pending Part replaced")
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Locking the Learner makes confirms and uploads take turns, so none slips under the allowance.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		// Locking the pending Part holds off a re-upload replacing it; one that already did leaves this one gone.
		var one int
		err := tx.QueryRowContext(r.Context(), `SELECT 1 FROM dataset_parts WHERE id = $1 AND state = 'pending' FOR UPDATE`, p.ID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return replaced
		}
		if err != nil {
			return err
		}
		// A Replace of one Part takes the replaced Part's place; the Part goes if another confirm or delete has not already.
		var position sql.NullInt64
		if mode.Kind == replacePart {
			err := tx.QueryRowContext(r.Context(), `SELECT position FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current' FOR UPDATE`,
				mode.Part, d.ID).Scan(&position)
			if errors.Is(err, sql.ErrNoRows) {
				return replaced
			}
			if err != nil {
				return err
			}
		}
		if room, err = roomFor(r.Context(), tx, d.ID, p.Size, mode, acc); err != nil || !room.Fits {
			return cmp.Or(err, errStorageFull)
		}
		if newKey != "" {
			oldKey := p.Key
			p.PartDate, p.DateFrom, p.DateCell = day, lab.DateFromChosen, ""
			c, err := json.Marshal(p.cleaned)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `UPDATE dataset_parts SET storage_key = $2, part_date = $3, date_source = $4, cleanup = $5 WHERE id = $1`,
				p.ID, newKey, day, p.DateFrom, c); err != nil {
				return err
			}
			// The trigger queues the object of a deleted record only, so the old object is queued here.
			if _, err := tx.ExecContext(r.Context(), `INSERT INTO stored_file_deletions (storage_key) VALUES ($1) ON CONFLICT DO NOTHING`, oldKey); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND (state = 'earlier' OR state = 'current' AND ($2::boolean OR id = $3))`,
			d.ID, mode.Kind == replaceData, mode.Part); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE dataset_parts SET state = 'current', confirmed_at = now(),
			position = COALESCE($3::integer, (SELECT COALESCE(max(position), 0) + 1 FROM dataset_parts WHERE dashboard_id = $2 AND state = 'current'))
			WHERE id = $1`, p.ID, d.ID, position); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET config = $2, upload_settings = $3, dataset_columns = $4, updated_at = now() WHERE id = $1`,
			d.ID, config, settings, columns)
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, newKey) // the rewritten rows were not kept
	}
	if errors.Is(err, errStorageFull) {
		page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Pending: p, Upload: p.File}
		page.Error = text.T(lang, "lab.err.storage-full", sizeText(p.Size), sizeText(room.Used), sizeText(acc.Period.Storage()))
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		return
	}
	if errors.Is(err, replaced) { // the new upload's preview
		http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
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
