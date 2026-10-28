package dataset_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
)

// addPending gives the Dashboard a pending Part of kind; replaces is the ID of the Part a ReplacePart one replaces,
// else 0.
func (w *world) addPending(key string, kind dataset.Kind, replaces int64) {
	w.t.Helper()
	var of any
	if replaces != 0 {
		of = replaces
	}
	w.exec(nil, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, part_date, upload_kind, replaces_part_id)
		VALUES ($1, $2, $2 || '.xlsx', 1, 1, 'pending', '2026-10-01', $3, $4)`, w.dashboard, key, string(kind), of)
}

func (w *world) idOf(key string) int64 {
	w.t.Helper()
	var id int64
	w.exec(&id, `SELECT id FROM dataset_parts WHERE storage_key = $1`, key)
	return id
}

// pendingKey is the key of the Dashboard's pending Part, "" without one.
func (w *world) pendingKey() string {
	w.t.Helper()
	var key string
	err := w.db.QueryRow(`SELECT storage_key FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending'`, w.dashboard).Scan(&key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		w.t.Fatal(err)
	}
	return key
}

func (w *world) count(state string) int {
	w.t.Helper()
	var n int
	w.exec(&n, `SELECT count(*) FROM dataset_parts WHERE dashboard_id = $1 AND state = $2`, w.dashboard, state)
	return n
}

// setUp gives the Dashboard the columns and the recipe a first Part sets.
func (w *world) setUp() {
	w.t.Helper()
	w.exec(nil, `UPDATE dashboards SET dataset_columns = '[{"name":"Perkara","type":"text"}]', upload_settings = '{"recipe":{"sheet":"Sheet1"}}' WHERE id = $1`, w.dashboard)
}

func (w *world) delete(key string) error {
	w.t.Helper()
	return w.ds.Delete(context.Background(), w.dashboard, w.idOf(key))
}

func TestDeletingAPartRemovesItAndQueuesItsRowsForDeletionFromTheStore(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	w.addPart("b", "current", 2, 1)
	if err := w.delete("a"); err != nil {
		t.Fatal(err)
	}
	if keys, _ := w.keys(); !slices.Equal(keys, []string{"b"}) {
		t.Fatalf("keys = %v, want [b]", keys)
	}
	var queued []string
	rows, err := w.db.Query(`SELECT storage_key FROM stored_file_deletions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		queued = append(queued, key)
	}
	if !slices.Equal(queued, []string{"a"}) {
		t.Fatalf("queued for deletion = %v, want [a]", queued)
	}
}

func TestDeletingAPartDropsAPendingAdd(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	w.addPart("b", "current", 2, 1)
	w.addPending("more", dataset.Add, 0)
	if err := w.delete("b"); err != nil {
		t.Fatal(err)
	}
	if key := w.pendingKey(); key != "" {
		t.Fatalf("a pending Add %q outlived a deleted Part", key)
	}
}

func TestDeletingAPartKeepsAPendingReplaceOfDataAndDropsAPendingReplaceOfThatPartOnly(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	w.addPart("b", "current", 2, 1)
	w.addPart("c", "current", 3, 1)

	w.addPending("fresh", dataset.ReplaceData, 0)
	if err := w.delete("a"); err != nil {
		t.Fatal(err)
	}
	if key := w.pendingKey(); key != "fresh" {
		t.Fatalf("pending Part = %q, want the Replace of the data kept", key)
	}

	w.exec(nil, `DELETE FROM dataset_parts WHERE state = 'pending'`)
	w.addPending("new-c", dataset.ReplacePart, w.idOf("c"))
	if err := w.delete("b"); err != nil {
		t.Fatal(err)
	}
	if key := w.pendingKey(); key != "new-c" {
		t.Fatalf("pending Part = %q, want the Replace of another Part kept", key)
	}
	if err := w.delete("c"); err != nil {
		t.Fatal(err)
	}
	if key := w.pendingKey(); key != "" {
		t.Fatalf("a pending Replace %q outlived the Part it replaces", key)
	}
}

func TestDeletingAPartDeletesTheEarlierDatasetsFromBeforeParts(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	w.addPart("b", "current", 2, 1)
	w.addPart("old", "earlier", 0, 1)
	if err := w.delete("a"); err != nil {
		t.Fatal(err)
	}
	if n := w.count("earlier"); n != 0 {
		t.Fatalf("%d earlier Datasets after a delete, want 0", n)
	}
}

func TestDeletingTheLastPartClearsTheDatasetsColumnsAndRecipeButNotWhileAPartRemains(t *testing.T) {
	w := newWorld(t)
	w.setUp()
	w.addPart("a", "current", 1, 1)
	w.addPart("b", "current", 2, 1)
	state := func() (columns, recipe bool) {
		var cols string
		w.exec(&cols, `SELECT dataset_columns::text FROM dashboards WHERE id = $1`, w.dashboard)
		w.exec(&recipe, `SELECT upload_settings ? 'recipe' FROM dashboards WHERE id = $1`, w.dashboard)
		return cols != "[]", recipe
	}
	if err := w.delete("a"); err != nil {
		t.Fatal(err)
	}
	if columns, recipe := state(); !columns || !recipe {
		t.Fatalf("columns kept = %v, recipe kept = %v after deleting one of two Parts, want both kept", columns, recipe)
	}
	if err := w.delete("b"); err != nil {
		t.Fatal(err)
	}
	if columns, recipe := state(); columns || recipe {
		t.Fatalf("columns kept = %v, recipe kept = %v after deleting the last Part, want neither", columns, recipe)
	}
}

func TestDeletingAPartTwiceSaysItIsGone(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	id := w.idOf("a")
	if err := w.ds.Delete(context.Background(), w.dashboard, id); err != nil {
		t.Fatal(err)
	}
	if err := w.ds.Delete(context.Background(), w.dashboard, id); !errors.Is(err, dataset.ErrPartGone) {
		t.Fatalf("second delete = %v, want ErrPartGone", err)
	}
}

func TestADeleteOnlyReachesThePartsOfTheDashboardItNames(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	var learner, other int64
	w.exec(&learner, `SELECT learner_id FROM dashboards WHERE id = $1`, w.dashboard)
	w.exec(&other, `INSERT INTO dashboards (learner_id, name) VALUES ($1, 'Lain') RETURNING id`, learner)
	if err := w.ds.Delete(context.Background(), other, w.idOf("a")); !errors.Is(err, dataset.ErrPartGone) {
		t.Fatalf("delete through another Dashboard = %v, want ErrPartGone", err)
	}
	if keys, _ := w.keys(); !slices.Equal(keys, []string{"a"}) {
		t.Fatalf("keys = %v, want [a] kept", keys)
	}
}
