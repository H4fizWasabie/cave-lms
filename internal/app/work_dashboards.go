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

// previewRows is how many cleaned rows a Work Dashboard's upload preview shows.
const previewRows = 20

// cleaned is what Clean-up found on an upload besides its data, kept with the Dataset as datasets.cleanup: the
// preview shows it, the next upload compares its changes, and the Dashboard remembers its Settings on confirm.
type cleaned struct {
	Changes   []lab.Change `json:"changes"`
	Guess     lab.Choices  `json:"guess"`
	Sheets    []lab.Sheet  `json:"sheets"`          // every sheet of the upload
	Used      []string     `json:"used"`            // the sheets that went into the Dataset
	HeaderRow int          `json:"headerRow"`       // the Excel row the column names came from
	Settings  lab.Settings `json:"settings"`        // the remembered part of the settings Clean-up read with
	Month     int          `json:"month,omitempty"` // the month the days-across grids were dated in; 0 when no grid was used
	Year      int          `json:"year,omitempty"`
}

// pendingDataset is a Work Dashboard's upload after Clean-up, waiting for its Learner to confirm the preview.
// Until then the Dashboard and its Deployment keep the current Dataset.
type pendingDataset struct {
	cleaned
	ID      int64
	File    string
	Size    int64 // saved size, which counts against the Storage allowance once confirmed
	Dataset *lab.Dataset
	Chosen  lab.Choices // the Learner's choices on a refused confirm; else Guess
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
	var out [][]string
	for _, row := range p.Dataset.Rows[:min(len(p.Dataset.Rows), previewRows)] {
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

// keepEarlier is how many earlier Datasets a Work Dashboard keeps beside its current one.
const keepEarlier = 3

// keptDataset is a Work Dashboard's Dataset that is not pending: an earlier one the Learner may restore or delete,
// or the current one.
type keptDataset struct {
	ID        int64
	File      string
	Size      int64 // saved size
	Rows      int
	Confirmed string // formatted as periodTime
}

func (e keptDataset) SizeText() string { return sizeText(e.Size) }

// queryer is a *sql.DB or a *sql.Tx, for reads that may run inside a caller's transaction.
type queryer interface {
	rowQueryer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// keptDatasets lists the Work Dashboard's earlier Datasets, oldest confirmed first, then its current one, if any.
func keptDatasets(ctx context.Context, q queryer, dashboardID int64) ([]keptDataset, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, file_name, saved_size, row_count, confirmed_at FROM datasets
		WHERE dashboard_id = $1 AND state <> 'pending' ORDER BY state = 'current', confirmed_at, id`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []keptDataset
	for rows.Next() {
		var e keptDataset
		var confirmed time.Time
		if err := rows.Scan(&e.ID, &e.File, &e.Size, &e.Rows, &confirmed); err != nil {
			return nil, err
		}
		e.Confirmed = confirmed.UTC().Format(periodTime)
		out = append(out, e)
	}
	return out, rows.Err()
}

// earlierOf lists the Work Dashboard's earlier Datasets, newest first, as the Dataset tab shows them.
func (a *App) earlierOf(ctx context.Context, d *dashboard) ([]keptDataset, error) {
	kept, err := keptDatasets(ctx, a.db, d.ID)
	if err != nil || d.Key == "" { // no current Dataset, so no earlier ones
		return nil, err
	}
	kept = kept[:len(kept)-1] // the current one
	slices.Reverse(kept)
	return kept, nil
}

// storageRoom is what confirming a pending Dataset does to the Learner's Storage allowance.
type storageRoom struct {
	Drop    []keptDataset // deleted on confirm: the earlier ones past keepEarlier, then oldest first for room
	ForRoom bool          // some of Drop go only to make room under the allowance
	Used    int64         // the bytes the Learner's Datasets take now
	Fits    bool          // false when the Dataset does not fit even with every earlier one gone
}

// roomFor is what confirming a pending Dataset of size bytes deletes, the current Dataset turning earlier: the
// earlier ones past keepEarlier, then, oldest first, as many more as the Storage allowance needs. The Admin has no
// allowance.
func roomFor(ctx context.Context, q queryer, d *dashboard, size int64, acc labAccess) (storageRoom, error) {
	kept, err := keptDatasets(ctx, q, d.ID)
	if err != nil {
		return storageRoom{}, err
	}
	past := max(len(kept)-keepEarlier, 0)
	period := acc.CountedPeriod()
	if period == nil {
		return storageRoom{Drop: kept[:past], Fits: true}, nil
	}
	var room storageRoom
	if err := q.QueryRowContext(ctx, `SELECT `+storageUsedSQL("this.learner_id")+` FROM dashboards this WHERE this.id = $1`, d.ID).Scan(&room.Used); err != nil {
		return storageRoom{}, err
	}
	after := room.Used + size
	for _, e := range kept[:past] {
		after -= e.Size
	}
	n := past
	for ; after > period.Storage() && n < len(kept); n++ {
		after -= kept[n].Size
	}
	room.Drop, room.ForRoom, room.Fits = kept[:n], n > past, after <= period.Storage()
	return room, nil
}

// routine reports whether Clean-up's changes are all of kinds it also made on the Dashboard's current Dataset, as
// when the same monthly workbook is skipped past its title rows and combined again.
func (a *App) routine(ctx context.Context, d *dashboard, changes []lab.Change) (bool, error) {
	var c []byte
	err := a.db.QueryRowContext(ctx, `SELECT cleanup FROM datasets WHERE dashboard_id = $1 AND state = 'current'`, d.ID).Scan(&c)
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

// pendingOf loads the Dashboard's pending Dataset, read from the store; nil if it has none.
func (a *App) pendingOf(ctx context.Context, d *dashboard) (*pendingDataset, error) {
	var p pendingDataset
	var key string
	var c []byte
	err := a.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, cleanup FROM datasets
		WHERE dashboard_id = $1 AND state = 'pending'`, d.ID).Scan(&p.ID, &key, &p.File, &p.Size, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(c, &p.cleaned); err != nil {
		return nil, err
	}
	data, err := a.datasets.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get Dataset %s: %w", key, err)
	}
	p.Chosen = p.Guess
	return &p, json.Unmarshal(data, &p.Dataset)
}

// maxCleanUps is how many uploads Clean-up reads at once: a 1.5 MB workbook peaks near 300 MB.
const maxCleanUps = 4

// cleanUpWaitLog is how long an upload may wait for its turn before the wait goes in the System log, so we know
// when reading in the Learner's browser becomes worth building.
const cleanUpWaitLog = 3 * time.Second

// clean runs Clean-up on an upload once one of the maxCleanUps turns is free; until then it waits, unless the
// Learner gives up.
func (a *App) clean(r *http.Request, file io.Reader, t *lab.Template, s lab.Settings) (*lab.CleanUp, error) {
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
	return lab.Clean(file, t, s)
}

// cleanSamples runs Clean-up on each Template's Sample, once, so the New Dashboard page reads none.
func cleanSamples() (map[string]*lab.CleanUp, error) {
	samples := map[string]*lab.CleanUp{}
	for _, t := range lab.TemplatesFor(true) {
		res, err := lab.Clean(bytes.NewReader(t.Sample), t, lab.Settings{})
		if err != nil {
			return nil, fmt.Errorf("clean the %s Sample: %w", t.Name, err)
		}
		samples[t.Key] = res
	}
	return samples, nil
}

// uploadWork runs Clean-up on a Work Dashboard's upload and keeps the result as its pending Dataset, replacing
// any earlier pending one, then shows the preview. A file Clean-up cannot read gets one message. Clean-up reads
// with the Dashboard's remembered settings, unless the upload comes with the preview's sheet choices, which
// the Dashboard remembers once the Learner confirms. When Clean-up made only the kinds of change it made on the
// current Dataset, every column the blocks use is still there and no earlier Dataset goes for room, there is
// nothing new to check: the upload is confirmed at once.
func (a *App) uploadWork(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, file io.Reader, name string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := d.Settings
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
	res, err := a.clean(r, file, d.Template, settings)
	var problem lab.Problem
	if errors.As(err, &problem) {
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Problem: problem.In(lang), Upload: name})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data, err := json.Marshal(res.Dataset)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	found := cleaned{Changes: res.Changes, Guess: res.Guess, Sheets: res.Sheets, Used: res.Used, HeaderRow: res.HeaderRow, Settings: settings, Month: res.Month, Year: res.Year}
	c, err := json.Marshal(found)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	key, err := newDatasetKey()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.datasets.Put(r.Context(), key, data); err != nil {
		a.logError(r, fmt.Errorf("put Dataset %s: %w", key, err))
		a.renderDashboard(w, r, http.StatusServiceUnavailable, &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Failed: true})
		return
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM datasets WHERE dashboard_id = $1 AND state = 'pending'`, d.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `INSERT INTO datasets (dashboard_id, storage_key, file_name, saved_size, row_count, state, cleanup)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6)`, d.ID, key, name, len(data), len(res.Dataset.Rows), c)
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	if d.Config != nil && allFit(d.Config, res.Dataset) { // nothing new to check: it goes live
		usual, err := a.routine(r.Context(), d, res.Changes)
		var room storageRoom
		if err == nil && usual {
			room, err = roomFor(r.Context(), a.db, d, int64(len(data)), acc)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if usual && room.Fits && !room.ForRoom {
			a.confirmWork(w, r, d)
			return
		}
	}
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}

// confirmWork makes the pending Dataset current; the one it replaces becomes earlier, and roomFor's earlier
// Datasets are deleted. On the first confirm the Learner's column choice for each blank opens the Template onto
// the Dashboard as its blocks; the Learner may switch to General instead. Later confirms keep the blocks,
// swapping in the Learner's pick (use:<column>) for each column they use that the Dataset lacks; one left out
// pauses its blocks. A confirm that would take the Learner over their Storage allowance even with every earlier
// Dataset of this Dashboard gone is refused and the Dataset stays pending.
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
	if !d.Work || p == nil {
		http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
		return
	}
	t, c, cfg := d.Template, d.Choices, d.Config
	if cfg == nil {
		if r.FormValue("template") == lab.General.Key {
			t = lab.General
		}
		c = lab.Choices{}
		for _, b := range t.Blanks {
			if v := r.FormValue(b.Key); v != "" {
				c[b.Key] = v
			}
		}
		var problem lab.Problem
		if err := t.Check(c, p.Dataset); errors.As(err, &problem) {
			p.Chosen = c
			page := &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Pending: p}
			page.Error = problem.In(lang)
			a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
			return
		}
		cfg = t.Open(c, p.Dataset)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		cfg, c = withPicks(cfg, c, p.Dataset, r.Form)
	}
	choices, err := json.Marshal(c)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	config, err := json.Marshal(cfg)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings, err := json.Marshal(p.Settings)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var room storageRoom
	overAllowance, replaced := errors.New("over the Storage allowance"), errors.New("pending Dataset replaced")
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Locking the Learner makes two confirms or uploads take turns, as in replaceDataset.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		// Locking the pending Dataset holds off a re-upload replacing it; one that already did leaves this one gone.
		var one int
		err := tx.QueryRowContext(r.Context(), `SELECT 1 FROM datasets WHERE id = $1 AND state = 'pending' FOR UPDATE`, p.ID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return replaced
		}
		if err != nil {
			return err
		}
		if room, err = roomFor(r.Context(), tx, d, p.Size, acc); err != nil || !room.Fits {
			return cmp.Or(err, overAllowance)
		}
		for _, e := range room.Drop {
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM datasets WHERE id = $1`, e.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE datasets SET state = 'earlier' WHERE dashboard_id = $1 AND state = 'current'`, d.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE datasets SET state = 'current', confirmed_at = now() WHERE id = $1`, p.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET template = $2, choices = $3, config = $4, upload_settings = $5, updated_at = now()
			WHERE id = $1`, d.ID, t.Key, choices, config, settings)
		return err
	})
	if errors.Is(err, overAllowance) {
		page := &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Pending: p, Upload: p.File}
		page.Error = text.T(lang, "lab.err.storage-full", sizeText(p.Size), sizeText(room.Used), sizeText(acc.Period.Storage()))
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
		return
	}
	if errors.Is(err, replaced) { // the new upload's preview
		http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dashboardURL(d)+"/preview", http.StatusSeeOther)
}

// restoreDataset makes an earlier Dataset current at once, with no preview: the Dashboard and its Deployment show
// it now, and the one it replaces becomes earlier. The blocks stay as they are. Restore and delete are
// POST /lab/dashboards/{id}/datasets/{ds}/restore and .../delete.
func (a *App) restoreDataset(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("ds"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	gone := errors.New("not an earlier Dataset")
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Two statements, as the one-current index is checked row by row.
		if _, err := tx.ExecContext(r.Context(), `UPDATE datasets SET state = 'earlier' WHERE dashboard_id = $1 AND state = 'current' AND id <> $2`, d.ID, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(r.Context(), `UPDATE datasets SET state = 'current' WHERE id = $1 AND dashboard_id = $2 AND state = 'earlier'`, id, d.ID)
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

// deleteDataset deletes an earlier Dataset and its stored data, freeing its bytes from the Storage allowance.
func (a *App) deleteDataset(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("ds"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM datasets WHERE id = $1 AND dashboard_id = $2 AND state = 'earlier'`, id, d.ID)
	var n int64
	if err == nil {
		n, err = res.RowsAffected()
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if n == 0 {
		a.notFound(w, r)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}
