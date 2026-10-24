package app

import (
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

// pendingDataset is a Work Dashboard's upload after Clean-up, waiting for its Learner to confirm the preview.
// Until then the Dashboard and its Deployment keep the current Dataset.
type pendingDataset struct {
	ID      int64
	File    string
	Size    int64 // saved size, which counts against the Storage allowance once confirmed
	Dataset *lab.Dataset
	Changes []lab.Change
	Guess   lab.Choices
	Chosen  lab.Choices // the Learner's choices on a refused confirm; else Guess
	Sheets  []lab.Sheet // every sheet of the upload
	Used    []string    // the sheets that went into the Dataset
	Header  int         // the Excel row the column names came from
}

// Picked is the sheet Clean-up read first, the one the others joined.
func (p *pendingDataset) Picked() string {
	for _, s := range p.Sheets {
		if s.Readable && !s.CanCombine && s.Reason == "" {
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

// earlierDataset is a Work Dashboard's Dataset that is not pending: an earlier one the Learner may restore or
// delete, or the current one.
type earlierDataset struct {
	ID        int64
	File      string
	Size      int64 // saved size
	Rows      int
	Confirmed string // formatted as periodTime
}

func (e earlierDataset) SizeText() string { return sizeText(e.Size) }

// queryer is a *sql.DB or a *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// keptDatasets lists the Work Dashboard's earlier Datasets, oldest confirmed first, then its current one, if any.
func keptDatasets(ctx context.Context, q queryer, dashboardID int64) ([]earlierDataset, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, file_name, saved_size, row_count, confirmed_at FROM datasets
		WHERE dashboard_id = $1 AND state <> 'pending' ORDER BY state = 'current', confirmed_at, id`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []earlierDataset
	for rows.Next() {
		var e earlierDataset
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
func (a *App) earlierOf(ctx context.Context, d *dashboard) ([]earlierDataset, error) {
	kept, err := keptDatasets(ctx, a.db, d.ID)
	if err != nil || d.Key == "" { // no current Dataset, so no earlier ones
		return nil, err
	}
	kept = kept[:len(kept)-1] // the current one
	slices.Reverse(kept)
	return kept, nil
}

// toDrop is what confirming a pending Dataset of size bytes deletes, the current Dataset turning earlier: the
// earlier ones past keepEarlier, then, oldest first, as many more as the Storage allowance needs. The Admin has no
// allowance. fits is false when the Dataset does not fit even with every earlier one gone; used is the bytes the
// Learner's Datasets take now.
func toDrop(ctx context.Context, q queryer, d *dashboard, size int64, acc labAccess) (drop []earlierDataset, used int64, fits bool, err error) {
	kept, err := keptDatasets(ctx, q, d.ID)
	if err != nil {
		return nil, 0, false, err
	}
	n := max(len(kept)-keepEarlier, 0)
	period := acc.CountedPeriod()
	if period == nil {
		return kept[:n], 0, true, nil
	}
	if err := q.QueryRowContext(ctx, `SELECT `+storageUsedSQL("this.learner_id")+` FROM dashboards this WHERE this.id = $1`, d.ID).Scan(&used); err != nil {
		return nil, 0, false, err
	}
	after := used + size
	for _, e := range kept[:n] {
		after -= e.Size
	}
	for ; after > period.Storage() && n < len(kept); n++ {
		after -= kept[n].Size
	}
	return kept[:n], used, after <= period.Storage(), nil
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
	var changes, guess, sheets, used []byte
	err := a.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, changes, guess, sheets, used, header_row FROM datasets
		WHERE dashboard_id = $1 AND state = 'pending'`, d.ID).Scan(&p.ID, &key, &p.File, &p.Size, &changes, &guess, &sheets, &used, &p.Header)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := errors.Join(json.Unmarshal(changes, &p.Changes), json.Unmarshal(guess, &p.Guess),
		json.Unmarshal(sheets, &p.Sheets), json.Unmarshal(used, &p.Used)); err != nil {
		return nil, err
	}
	data, err := a.datasets.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get Dataset %s: %w", key, err)
	}
	p.Chosen = p.Guess
	return &p, json.Unmarshal(data, &p.Dataset)
}

// uploadWork runs Clean-up on a Work Dashboard's upload and keeps the result as its pending Dataset, replacing
// any earlier pending one, then shows the preview. A file Clean-up cannot read gets one message. Clean-up reads
// with the Dashboard's remembered settings, unless the upload comes with the preview's sheet choices, which
// the Dashboard then remembers. When Clean-up changed nothing and every column the blocks use is still there,
// there is nothing to check: the upload is confirmed at once.
func (a *App) uploadWork(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, file io.Reader, name string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	settings := d.Settings
	if sheet := r.FormValue("sheet"); sheet != "" {
		settings.Sheet, settings.Alone, settings.Ticked = sheet, false, nil
		// Ticks were made against the sheet the preview showed picked; for another sheet, every match joins.
		if sheet == r.FormValue("shown") && r.FormValue("tickable") != "" {
			settings.Ticked = append([]string{}, r.Form["combine"]...)
			settings.Alone = len(settings.Ticked) == 0
		}
		remember, err := json.Marshal(settings)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		// ponytail: remembered on upload, not confirm; an abandoned preview still sets the next upload's choices.
		if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET upload_settings = $2 WHERE id = $1`, d.ID, remember); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	res, err := lab.Clean(file, d.Template, settings)
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
	changes, err := json.Marshal(res.Changes)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	guess, err := json.Marshal(res.Guess)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	sheets, err := json.Marshal(res.Sheets)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	used, err := json.Marshal(res.Used)
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
		_, err := tx.ExecContext(r.Context(), `INSERT INTO datasets (dashboard_id, storage_key, file_name, saved_size, row_count, state, changes, guess, sheets, used, header_row)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9, $10)`, d.ID, key, name, len(data), len(res.Dataset.Rows), changes, guess, sheets, used, res.HeaderRow)
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	if d.Config != nil && len(res.Changes) == 0 && fits(d.Config, res.Dataset) { // nothing to check: it goes live
		quiet, err := a.dropsOnlyPastKeep(r.Context(), d, int64(len(data)), acc)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if quiet {
			a.confirmWork(w, r, d)
			return
		}
	}
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}

// dropsOnlyPastKeep is whether confirming a Dataset of size fits the Storage allowance deleting no earlier Dataset
// but those past keepEarlier, so the preview has nothing to warn about.
func (a *App) dropsOnlyPastKeep(ctx context.Context, d *dashboard, size int64, acc labAccess) (bool, error) {
	kept, err := keptDatasets(ctx, a.db, d.ID)
	if err != nil {
		return false, err
	}
	drop, _, ok, err := toDrop(ctx, a.db, d, size, acc)
	return ok && len(drop) <= max(len(kept)-keepEarlier, 0), err
}

// confirmWork makes the pending Dataset current; the one it replaces becomes earlier, and toDrop's earlier
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
		r.ParseForm()
		cfg = picked(cfg, p.Dataset, r.Form)
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
	var used int64
	full := errors.New("full")
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Locking the Learner makes two confirms or uploads take turns, as in replaceDataset.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		drop, u, fits, err := toDrop(r.Context(), tx, d, p.Size, acc)
		if used = u; err != nil || !fits {
			return cmp.Or(err, full)
		}
		for _, e := range drop {
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
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET template = $2, choices = $3, config = $4, updated_at = now() WHERE id = $1`,
			d.ID, t.Key, choices, config)
		return err
	})
	if errors.Is(err, full) {
		page := &dashboardPage{Dashboard: d, Tab: "dataset", Access: acc, Pending: p, Upload: p.File}
		page.Error = text.T(lang, "lab.err.storage-full", sizeText(p.Size), sizeText(used), sizeText(acc.Period.Storage()))
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, page)
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
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM datasets WHERE id = $1 AND dashboard_id = $2 AND state = 'earlier'`, id, d.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}
