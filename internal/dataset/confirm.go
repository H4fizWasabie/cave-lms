package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// ErrLostRace is Confirm's error when it lost a race: the Dashboard's pending Part was replaced by a newer upload, or
// already confirmed, or the Part it replaces is gone. Nothing was changed; the newer preview is the one to show.
var ErrLostRace = errors.New("pending Part replaced or Part gone")

// ErrPartDate is Confirm's error for a date the Learner chose that is not a date from the year 1900 to 2100.
var ErrPartDate = errors.New("Part date out of range")

// Confirmed is what a confirm made.
type Confirmed struct {
	ID      int64        // the Part that was pending
	Key     string       // where its rows are stored: a new key when the Part date was changed
	Columns []lab.Column // the Dataset's columns now
	// Dataset holds the Part's rows in the Dataset's columns, without the Part date. The Dataset's other Parts stay in
	// the store; for a Part that is the whole Dataset, it is the Dataset.
	Dataset *lab.Dataset
}

// ConfirmReserve checks that a confirm fits the Learner's Storage allowance, in the transaction tx that makes the Part, and
// returns the error that refuses it. p is the pending Part, whose size it adds; Frees names what the confirm deletes,
// which the transaction does not yet. Confirm calls it first in the transaction, before any row lock, so a reserve that
// locks the Learner always takes that lock first (#463).
type ConfirmReserve func(ctx context.Context, tx *sql.Tx, p *Pending) error

// Hook is called in Confirm's transaction, last, with what the confirm made; the error it returns rolls the confirm
// back. The app builds or updates a Cave-built Dashboard's blocks with it.
type Hook func(ctx context.Context, tx *sql.Tx, c *Confirmed) error

// Queryer is a *sql.DB or a *sql.Tx, for reads that may run inside a caller's transaction.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Freed is a Part a confirm deletes.
type Freed struct {
	ID   int64
	Size int64 // saved size, in bytes
}

// freedBy is the Parts a confirm of kind deletes (replaces, 0 for none, is the Part a ReplacePart one replaces): every
// current Part for Replace data, the one Part for a Replace of a Part, none for an Add, and for all three the earlier
// Datasets from before Parts. $1 is the Dashboard, $2 whether it is Replace data and $3 the Part replaced. Confirm
// deletes by it and Frees lists by it, so they cannot differ.
const freedBy = `dashboard_id = $1 AND (state = 'earlier' OR state = 'current' AND ($2::boolean OR id = $3))`

