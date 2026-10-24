package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
}

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
	var changes, guess []byte
	err := a.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, changes, guess FROM datasets
		WHERE dashboard_id = $1 AND state = 'pending'`, d.ID).Scan(&p.ID, &key, &p.File, &p.Size, &changes, &guess)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := errors.Join(json.Unmarshal(changes, &p.Changes), json.Unmarshal(guess, &p.Guess)); err != nil {
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
// any earlier pending one, then shows the preview. A file Clean-up cannot read gets one message.
func (a *App) uploadWork(w http.ResponseWriter, r *http.Request, d *dashboard, acc labAccess, file io.Reader, name string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// A header row that is not a number lets Cave find it, as when left empty.
	header, _ := strconv.Atoi(r.FormValue("header"))
	res, err := lab.Clean(file, d.Template, lab.Settings{HeaderRow: max(header, 0), TwoRowHeader: r.FormValue("two-row") == "on"})
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
		_, err := tx.ExecContext(r.Context(), `INSERT INTO datasets (dashboard_id, storage_key, file_name, saved_size, row_count, state, changes, guess)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)`, d.ID, key, name, len(data), len(res.Dataset.Rows), changes, guess)
		return err
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
}

// confirmWork makes the pending Dataset current; the one it replaces is deleted. On the first confirm the
// Learner's column choice for each blank opens the Template onto the Dashboard as its blocks; the Learner may
// switch to General instead. Later confirms leave the blocks as they are. A confirm that would take the Learner
// over their Storage allowance is refused and the Dataset stays pending.
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
		if period := acc.CountedPeriod(); period != nil {
			if err := tx.QueryRowContext(r.Context(), `SELECT `+storageUsedSQL("this.learner_id")+` FROM dashboards this WHERE this.id = $1`, d.ID).Scan(&used); err != nil {
				return err
			}
			if used-d.Size+p.Size > period.Storage() {
				return full
			}
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM datasets WHERE dashboard_id = $1 AND state = 'current'`, d.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE datasets SET state = 'current', confirmed_at = now() WHERE id = $1`, p.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `UPDATE dashboards SET template = $2, choices = $3, config = $4, updated_at = now() WHERE id = $1`,
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
