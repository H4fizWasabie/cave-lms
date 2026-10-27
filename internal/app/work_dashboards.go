package app

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// previewRows is how many cleaned rows a Dashboard's upload preview shows.
const previewRows = 20

// cleaned is what Clean-up found on an upload besides its data, kept with the Part as dataset_parts.cleanup: the
// preview shows it, the next upload compares its changes, and the Dashboard remembers its Settings on confirm.
type cleaned struct {
	lab.CleanUp              // without its Dataset
	Settings    lab.Settings `json:"settings"`        // the remembered part of the settings Clean-up read with
	Columns     []lab.Column `json:"columns"`         // the Part's columns, in the order its stored rows read back
	Added       *addition    `json:"added,omitempty"` // an Add to data; nil for one that replaces the data
}

// addition is what an Add to data put together: the upload's rows and the rows the Dataset had before them.
type addition struct {
	Rows int `json:"rows"`
	Had  int `json:"had"`
}

// Total is the Dataset's rows once the Add is confirmed.
func (a addition) Total() int { return a.Rows + a.Had }

// pendingDataset is a Dashboard's upload after Clean-up, a pending Part waiting for its Learner to confirm the preview.
// Until then the Dashboard and its Deployment keep the current Dataset. Dataset holds the upload's rows alone.
type pendingDataset struct {
	cleaned
	ID      int64
	Key     string // where the Part's rows are stored
	File    string
	Size    int64 // saved size, which counts against the Storage allowance once confirmed
	Dataset *lab.Dataset
}

// Picked is the sheet Clean-up read first, the one the others joined.
func (p *pendingDataset) Picked() string {
	for _, s := range p.Sheets {
		if s.Readable && !s.CanCombine && s.ReasonKey == "" {
			return s.Name
		}
	}
	return ""
}

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
// its confirm deletes: every current Part when it replaces the data, and the earlier Datasets from before Parts,
// which any confirm deletes. dashboardID is 0 for a Dashboard not made yet. The Admin has no allowance.
func roomFor(ctx context.Context, q queryer, dashboardID, size int64, replace bool, acc labAccess) (storageRoom, error) {
	period := acc.CountedPeriod()
	if period == nil {
		return storageRoom{Fits: true}, nil
	}
	var room storageRoom
	if err := q.QueryRowContext(ctx, `SELECT `+storageUsedSQL("this.learner_id")+`,
			(SELECT COALESCE(sum(p.saved_size), 0) FROM dataset_parts p
				WHERE p.dashboard_id = this.id AND (p.state = 'earlier' OR p.state = 'current' AND $2::boolean))
		FROM dashboards this WHERE this.id = $1`, dashboardID, replace).Scan(&room.Used, &room.Freed); err != nil {
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
	err := a.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, cleanup FROM dataset_parts
		WHERE dashboard_id = $1 AND state = 'pending'`, d.ID).Scan(&p.ID, &p.Key, &p.File, &p.Size, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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

// errDatasetStore is keepPending's and keepWorkbook's error when the store refuses the Dataset or the Workbook.
var errDatasetStore = errors.New("the store refused the Dataset")

// keepPending keeps Clean-up's result on the file called name as the Dashboard's pending Part, replacing any earlier
// pending one, and returns its saved size. With acc's counted period, the Learner's other pending Parts count against
// the Storage allowance too. A Part that would not fit, even with the Parts its confirm deletes gone (all the current
// ones when replace, else none; see roomFor), is dropped with errStorageFull; used is then the bytes the Learner's
// Parts, Workbooks and pages take, pending Parts included. workbook is the Workbook the upload was kept as, 0 for none.
func (a *App) keepPending(r *http.Request, d *dashboard, acc labAccess, res cleaned, name string, workbook int64) (size, used int64, err error) {
	res.Columns = res.Dataset.Columns
	if res.PartDate == "" { // the Excel names no date: the upload day
		res.PartDate, res.DateFrom, res.DateCell = a.now().UTC().Format(time.DateOnly), "upload", ""
	}
	day := res.PartDate
	data, err := lab.PartRows(res.Dataset, day)
	if err != nil {
		return 0, 0, err
	}
	c, err := json.Marshal(res)
	if err != nil {
		return 0, 0, err
	}
	key, err := newDatasetKey()
	if err != nil {
		return 0, 0, err
	}
	if err := a.datasets.Put(r.Context(), key, data); err != nil {
		a.logError(r, fmt.Errorf("put Part %s: %w", key, err))
		return 0, 0, errDatasetStore
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if acc.CountedPeriod() != nil {
			// Locking the Learner makes uploads and confirms take turns, so none slips under the allowance.
			if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending'`, d.ID); err != nil {
			return err
		}
		if acc.CountedPeriod() != nil {
			var pending int64
			if err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(sum(p.saved_size), 0) FROM dataset_parts p
				JOIN dashboards owner ON owner.id = p.dashboard_id JOIN dashboards this ON this.learner_id = owner.learner_id
				WHERE this.id = $1 AND p.state = 'pending'`, d.ID).Scan(&pending); err != nil {
				return err
			}
			// ponytail: what confirm deletes counts as free before it is, so the store can hold up to about twice the
			// allowance until the Learner confirms; refusing those uploads would refuse ones that fit once confirmed.
			room, err := roomFor(r.Context(), tx, d.ID, pending+int64(len(data)), res.Added == nil, acc)
			if err != nil {
				return err
			}
			if used = room.Used + pending; !room.Fits {
				return errStorageFull
			}
		}
		var from any // NULL for a Part with no Workbook
		if workbook != 0 {
			from = workbook
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, cleanup, workbook_id, part_date)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8)`, d.ID, key, name, len(data), len(res.Dataset.Rows), c, from, day)
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		return int64(len(data)), used, err
	}
	a.dropDeletedFiles(r.Context(), r)
	return int64(len(data)), used, nil
}

