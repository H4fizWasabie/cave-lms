package dataset_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// world is a Dashboard in a migrated test database with the Dataset module over it and a store in a temp directory. The
// helpers below make and change Parts with SQL for the tests of Keys and Delete, which need Parts the module's own calls
// do not make.
type world struct {
	t         *testing.T
	db        *sql.DB
	dir       string // where the store keeps the objects
	store     storage.Store
	learner   int64
	dashboard int64
	ds        *dataset.Dataset
	now       time.Time // the clock; tests may set it before an upload
}

func newWorld(t *testing.T) *world {
	t.Helper()
	db, _ := testutil.NewDatabase(t)
	dir := t.TempDir()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, db: db, dir: dir, store: store, now: uploadAt}
	w.ds = dataset.New(db, store, func() time.Time { return w.now })
	w.exec(&w.learner, `INSERT INTO learners (name, username, email, password_hash) VALUES ('Ada', 'ada', 'ada@example.invalid', 'x') RETURNING id`)
	w.exec(&w.dashboard, `INSERT INTO dashboards (learner_id, name) VALUES ($1, 'Kedai') RETURNING id`, w.learner)
	return w
}

func (w *world) exec(into any, query string, args ...any) {
	w.t.Helper()
	var err error
	if into == nil {
		_, err = w.db.Exec(query, args...)
	} else {
		err = w.db.QueryRow(query, args...).Scan(into)
	}
	if err != nil {
		w.t.Fatal(err)
	}
}

// addPart gives the Dashboard a Part in state ('current', 'pending' or 'earlier') that stores its rows under key; a
// current one is at position.
func (w *world) addPart(key, state string, position, rows int) {
	w.t.Helper()
	var pos any
	if state == "current" {
		pos = position
	}
	w.exec(nil, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date)
		VALUES ($1, $2, $2 || '.xlsx', 1, $3, $4, $5, '2026-10-01')`, w.dashboard, key, rows, state, pos)
}

// replacePart makes the Part stored under old store its rows under key instead.
func (w *world) replacePart(old, key string) {
	w.t.Helper()
	w.exec(nil, `UPDATE dataset_parts SET storage_key = $2 WHERE storage_key = $1`, old, key)
}

func (w *world) deletePart(key string) {
	w.t.Helper()
	w.exec(nil, `DELETE FROM dataset_parts WHERE storage_key = $1`, key)
}

func (w *world) parts() ([]dataset.Part, string) {
	w.t.Helper()
	parts, key, err := w.ds.Keys(context.Background(), w.dashboard)
	if err != nil {
		w.t.Fatal(err)
	}
	return parts, key
}

func (w *world) keys() ([]string, string) {
	w.t.Helper()
	parts, key := w.parts()
	var keys []string
	for _, p := range parts {
		keys = append(keys, p.Key)
	}
	return keys, key
}

func TestAnEmptyDatasetHasNoPartsAndAnEmptyKey(t *testing.T) {
	w := newWorld(t)
	keys, key := w.keys()
	if len(keys) != 0 || key != "" {
		t.Fatalf("Keys = %v, %q, want none and an empty key", keys, key)
	}
}

func TestKeysListsPartsInPositionOrderAndNamesThemByTheirKeys(t *testing.T) {
	w := newWorld(t)
	w.addPart("b", "current", 2, 5)
	w.addPart("a", "current", 1, 5)
	keys, key := w.keys()
	if !slices.Equal(keys, []string{"a", "b"}) {
		t.Fatalf("keys = %v, want [a b]", keys)
	}
	// The md5 of "a,b" in hex, as the Deployment's files were cached under before the module made the key.
	if want := "b345e1dc09f20fdefdea469f09167892"; key != want {
		t.Fatalf("dataset key = %q, want %q", key, want)
	}
}

func TestKeysTellsWhichPartsHaveRows(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 0)
	w.addPart("b", "current", 2, 3)
	parts, _ := w.parts()
	if len(parts) != 2 || parts[0].HasRows || !parts[1].HasRows {
		t.Fatalf("parts = %+v, want the first without rows and the second with", parts)
	}
}

func TestTheDatasetKeyChangesWhenAPartIsAddedReplacedOrDeleted(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	_, one := w.keys()

	w.addPart("b", "current", 2, 1)
	_, added := w.keys()

	w.replacePart("b", "b2")
	_, replaced := w.keys()

	w.deletePart("a")
	_, deleted := w.keys()

	seen := map[string]bool{}
	for name, key := range map[string]string{"one Part": one, "added": added, "replaced": replaced, "deleted": deleted} {
		if key == "" || seen[key] {
			t.Fatalf("the key after %s is %q, which is empty or one an earlier change gave", name, key)
		}
		seen[key] = true
	}
}

func TestPendingAndEarlierPartsAreNotInTheDataset(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 1)
	w.addPart("waiting", "pending", 0, 1)
	w.addPart("old", "earlier", 0, 1)
	keys, _ := w.keys()
	if !slices.Equal(keys, []string{"a"}) {
		t.Fatalf("keys = %v, want [a]", keys)
	}
}
