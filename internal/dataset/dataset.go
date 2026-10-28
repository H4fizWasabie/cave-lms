// Package dataset is the Dataset module: it owns how a Dashboard's Dataset, its Parts, is read and changed (ADR 0022).
// It knows nothing of HTTP, text, Themes or blocks.
package dataset

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/storage"
)

// Dataset is the Dataset module over the database and the store the Parts' rows live in.
type Dataset struct {
	db    *sql.DB
	store storage.Store
}

// New is the Dataset module over db and store.
func New(db *sql.DB, store storage.Store) *Dataset {
	return &Dataset{db: db, store: store}
}

// Part is a current Part as Keys lists it.
type Part struct {
	ID      int64
	Key     string // where its rows are stored
	HasRows bool
}

// Keys lists the Dashboard's current Parts in position order and gives the dataset key, which names the Parts the
// Dataset is made of so it changes whenever they do; the key is empty without Parts.
func (d *Dataset) Keys(ctx context.Context, dashboardID int64) (parts []Part, key string, err error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, storage_key, row_count > 0 FROM dataset_parts
		WHERE dashboard_id = $1 AND state = 'current' ORDER BY position`, dashboardID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.ID, &p.Key, &p.HasRows); err != nil {
			return nil, "", err
		}
		parts = append(parts, p)
		keys = append(keys, p.Key)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return parts, datasetKey(keys), nil
}

// datasetKey names the Parts stored under keys, in order: the md5 in hex of the keys joined by commas; empty without
// Parts.
func datasetKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(keys, ","))))
}