// uploadWork keeps a Dashboard's upload as its Learner's Workbook, then runs Clean-up on it and keeps the result as its
// pending Part, replacing any earlier pending one, then shows the preview. A file Clean-up cannot read gets one
// message. Clean-up reads with the Dashboard's remembered settings, unless the upload comes with the preview's sheet
// choices, which the Dashboard remembers once the Learner confirms. When Clean-up made only the kinds of change it made
// on the latest Part, every column the blocks use is still there, the Dataset has at most one Part for a Replace to
// delete and the Part fits the Storage allowance, there is nothing new to check: the upload is confirmed at once.
func (a *App) uploadWork(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, file io.Reader, name string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := d.Settings
	settings.FileName = name
	// A header row that is not a number lets Cave find it, as when left empty. It counts for this upload only.
	header, _ := strconv.Atoi(r.FormValue("header"))
	settings.HeaderRow = max(header, 0)
	settings.TwoRowHeader = r.FormValue("two-row") == "on"
	if sheet := r.FormValue("sheet"); sheet != "" {
		settings.Sheet, settings.Alone, settings.Ticked = sheet, false, nil
		// Ticks were made against the sheet the preview showed picked; for another sheet, every match joins.
		if sheet == r.FormValue("shown") && r.FormValue("tickable") != "" {
			settings.Ticked = append([]string{}, r.Form["combine"]...)
			settings.Alone = len(settings.Ticked) == 0
		}
	}
	// The grid month and year hold for this upload only, so the next month's workbook is dated afresh.
	settings.Month, _ = strconv.Atoi(r.FormValue("month"))
	if settings.Year, _ = strconv.Atoi(r.FormValue("year")); settings.Year < 1900 || settings.Year > 2100 {
		settings.Year = 0 // this year
	}
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
	// Add to data on a Dashboard with a Dataset adds a Part; any other upload replaces the data (ADR 0022).
	add := r.FormValue("mode") == "add" && d.Parts > 0
	// The Workbook is kept before Clean-up reads it, so it stays whatever Clean-up finds or the Learner confirms (ADR 0021).
	// A refused upload shows the Learner the message of the refusal: the Workbook that does not fit, or the store's.
	refused := func(size, used int64, err error) bool {
		switch {
		case err == nil:
			return false
		case errors.Is(err, errStorageFull):
			page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
			page.Error = text.T(lang, "lab.err.storage-full", sizeText(size), sizeText(used), sizeText(acc.Period.Storage()))
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		case errors.Is(err, errDatasetStore):
			a.renderDashboard(w, r, http.StatusServiceUnavailable, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Failed: true})
		default:
			a.serverError(w, r, err)
		}
		return true
	}
	workbook, used, err := a.keepWorkbook(r, learner.ID, d.ID, acc, !add, name, data)
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
	kept := cleaned{CleanUp: *res, Settings: settings}
	if add {
		// Only a Dashboard Cave builds in a Theme keeps a row total; one behind the Learner's own page is limited by the
		// Storage allowance alone (ADR 0022).
		limit := lab.MaxCleanRows
		if d.Page != nil {
			limit = 0
		}
		more, changes, err := lab.AddPart(d.Columns, d.Rows, res.Dataset, limit)
		if errors.As(err, &problem) {
			page := &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Upload: name}
			page.Error = problem.In(lang)
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
			return
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		kept.Added = &addition{Rows: len(more.Rows), Had: d.Rows}
		kept.Dataset = more
		kept.Changes = append(kept.Changes, changes...)
	}
	size, used, err := a.keepPending(r, d, acc, kept, name, workbook)
	if refused(size, used, err) {
		return
	}
	// Nothing new to check: it goes live. An Add is always shown, as its rows may repeat ones already there.
	if d.Config != nil && !add && d.Parts <= 1 && allFit(d.Config, res.Dataset) {
		usual, err := a.routine(r.Context(), d, res.Changes)
		var room storageRoom
		if err == nil && usual {
			room, err = roomFor(r.Context(), a.db, d.ID, size, true, acc)
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

// confirmWork makes the pending Part current. An Add to data puts it last in the Dataset's order; a Replace makes it the
// only Part, deleting the others, and any Dataset from before Parts is deleted either way, their files queued for
// deletion from the store. The first confirm of a built Dashboard builds its blocks by the General rules (ADR 0019).
// Later confirms keep the blocks, swapping in the Learner's pick (use:<column>) for each column they use that the
// Dataset lacks; one left out pauses its blocks. A confirm that would take the Learner over their Storage allowance
// even with the Parts it deletes gone is refused and the Part stays pending.
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
		replace := p.Added == nil
		if room, err = roomFor(r.Context(), tx, d.ID, p.Size, replace, acc); err != nil || !room.Fits {
			return cmp.Or(err, errStorageFull)
		}
		if day != p.PartDate {
			// The same length as before, so the saved size and the room check stand. The pending row is locked, so no
			// re-upload deletes this object meanwhile.
			rows, err := lab.PartRows(p.Dataset, day)
			if err != nil {
				return err
			}
			if err := a.datasets.Put(r.Context(), p.Key, rows); err != nil {
				return fmt.Errorf("put Part %s: %w", p.Key, err)
			}
			p.PartDate, p.DateFrom, p.DateCell = day, "chosen", ""
			c, err := json.Marshal(p.cleaned)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `UPDATE dataset_parts SET part_date = $2, cleanup = $3 WHERE id = $1`, p.ID, day, c); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND (state = 'earlier' OR state = 'current' AND $2::boolean)`,
			d.ID, replace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE dataset_parts SET state = 'current', confirmed_at = now(),
			position = (SELECT COALESCE(max(position), 0) + 1 FROM dataset_parts WHERE dashboard_id = $2 AND state = 'current')
			WHERE id = $1`, p.ID, d.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET config = $2, upload_settings = $3, dataset_columns = $4, updated_at = now() WHERE id = $1`,
			d.ID, config, settings, columns)
		return err
	})
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