// Frees lists the Parts a confirm of a pending Part of kind deletes (see freedBy). The Storage allowance counts them as free.
func Frees(ctx context.Context, q Queryer, dashboardID int64, kind Kind, replaces int64) ([]Freed, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, saved_size FROM dataset_parts WHERE `+freedBy+` ORDER BY id`, dashboardID, kind == ReplaceData, replaces)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Freed
	for rows.Next() {
		var f Freed
		if err := rows.Scan(&f.ID, &f.Size); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Pending loads the Dashboard's pending Part with its rows from the store; nil if it has none. A store that cannot give
// them is ErrStore.
func (d *Dataset) Pending(ctx context.Context, dashboardID int64) (*Pending, error) {
	var p Pending
	var c []byte
	var replaces sql.NullInt64
	err := d.db.QueryRowContext(ctx, `SELECT id, storage_key, file_name, saved_size, cleanup, COALESCE(workbook_id, 0), upload_kind, replaces_part_id FROM dataset_parts
		WHERE dashboard_id = $1 AND state = 'pending'`, dashboardID).Scan(&p.ID, &p.Key, &p.File, &p.Size, &c, &p.Workbook, &p.Kind, &replaces)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.ReplacedPart = replaces.Int64
	if err := json.Unmarshal(c, &p.Preview); err != nil {
		return nil, err
	}
	data, err := d.store.Get(ctx, p.Key)
	if err != nil {
		return nil, fmt.Errorf("%w: get Part %s: %v", ErrStore, p.Key, err)
	}
	if p.Dataset, err = lab.PartDataset(data, p.Columns); err != nil {
		return nil, fmt.Errorf("read Part %s: %w", p.Key, err)
	}
	return &p, nil
}

// otherRows is the rows the Dataset keeps besides the Part once it is confirmed: 0 for a Part that is the whole Dataset.
func (p *Pending) otherRows() int {
	switch {
	case p.Kind == Add && p.Added != nil:
		return p.Added.Had
	case p.Kind == ReplacePart && p.Preview.Replaces != nil:
		return p.Preview.Replaces.Had
	}
	return 0
}

// Confirm makes the Dashboard's pending Part a Part, in one transaction, and returns it with the Dataset's columns. An
// Add puts it last in the Dataset's order; a Replace of one Part takes the place of that Part, which goes; Replace data
// makes it the only Part. The Datasets from before Parts go in each case. The stored rows of what is deleted are queued
// for deletion from the store by the database.
//
// date is the Part date the Learner chose, "" to keep the preview's. One outside 1900 to 2100, or not a date, is
// ErrPartDate. A changed date writes the Part's rows again under a new key and switches the Part to it, so a confirm that
// fails leaves the Part's rows as they were; the old rows are queued for deletion.
//
// The Dataset's columns and recipe are settled as ADR 0022 says. The upload's recipe is the Dashboard's from now on: a
// first Part and Replace data set it, with the picks cleared, and an Add or a Replace of one Part keeps it with its
// picks. A month column holding a number outside 1 to 12 once this Part is in is no longer one; a Part that is all the
// Dataset has decides them by its own values, as a first Part does. (A Dataset from before month columns had them found
// when the Part was uploaded.)
//
// reserve is called first in the transaction, before any row lock; its error refuses the confirm, which changes nothing.
// hook, if not nil, is called last, still in the transaction. ErrLostRace says the pending Part is not there to
// confirm. A store that refuses the rewritten rows is ErrStore.
func (d *Dataset) Confirm(ctx context.Context, dashboardID int64, date string, reserve ConfirmReserve, hook Hook) (*Confirmed, error) {
	p, err := d.Pending(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrLostRace
	}
	day := p.PartDate
	if v := strings.TrimSpace(date); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil || t.Year() < 1900 || t.Year() > 2100 {
			return nil, ErrPartDate
		}
		day = t.Format(time.DateOnly)
	}

	if recipe := p.Settings.Recipe; p.Kind != ReplaceData && recipe != nil {
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
	if p.Kind == ReplaceData { // the first Part, and Replace data, set the recipe every later Part is read with
		// empty, not nil: found, and none
		p.Settings.Recipe = &lab.Recipe{Sheet: p.PickedSheet, HeaderRow: p.HeaderRow, HeaderTyped: p.HeaderTyped, MonthColumns: append([]string{}, p.MonthColumns...)}
	}
	settings, err := json.Marshal(p.Settings)
	if err != nil {
		return nil, err
	}
	columns, err := json.Marshal(p.Columns)
	if err != nil {
		return nil, err
	}

	// A changed date rewrites the Part's rows (the same length, so the saved size and the allowance check stand). They go
	// under a new key first and the confirm switches the Part to it.
	oldKey := p.Key
	if day != p.PartDate {
		rows, err := lab.PartRows(p.Dataset, day)
		var key string
		if err == nil {
			key, err = NewKey()
		}
		if err == nil {
			if err = d.store.Put(ctx, key, rows); err != nil {
				err = fmt.Errorf("%w: put Part %s: %v", ErrStore, key, err)
			}
		}
		if err != nil {
			return nil, err
		}
		p.Key, p.PartDate, p.DateFrom, p.DateCell = key, day, lab.DateFromChosen, ""
	}
	c, err := d.confirm(ctx, dashboardID, p, oldKey, settings, columns, reserve, hook)
	if err != nil && p.Key != oldKey {
		// A failed delete leaves only an unused object.
		// ponytail: not logged, as the module has no logger; the object is lost until the store is swept.
		_ = d.store.Delete(context.WithoutCancel(ctx), p.Key)
	}
	return c, err
}

// confirm is Confirm's transaction. p.Key differs from oldKey when the Part date was changed.
func (d *Dataset) confirm(ctx context.Context, dashboardID int64, p *Pending, oldKey string, settings, columns []byte, reserve ConfirmReserve, hook Hook) (*Confirmed, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// The reserve comes first, before any row is locked: it may lock the Learner, who is always locked first (#463).
	if err := reserve(ctx, tx, p); err != nil {
		return nil, err
	}
	// Locking the Learner makes confirms, uploads and deletes take turns.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, dashboardID); err != nil {
		return nil, err
	}
	// Locking the pending Part holds off a re-upload replacing it; one that already did leaves this one gone.
	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM dataset_parts WHERE id = $1 AND state = 'pending' FOR UPDATE`, p.ID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLostRace
	}
	if err != nil {
		return nil, err
	}
	// A Replace of one Part takes the replaced Part's place; the Part goes if another confirm or delete has not already.
	var position sql.NullInt64
	if p.Kind == ReplacePart {
		err := tx.QueryRowContext(ctx, `SELECT position FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current' FOR UPDATE`,
			p.ReplacedPart, dashboardID).Scan(&position)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLostRace
		}
		if err != nil {
			return nil, err
		}
	}
	if p.Key != oldKey {
		cleanup, err := json.Marshal(p.Preview)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE dataset_parts SET storage_key = $2, part_date = $3, date_source = $4, cleanup = $5 WHERE id = $1`,
			p.ID, p.Key, p.PartDate, p.DateFrom, cleanup); err != nil {
			return nil, err
		}
		// The trigger queues the object of a deleted record only, so the old object is queued here.
		if _, err := tx.ExecContext(ctx, `INSERT INTO stored_file_deletions (storage_key) VALUES ($1) ON CONFLICT DO NOTHING`, oldKey); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dataset_parts WHERE `+freedBy, dashboardID, p.Kind == ReplaceData, p.ReplacedPart); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dataset_parts SET state = 'current', confirmed_at = now(),
		position = COALESCE($3::integer, (SELECT COALESCE(max(position), 0) + 1 FROM dataset_parts WHERE dashboard_id = $2 AND state = 'current'))
		WHERE id = $1`, p.ID, dashboardID, position); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboards SET upload_settings = $2, dataset_columns = $3, updated_at = now() WHERE id = $1`,
		dashboardID, settings, columns); err != nil {
		return nil, err
	}
	c := &Confirmed{ID: p.ID, Key: p.Key, Columns: slices.Clone(p.Columns), Dataset: p.Dataset}
	if hook != nil {
		if err := hook(ctx, tx, c); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}

// Routine reports whether Clean-up's changes are all of kinds it also made on the Dashboard's latest Part, as when the
// same monthly workbook is skipped past its title rows and combined again. With no Part, it is whether there are no changes.
func (d *Dataset) Routine(ctx context.Context, dashboardID int64, changes []lab.Change) (bool, error) {
	var c []byte
	err := d.db.QueryRowContext(ctx, `SELECT cleanup FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current'
		ORDER BY position DESC LIMIT 1`, dashboardID).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return len(changes) == 0, nil
	}
	var last Preview
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
