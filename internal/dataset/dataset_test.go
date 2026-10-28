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

func TestPartsTellHowManyRowsEachHasAndWhatTheDataTabShowsOfThem(t *testing.T) {
	w := newWorld(t)
	w.addPart("a", "current", 1, 0)
	w.addPart("b", "current", 2, 3)
	parts, _ := w.parts()
	if len(parts) != 2 || parts[0].Rows != 0 || parts[1].Rows != 3 {
		t.Fatalf("parts = %+v, want the first without rows and the second with 3", parts)
	}
	if b := parts[1]; b.File != "b.xlsx" || b.Date != "2026-10-01" || b.Size != 1 {
		t.Fatalf("part b = %+v, want its file, Part date and saved size", b)
	}
}

func TestAConfirmedPartIsListedWithItsFileDateRowsSizeAndWhenItWasConfirmed(t *testing.T) {
	w := newWorld(t)
	before := time.Now().Add(-time.Minute)
	w.junePart()
	parts, _ := w.parts()
	if len(parts) != 1 {
		t.Fatalf("parts = %+v, want one", parts)
	}
	p := parts[0]
	if p.File != "jun.xlsx" || p.Date != uploadDay || p.Rows != 2 || p.Size <= 0 || p.Confirmed.Before(before) {
		t.Fatalf("part = %+v, want jun.xlsx, %s, 2 rows, a size and a confirm time of now", p, uploadDay)
	}
}

func TestSettingsGiveWhatTheDashboardRemembersAndItsColumnsNilUntilTheFirstUploadAndEmptyOnceTheLastPartIsDeleted(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	settings, columns, err := w.ds.Settings(ctx, w.dashboard)
	if err != nil || settings.Recipe != nil || columns != nil {
		t.Fatalf("before an upload: %+v, %v, %v, want no recipe and nil columns", settings, columns, err)
	}

	w.junePart()
	settings, columns, err = w.ds.Settings(ctx, w.dashboard)
	if err != nil || settings.Recipe == nil || settings.Recipe.Sheet == "" || len(columns) != 3 {
		t.Fatalf("after a first Part: %+v, %v, %v, want its recipe and 3 columns", settings, columns, err)
	}

	parts, _ := w.parts()
	if err := w.ds.Delete(ctx, w.dashboard, parts[0].ID); err != nil {
		t.Fatal(err)
	}
	settings, columns, err = w.ds.Settings(ctx, w.dashboard)
	if err != nil || settings.Recipe != nil || columns == nil || len(columns) != 0 {
		t.Fatalf("after deleting the last Part: %+v, %#v, %v, want no recipe and an empty, not nil, list", settings, columns, err)
	}
}

func TestUsesListsOnceEachDashboardThatHasAPartCleanedFromAWorkbookByNameForTheLearnersWorkbooksOnly(t *testing.T) {
	w := newWorld(t)
	var workbook, other, second, stranger, strangersDashboard int64
	w.exec(&workbook, `INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, 'wb1', 'a.xlsx', 1, '\x01') RETURNING id`, w.learner)
	w.exec(&other, `INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, 'wb2', 'b.xlsx', 1, '\x02') RETURNING id`, w.learner)
	w.exec(&second, `INSERT INTO dashboards (learner_id, name) VALUES ($1, 'akaun') RETURNING id`, w.learner)
	w.exec(&stranger, `INSERT INTO learners (name, username, email, password_hash) VALUES ('Bob', 'bob', 'bob@example.invalid', 'x') RETURNING id`)
	w.exec(&strangersDashboard, `INSERT INTO dashboards (learner_id, name) VALUES ($1, 'Lain') RETURNING id`, stranger)
	part := func(dashboard int64, key string, workbook any) {
		w.exec(nil, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date, workbook_id)
			VALUES ($1, $2, $2, 1, 1, 'current', $3, '2026-10-01', $4)`, dashboard, key, len(key), workbook)
	}
	part(w.dashboard, "p1", workbook)
	part(w.dashboard, "p22", workbook) // a second Part of the same Dashboard from it
	part(second, "p3", workbook)
	part(w.dashboard, "p444", nil) // not cleaned from a Workbook
	part(strangersDashboard, "p5", workbook)

	uses, err := w.ds.Uses(context.Background(), w.learner)
	if err != nil {
		t.Fatal(err)
	}
	// By name ignoring case, so akaun comes before Kedai; none for the other Workbook.
	want := []dataset.Use{{ID: second, Name: "akaun"}, {ID: w.dashboard, Name: "Kedai"}}
	if len(uses) != 1 || !slices.Equal(uses[workbook], want) {
		t.Fatalf("uses = %+v, want only Workbook %d on %+v (not %d)", uses, workbook, want, other)
	}
}

func TestPruneDeletesTheGivenLearnersPartsAndEveryPendingPartLeftPastTheHoursAndReportsHowMany(t *testing.T) {
	w := newWorld(t)
	var stranger, strangersDashboard int64
	w.exec(&stranger, `INSERT INTO learners (name, username, email, password_hash) VALUES ('Bob', 'bob', 'bob@example.invalid', 'x') RETURNING id`)
	w.exec(&strangersDashboard, `INSERT INTO dashboards (learner_id, name) VALUES ($1, 'Lain') RETURNING id`, stranger)
	w.addPart("a", "current", 1, 1)
	w.addPart("old", "pending", 0, 1)
	w.exec(nil, `UPDATE dataset_parts SET uploaded_at = now() - interval '25 hours' WHERE storage_key = 'old'`)
	w.exec(nil, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date)
		VALUES ($1, 'theirs', 'theirs.xlsx', 1, 1, 'current', 1, '2026-10-01')`, strangersDashboard)
	w.exec(nil, `INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, part_date)
		VALUES ($1, 'waiting', 'waiting.xlsx', 1, 1, 'pending', '2026-10-01')`, strangersDashboard)

	ctx := context.Background()
	n, err := w.ds.Prune(ctx, nil, 24)
	if err != nil || n != 1 || w.count("pending") != 0 || w.count("current") != 1 {
		t.Fatalf("Prune of no Learner = %d, %v, want 1: only the pending Part left past 24 hours", n, err)
	}
	n, err = w.ds.Prune(ctx, []int64{w.learner}, 24)
	if err != nil || n != 1 || w.count("current") != 0 {
		t.Fatalf("Prune of the Learner = %d, %v, want their 1 Part", n, err)
	}
	var theirs int
	w.exec(&theirs, `SELECT count(*) FROM dataset_parts WHERE dashboard_id = $1`, strangersDashboard)
	if theirs != 2 {
		t.Fatalf("%d Parts of the Learner not given, want their 2 kept", theirs)
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
