// Package dataset is the Dataset module: it owns how a Dashboard's Dataset, its Parts, is read and changed (ADR 0022).
// It knows nothing of HTTP, text, Themes or blocks.
package dataset

import (
	"context"
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
)

// Dataset is the Dataset module over the database and the store the Parts' rows live in.
type Dataset struct {
	db    *sql.DB
	store storage.Store
	now   func() time.Time // the clock, which dates a Part that names no date
}

// New is the Dataset module over db and store, dating Parts by the clock now.
func New(db *sql.DB, store storage.Store, now func() time.Time) *Dataset {
	return &Dataset{db: db, store: store, now: now}
}

// Kind is what an upload does to the Dataset once confirmed (ADR 0022), kept on its Part.
type Kind string

const (
	ReplaceData Kind = "replace-data" // every current Part is deleted and the upload is the Dataset
	Add         Kind = "add"          // the upload becomes the last Part and the others stay
	ReplacePart Kind = "replace-part" // the upload takes the place of one Part and the others stay
)

// ErrPartGone is Delete's error for a Part that is not a current Part of the Dashboard: another delete or confirm
// already took it.
var ErrPartGone = errors.New("Part gone")

// Part is a current Part as Parts lists it.
type Part struct {
	ID        int64
	Key       string // where its rows are stored
	File      string
	Date      string // the Part date
	Rows      int
	Size      int64     // saved size, in bytes
	Confirmed time.Time // when it became a Part
}

// Parts lists the Dashboard's current Parts in position order.
func (d *Dataset) Parts(ctx context.Context, dashboardID int64) ([]Part, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, storage_key, file_name, part_date::text, row_count, saved_size, confirmed_at
		FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current' ORDER BY position`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []Part
	for rows.Next() {
		var p Part
		var confirmed sql.NullTime
		if err := rows.Scan(&p.ID, &p.Key, &p.File, &p.Date, &p.Rows, &p.Size, &confirmed); err != nil {
			return nil, err
		}
		p.Confirmed = confirmed.Time
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

// Keys lists the Dashboard's current Parts in position order and gives the dataset key, which names the Parts the
// Dataset is made of so it changes whenever they do; the key is empty without Parts.
func (d *Dataset) Keys(ctx context.Context, dashboardID int64) (parts []Part, key string, err error) {
	if parts, err = d.Parts(ctx, dashboardID); err != nil {
		return nil, "", err
	}
	keys := make([]string, len(parts))
	for i, p := range parts {
		keys[i] = p.Key
	}
	return parts, datasetKey(keys), nil
}

// Settings is what the Dashboard remembers for its next upload (its recipe once the Dataset has Parts) and the Dataset's
// columns: nil until the first upload, the empty list once the last Part is deleted.
func (d *Dataset) Settings(ctx context.Context, dashboardID int64) (lab.Settings, []lab.Column, error) {
	st, err := d.load(ctx, dashboardID)
	if err != nil {
		return lab.Settings{}, nil, err
	}
	return st.settings, st.columns, nil
}

// Use is a Dashboard that has a Part cleaned from a Workbook.
type Use struct {
	ID   int64 // the Dashboard
	Name string
}

// Uses lists, for each of the Learner's Workbooks that has Parts, the Learner's Dashboards that do, once each, by name.
func (d *Dataset) Uses(ctx context.Context, learnerID int64) (map[int64][]Use, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT ds.workbook_id, d.id, d.name FROM dataset_parts ds
		JOIN dashboards d ON d.id = ds.dashboard_id WHERE d.learner_id = $1 AND ds.workbook_id IS NOT NULL
		GROUP BY ds.workbook_id, d.id, d.name ORDER BY lower(d.name), d.id`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	uses := map[int64][]Use{}
	for rows.Next() {
		var workbook int64
		var u Use
		if err := rows.Scan(&workbook, &u.ID, &u.Name); err != nil {
			return nil, err
		}
		uses[workbook] = append(uses[workbook], u)
	}
	return uses, rows.Err()
}

// Prune deletes every Part of the Learners learnerIDs, whatever its state, and every pending Part left past pendingHours
// hours, and returns how many Parts it deleted. The database queues their stored rows for deletion from the store.
func (d *Dataset) Prune(ctx context.Context, learnerIDs []int64, pendingHours int) (int, error) {
	res, err := d.db.ExecContext(ctx, `DELETE FROM dataset_parts ds USING dashboards d WHERE d.id = ds.dashboard_id
		AND (ds.state = 'pending' AND ds.uploaded_at < now() - make_interval(hours => $2) OR d.learner_id = ANY($1))`,
		learnerIDs, pendingHours)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// datasetKey names the Parts stored under keys, in order: the md5 in hex of the keys joined by commas; empty without
// Parts.
func datasetKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(keys, ","))))
}

// Delete removes the Dashboard's current Part partID, and with it the Datasets from before Parts and the pending Part
// that depended on it: an Add, which was put together from the Parts as they were, or a Replace of this Part. A Replace
// of the data, or of another Part, stays. With no Part left the Dataset's columns are cleared to the empty list (NULL
// is a Dataset never uploaded, which data.json answers 404) and so is its recipe, so the next upload is a first Part.
// The stored rows of what it deletes are queued for deletion from the store by the database. It returns ErrPartGone
// for a Part already deleted.
func (d *Dataset) Delete(ctx context.Context, dashboardID, partID int64) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Locking the Learner makes deletes, confirms and uploads take turns.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, dashboardID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`, partID, dashboardID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrPartGone
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND (state = 'earlier'
			OR state = 'pending' AND (upload_kind = $3 OR upload_kind = $4 AND replaces_part_id = $2))`,
		dashboardID, partID, Add, ReplacePart); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboards SET updated_at = now(),
			dataset_columns = CASE WHEN left_over.none THEN '[]' ELSE dataset_columns END,
			upload_settings = CASE WHEN left_over.none THEN upload_settings - 'recipe' ELSE upload_settings END
		FROM (SELECT NOT EXISTS (SELECT 1 FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current') AS none) left_over
		WHERE id = $1`, dashboardID); err != nil {
		return err
	}
	return tx.Commit()
}
