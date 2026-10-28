package dataset_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// uploadAt is the clock of every test: 2 in the morning of 21 October in Malaysia, still the 20th in UTC, which dates a
// Part that names no date.
var uploadAt = time.Date(2026, 10, 21, 2, 0, 0, 0, time.FixedZone("MYT", 8*3600))

const uploadDay = "2026-10-20"

var noReserve dataset.Reserve = func(context.Context, *sql.Tx, int64) error { return nil }

// shop is a workbook of the Dataset these tests grow: Perkara (text), Jumlah and Bulan (numbers; Bulan is 1 to 12 in the
// first Part, so it is a month column).
func shop(t *testing.T, rows ...[]any) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Perkara", "Jumlah", "Bulan"}, rows...)
}

// upload is what the app does for an upload: it asks the module for the recipe to read with, runs Clean-up, and hands
// both to Upload.
type upload struct {
	kind     dataset.Kind
	replaces int64
	name     string
	file     []byte
	picks    map[string]string
	settings lab.Settings
	reserve  dataset.Reserve
}

func (w *world) try(u upload) (*dataset.Pending, error) {
	w.t.Helper()
	ctx := context.Background()
	recipe, err := w.ds.Recipe(ctx, w.dashboard, u.kind)
	if err != nil {
		w.t.Fatal(err)
	}
	u.settings.Recipe, u.settings.FileName = recipe, u.name
	read, err := lab.Clean(bytes.NewReader(u.file), u.settings)
	if err != nil {
		return nil, err
	}
	if u.reserve == nil {
		u.reserve = noReserve
	}
	return w.ds.Upload(ctx, w.dashboard, dataset.Uploaded{Kind: u.kind, Replaces: u.replaces, File: u.name, Sum: sha256.Sum256(u.file),
		Read: read, Settings: u.settings, Picks: u.picks}, u.reserve)
}

func (w *world) put(u upload) *dataset.Pending {
	w.t.Helper()
	p, err := w.try(u)
	if err != nil {
		w.t.Fatal(err)
	}
	return p
}

var noConfirmReserve dataset.ConfirmReserve = func(context.Context, *sql.Tx, *dataset.Pending) error { return nil }

// confirm confirms the Dashboard's pending Part p, keeping the date it has, and returns p.
func (w *world) confirm(p *dataset.Pending) *dataset.Pending {
	w.t.Helper()
	if _, err := w.ds.Confirm(context.Background(), w.dashboard, "", noConfirmReserve, nil); err != nil {
		w.t.Fatal(err)
	}
	return p
}

// junePart is a confirmed first Part, jun.xlsx, of two rows.
func (w *world) junePart() *dataset.Pending {
	w.t.Helper()
	return w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(w.t, []any{"Beras", 120.0, 6.0}, []any{"Gula", 80.0, 6.0})}))
}

// rows are the Part's rows as the store holds them, in the Part's columns.
func (w *world) rows(p *dataset.Pending) [][]any {
	w.t.Helper()
	data, err := w.store.Get(context.Background(), p.Key)
	if err != nil {
		w.t.Fatal(err)
	}
	d, err := lab.PartDataset(data, p.Columns)
	if err != nil {
		w.t.Fatal(err)
	}
	return d.Rows
}

// dates are the Part date on every row the store holds for the Part.
func (w *world) dates(p *dataset.Pending) []string {
	w.t.Helper()
	data, err := w.store.Get(context.Background(), p.Key)
	if err != nil {
		w.t.Fatal(err)
	}
	var objects []map[string]any
	if err := json.Unmarshal(append(append([]byte("["), data...), ']'), &objects); err != nil {
		w.t.Fatal(err)
	}
	var out []string
	for _, o := range objects {
		out = append(out, o["Part date"].(string))
	}
	return out
}

// objects is how many Parts' rows the store holds.
func (w *world) objects() int {
	w.t.Helper()
	files, err := filepath.Glob(filepath.Join(w.dir, "dataset-*"))
	if err != nil {
		w.t.Fatal(err)
	}
	return len(files)
}

// loseObjects takes every Part's rows out of the store, as a store that has lost them.
func (w *world) loseObjects() {
	w.t.Helper()
	files, err := filepath.Glob(filepath.Join(w.dir, "dataset-*"))
	if err != nil {
		w.t.Fatal(err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			w.t.Fatal(err)
		}
	}
}

// forgetMonthColumns takes the month columns out of the recipe, as a Dataset confirmed before they were recorded has it.
func (w *world) forgetMonthColumns() {
	w.t.Helper()
	w.exec(nil, `UPDATE dashboards SET upload_settings = upload_settings #- '{recipe,monthColumns}'`)
}

func changeOf(p *dataset.Pending, key string) *lab.Change {
	for i := range p.Changes {
		if p.Changes[i].Key == key {
			return &p.Changes[i]
		}
	}
	return nil
}

func columnNames(cols []lab.Column) []string {
	var out []string
	for _, c := range cols {
		out = append(out, c.Name)
	}
	return out
}

func TestAFirstPartIsKeptPendingWithItsRowsInTheStore(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	file := shop(t, []any{"Beras", 120.0, 6.0}, []any{"Gula", 80.0, 6.0})
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: file})
	if p.Kind != dataset.ReplaceData || p.File != "jun.xlsx" || p.Added != nil || p.Preview.Replaces != nil || p.ReplacedPart != 0 {
		t.Fatalf("pending Part = %+v, want a Replace data of jun.xlsx with no addition or replacement", p)
	}
	if want := [][]any{{"Beras", 120.0, 6.0}, {"Gula", 80.0, 6.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}
	data, _ := w.store.Get(context.Background(), p.Key)
	if p.Size != int64(len(data)) || p.Size == 0 {
		t.Fatalf("size = %d, want the %d bytes stored", p.Size, len(data))
	}
	if w.count("pending") != 1 {
		t.Fatalf("%d pending Parts, want 1", w.count("pending"))
	}
	if keys, key := w.keys(); len(keys) != 0 || key != "" {
		t.Fatalf("Keys = %v, %q, want the pending Part not in the Dataset", keys, key)
	}
}

func TestANewUploadReplacesThePendingPartAndQueuesItsRowsForDeletion(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	first := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
	second := w.put(upload{kind: dataset.ReplaceData, name: "julai.xlsx", file: shop(t, []any{"Gula", 80.0, 7.0})})
	if w.count("pending") != 1 || w.pendingKey() != second.Key {
		t.Fatalf("pending Part = %q (%d), want only the second, %q", w.pendingKey(), w.count("pending"), second.Key)
	}
	var queued int
	w.exec(&queued, `SELECT count(*) FROM stored_file_deletions WHERE storage_key = $1`, first.Key)
	if queued != 1 {
		t.Fatalf("the first upload's rows are queued %d times for deletion, want once", queued)
	}
}

func TestAnAddAddsAPartKeepingTheDatasetsColumnsAndTellsHowManyRowsItHadBefore(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})})
	if p.Kind != dataset.Add || p.Added == nil || *p.Added != (dataset.Addition{Rows: 1, Had: 2}) || p.Added.Total() != 3 {
		t.Fatalf("pending Part = %+v (added %+v), want an Add of 1 row to 2", p, p.Added)
	}
	if want := [][]any{{"Tepung", 60.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}
	if keys, _ := w.keys(); len(keys) != 1 {
		t.Fatalf("Keys = %v, want the pending Add outside the Dataset", keys)
	}
}

func TestAnAddToADatasetWithNoPartIsReplaceData(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	p := w.put(upload{kind: dataset.Add, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
	if p.Kind != dataset.ReplaceData || p.Added != nil {
		t.Fatalf("pending Part = %+v, want a Replace data", p)
	}
}

func TestAReplaceOfOnePartNamesThePartAndCountsTheRowsOfTheOthers(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})}))
	p := w.put(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun-betul.xlsx", file: shop(t, []any{"Garam", 70.0, 6.0})})
	want := dataset.Replacement{Part: june.ID, File: "jun.xlsx", Rows: 2, Had: 1}
	if p.Kind != dataset.ReplacePart || p.ReplacedPart != june.ID || p.Preview.Replaces == nil || *p.Preview.Replaces != want || p.Preview.Replaces.Total(1) != 2 {
		t.Fatalf("pending Part = %+v (replaces %+v), want %+v", p, p.Preview.Replaces, want)
	}
	if p.Added != nil {
		t.Fatalf("a Replace of a Part has an addition: %+v", p.Added)
	}
}

func TestAReplaceOfAPartThatIsGoneIsRefusedAndKeepsNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})}))
	if err := w.ds.Delete(context.Background(), w.dashboard, june.ID); err != nil {
		t.Fatal(err)
	}
	before := w.objects()
	_, err := w.try(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun-betul.xlsx", file: shop(t, []any{"Garam", 70.0, 6.0})})
	if !errors.Is(err, dataset.ErrPartGone) {
		t.Fatalf("Replace of a deleted Part = %v, want ErrPartGone", err)
	}
	if w.count("pending") != 0 || w.objects() != before {
		t.Fatalf("%d pending Parts and %d stored objects (was %d) after the refusal, want none and no new object", w.count("pending"), w.objects(), before)
	}
}

func TestReplacingDataReadsTheUploadAsAFirstPartWithNewColumnsButReplacingTheOnlyPartDoesNot(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	markah := testutil.XLSX(t, []string{"Nama", "Markah"}, []any{"Ali", 80.0}, []any{"Siti", 90.0})

	_, err := w.try(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "markah.xlsx", file: markah})
	var missing *dataset.MissingColumnsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Missing, []string{"Perkara", "Jumlah", "Bulan"}) || !slices.Equal(missing.Unused, []string{"Nama", "Markah"}) {
		t.Fatalf("Replace of the only Part with other columns = %v, want the missing columns and the file's unused ones", err)
	}

	p := w.put(upload{kind: dataset.ReplaceData, name: "markah.xlsx", file: markah})
	if !slices.Equal(columnNames(p.Columns), []string{"Nama", "Markah"}) || p.Settings.Recipe != nil || p.Added != nil {
		t.Fatalf("pending Part = %+v, want the file's own columns, read with no recipe", p)
	}
}

func TestAnAddDropsTheFilesExtraColumnsAndListsThem(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	file := testutil.XLSX(t, []string{"Perkara", "Catatan", "Jumlah", "Bulan"}, []any{"Tepung", "baru", 60.0, 7.0})
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: file})
	if !slices.Equal(columnNames(p.Columns), []string{"Perkara", "Jumlah", "Bulan"}) || !slices.Equal(p.Unused, []string{"Catatan"}) {
		t.Fatalf("columns = %v, unused = %v, want the Dataset's three and Catatan dropped", columnNames(p.Columns), p.Unused)
	}
	if c := changeOf(p, "lab.clean.extra-columns"); c == nil || c.Column != "Catatan" {
		t.Fatalf("changes = %+v, want the extra column listed", p.Changes)
	}
	if want := [][]any{{"Tepung", 60.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}
}

func TestAnAddMissingAColumnIsRefusedNamingItWithTheFilesUnusedColumnsAndKeepsNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	file := testutil.XLSX(t, []string{"Perkara", "Amaun", "Catatan", "Bulan"}, []any{"Tepung", 60.0, "baru", 7.0})
	before := w.objects()
	_, err := w.try(upload{kind: dataset.Add, name: "julai.xlsx", file: file})
	var missing *dataset.MissingColumnsError
	if !errors.As(err, &missing) {
		t.Fatalf("Add missing a column = %v, want a *MissingColumnsError", err)
	}
	if !slices.Equal(missing.Missing, []string{"Jumlah"}) || !slices.Equal(missing.Unused, []string{"Amaun", "Catatan"}) || missing.Problem.Key != "lab.err.add-columns" {
		t.Fatalf("refusal = %+v, want Jumlah missing and Amaun, Catatan unused", missing)
	}
	if w.count("pending") != 0 || w.objects() != before {
		t.Fatalf("%d pending Parts and %d stored objects (was %d) after the refusal, want none and no new object", w.count("pending"), w.objects(), before)
	}
}

func TestAMissingColumnIsMatchedByNameIgnoringCaseSpacesUnderscoresAndDashesWithANote(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	file := testutil.XLSX(t, []string{"Perkara", "jum_lah", "Bulan"}, []any{"Tepung", 60.0, 7.0})
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: file})
	if want := []lab.Mapping{{File: "jum_lah", Column: "Jumlah"}}; !reflect.DeepEqual(p.Mapped, want) {
		t.Fatalf("mapped = %+v, want %+v", p.Mapped, want)
	}
	if c := changeOf(p, "lab.clean.matched-columns"); c == nil || c.Column != "jum_lah → Jumlah" {
		t.Fatalf("changes = %+v, want the match noted", p.Changes)
	}
	if want := [][]any{{"Tepung", 60.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want the value under the Dataset's name, %v", w.rows(p), want)
	}
}

// amaun is a file whose Jumlah is called Amaun, and which has another column to pick instead.
func amaun(t *testing.T, jumlah float64) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Perkara", "Amaun", "Lain", "Bulan"}, []any{"Tepung", jumlah, 5.0, 7.0})
}

func TestAMissingColumnIsReadFromTheLearnersPickAndTheRecipeRemembersIt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: amaun(t, 60), picks: map[string]string{"Jumlah": "Amaun"}})
	if want := []lab.Mapping{{File: "Amaun", Column: "Jumlah", Chosen: true}}; !reflect.DeepEqual(p.Mapped, want) {
		t.Fatalf("mapped = %+v, want %+v", p.Mapped, want)
	}
	if c := changeOf(p, "lab.clean.mapped-columns"); c == nil || c.Column != "Amaun → Jumlah" {
		t.Fatalf("changes = %+v, want the pick noted", p.Changes)
	}
	if want := [][]any{{"Tepung", 60.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}
	if want := map[string]string{"Amaun": "Jumlah"}; !reflect.DeepEqual(p.Settings.Recipe.Map, want) {
		t.Fatalf("the recipe's map = %v, want %v", p.Settings.Recipe.Map, want)
	}

	// Nothing is remembered before the Part is confirmed; after, the next file with the same headers maps itself.
	if recipe, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add); err != nil || len(recipe.Map) != 0 {
		t.Fatalf("recipe map before the confirm = %v (%v), want none", recipe.Map, err)
	}
	w.confirm(p)
	next := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: amaun(t, 70)})
	if want := []lab.Mapping{{File: "Amaun", Column: "Jumlah", Chosen: true}}; !reflect.DeepEqual(next.Mapped, want) {
		t.Fatalf("next file mapped = %+v, want it mapped by the remembered pick %+v", next.Mapped, want)
	}
}

func TestARememberedPickCanBeChangedOrClearedAndTheRefusalLeavesTheRecipeAsItWas(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: amaun(t, 60), picks: map[string]string{"Jumlah": "Amaun"}}))

	changed := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: amaun(t, 70), picks: map[string]string{"Jumlah": "Lain"}})
	if want := []lab.Mapping{{File: "Lain", Column: "Jumlah", Chosen: true}}; !reflect.DeepEqual(changed.Mapped, want) {
		t.Fatalf("mapped = %+v, want the changed pick %+v", changed.Mapped, want)
	}
	if want := map[string]string{"Lain": "Jumlah"}; !reflect.DeepEqual(changed.Settings.Recipe.Map, want) {
		t.Fatalf("the recipe's map = %v, want %v", changed.Settings.Recipe.Map, want)
	}

	_, err := w.try(upload{kind: dataset.Add, name: "sep.xlsx", file: amaun(t, 80), picks: map[string]string{"Jumlah": ""}})
	var missing *dataset.MissingColumnsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Missing, []string{"Jumlah"}) {
		t.Fatalf("Add with the pick cleared = %v, want Jumlah missing", err)
	}
	if recipe, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add); err != nil || !reflect.DeepEqual(recipe.Map, map[string]string{"Amaun": "Jumlah"}) {
		t.Fatalf("recipe map after the refusal = %v (%v), want the confirmed pick", recipe.Map, err)
	}
}

func TestPicksThatAreNotRealColumnsAreDroppedAndNeverRemembered(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	picks := map[string]string{"Perkara": "Lain", "Bulan": "Amaun", "Jumlah": "Amaun", "Tiada": "Lain", "Tiada juga": "Nombor"}
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: amaun(t, 60), picks: picks})
	if want := map[string]string{"Amaun": "Jumlah"}; !reflect.DeepEqual(p.Settings.Recipe.Map, want) {
		t.Fatalf("the recipe's map = %v, want only the one real pick %v", p.Settings.Recipe.Map, want)
	}
}

func TestOneFileColumnPickedForTwoDatasetColumnsIsRefusedNamingAllThree(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	file := testutil.XLSX(t, []string{"Perkara", "Amaun", "Lain"}, []any{"Tepung", 60.0, 5.0})
	_, err := w.try(upload{kind: dataset.Add, name: "julai.xlsx", file: file, picks: map[string]string{"Jumlah": "Amaun", "Bulan": "Amaun"}})
	var missing *dataset.MissingColumnsError
	if !errors.As(err, &missing) {
		t.Fatalf("Add with one column picked twice = %v, want a *MissingColumnsError", err)
	}
	if missing.Problem.Key != "lab.err.map-twice" || !reflect.DeepEqual(missing.Problem.Args, []any{"Amaun", "Bulan", "Jumlah"}) {
		t.Fatalf("problem = %+v, want map-twice naming Amaun, Bulan and Jumlah", missing.Problem)
	}
	if !slices.Equal(missing.Unused, []string{"Amaun", "Lain"}) {
		t.Fatalf("unused = %v, want the file's columns to pick from again", missing.Unused)
	}
}

func TestReplacingDataForgetsThePicksWithTheRecipe(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: amaun(t, 60), picks: map[string]string{"Jumlah": "Amaun"}}))
	if recipe, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add); err != nil || len(recipe.Map) == 0 {
		t.Fatalf("recipe map = %v (%v), want the pick remembered", recipe.Map, err)
	}

	// The file with the renamed column replaces the data: its columns are the Dataset's, with nothing to map.
	p := w.put(upload{kind: dataset.ReplaceData, name: "ogos.xlsx", file: amaun(t, 70)})
	if len(p.Mapped) != 0 || p.Settings.Recipe != nil || !slices.Equal(columnNames(p.Columns), []string{"Perkara", "Amaun", "Lain", "Bulan"}) {
		t.Fatalf("pending Part = %+v, want the file's own columns and no mapping", p)
	}
	w.confirm(p)
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})}))
	if recipe, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add); err != nil || len(recipe.Map) != 0 {
		t.Fatalf("recipe map after Replace data = %v (%v), want none", recipe.Map, err)
	}
	_, err := w.try(upload{kind: dataset.Add, name: "sep.xlsx", file: amaun(t, 80)})
	var missing *dataset.MissingColumnsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Missing, []string{"Jumlah"}) {
		t.Fatalf("Add of the renamed file after Replace data = %v, want Jumlah missing", err)
	}
}

func TestAMappedColumnWhoseCellsDoNotFitTheDatasetsTypeLeavesThemEmptyAndListsThem(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	file := testutil.XLSX(t, []string{"Perkara", "Amaun", "Bulan"}, []any{"Tepung", "ramai", 7.0}, []any{"Garam", 70.0, 7.0})
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: file, picks: map[string]string{"Jumlah": "Amaun"}})
	if c := changeOf(p, "lab.clean.not-numbers"); c == nil || c.Column != "Jumlah" || !slices.Equal(c.Args, []int{1}) {
		t.Fatalf("changes = %+v, want one cell of Jumlah left empty", p.Changes)
	}
	if want := [][]any{{"Tepung", nil, 7.0}, {"Garam", 70.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}
}

func rowsIn(sheet string, rows ...[]any) testutil.Sheet {
	return testutil.Sheet{Name: sheet, Rows: append([][]any{{"Perkara", "Jumlah", "Bulan"}}, rows...)}
}

func TestAnAddReadsTheRecipeSheetIgnoringCaseAndNeverCombinesAnotherSheetWithTheSameColumns(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: testutil.Workbook(t, rowsIn("Sheet1", []any{"Beras", 120.0, 6.0}))}))
	file := testutil.Workbook(t, rowsIn("SHEET1", []any{"Tepung", 60.0, 7.0}, []any{"Garam", 70.0, 7.0}), rowsIn("Subset", []any{"Gula", 80.0, 7.0}))
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: file})
	if want := [][]any{{"Tepung", 60.0, 7.0}, {"Garam", 70.0, 7.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want only the recipe sheet's %v", w.rows(p), want)
	}
	if p.Added.Rows != 2 || changeOf(p, "lab.clean.combined") != nil {
		t.Fatalf("pending Part = %+v, want 2 rows and no sheets combined", p)
	}
}

func TestAPartIsReadWithItsOwnHeaderRowWhenTheFirstPartsWasFoundByCaveButWithTheTypedOneWhenTheLearnerTypedIt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	titled := testutil.Workbook(t, testutil.Sheet{Name: "Sheet1", Rows: [][]any{{"Kedai Runcit Ali"}, {"Laporan Jun"}, {},
		{"Perkara", "Jumlah", "Bulan"}, {"Beras", 120.0, 6.0}}})
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: titled}))
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Gula", 80.0, 7.0}, []any{"Garam", 70.0, 7.0})})
	if p.Added.Rows != 2 {
		t.Fatalf("pending Part = %+v, want its 2 rows read from its own header row", p)
	}

	// A header the Learner typed for the first Part is read again for every Part: row 1 looks like a header too.
	typed := newWorld(t)
	layout := func(item string) []byte {
		return testutil.XLSX(t, []string{"Perkara", "Jumlah", "Bulan"}, []any{"Perkara", "Jumlah", "Bulan"}, []any{item, 5.0, 6.0})
	}
	typed.confirm(typed.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: layout("Beras"), settings: lab.Settings{HeaderRow: 2}}))
	if recipe, err := typed.ds.Recipe(context.Background(), typed.dashboard, dataset.Add); err != nil || recipe.HeaderRow != 2 || !recipe.HeaderTyped {
		t.Fatalf("recipe = %+v (%v), want the typed header row 2", recipe, err)
	}
	q := typed.put(upload{kind: dataset.Add, name: "julai.xlsx", file: layout("Gula")})
	if q.Added.Rows != 1 || q.Added.Had != 1 {
		t.Fatalf("pending Part = %+v, want 1 row read under the typed header row", q)
	}
}

func TestAMonthNameInTheDatasetsMonthColumnIsReadAsItsNumberAndListedOnItsOwn(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	p := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Tepung", 60.0, "OGOS"}, []any{"Garam", 70.0, "Ogos"})})
	if want := [][]any{{"Tepung", 60.0, 8.0}, {"Garam", 70.0, 8.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want the month names as 8", w.rows(p))
	}
	if c := changeOf(p, "lab.clean.month-names"); c == nil || c.Column != "Bulan" || !slices.Equal(c.Args, []int{2}) {
		t.Fatalf("changes = %+v, want 2 month names in Bulan listed", p.Changes)
	}
	if changeOf(p, "lab.clean.not-numbers") != nil {
		t.Fatalf("changes = %+v, want the month names not counted among the cells left empty", p.Changes)
	}
	// The recipe keeps the month column, so the next Part reads its names the same way.
	w.confirm(p)
	next := w.put(upload{kind: dataset.Add, name: "sep.xlsx", file: shop(t, []any{"Lada", 20.0, "Sep"})})
	if want := [][]any{{"Lada", 20.0, 9.0}}; !reflect.DeepEqual(w.rows(next), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(next), want)
	}
}

func TestADatasetWithoutRecordedMonthColumnsFindsThemFromItsPartsOnceOnTheNextAdd(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.forgetMonthColumns()
	if recipe, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add); err != nil || recipe.MonthColumns != nil {
		t.Fatalf("recipe month columns = %v (%v), want never found", recipe.MonthColumns, err)
	}
	p := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Tepung", 60.0, "OGOS"})})
	if got := p.Settings.Recipe.MonthColumns; !slices.Equal(got, []string{"Bulan"}) {
		t.Fatalf("month columns found = %v, want [Bulan]", got)
	}
	if want := [][]any{{"Tepung", 60.0, 8.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want %v", w.rows(p), want)
	}

	// Once recorded they are never found again: with every Part's rows gone from the store the next Add still works.
	w.confirm(p)
	w.loseObjects()
	next := w.put(upload{kind: dataset.Add, name: "sep.xlsx", file: shop(t, []any{"Lada", 20.0, "Sep"})})
	if changeOf(next, "lab.clean.month-names") == nil {
		t.Fatalf("changes = %+v, want the month name read", next.Changes)
	}
}

func TestADatasetWithNoMonthColumnRecordsNoneAndTheNameIsLeftEmpty(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 13.0})}))
	w.forgetMonthColumns()
	p := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Tepung", 60.0, "OGOS"})})
	if got := p.Settings.Recipe.MonthColumns; got == nil || len(got) != 0 {
		t.Fatalf("month columns found = %#v, want none, recorded as an empty list", got)
	}
	if c := changeOf(p, "lab.clean.not-numbers"); c == nil || c.Column != "Bulan" || changeOf(p, "lab.clean.month-names") != nil {
		t.Fatalf("changes = %+v, want OGOS left empty in Bulan and not read as a month", p.Changes)
	}
	// Recorded as none, it is not looked for again either.
	w.confirm(p)
	w.loseObjects()
	w.put(upload{kind: dataset.Add, name: "sep.xlsx", file: shop(t, []any{"Lada", 20.0, "Sep"})})
}

func TestAStoreThatCannotGiveAPartToTheOneTimeScanRefusesWithErrStoreAndKeepsNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.forgetMonthColumns()
	w.loseObjects()
	_, err := w.try(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Tepung", 60.0, "OGOS"})})
	if !errors.Is(err, dataset.ErrStore) {
		t.Fatalf("Add with the store failing = %v, want ErrStore", err)
	}
	if w.count("pending") != 0 {
		t.Fatalf("%d pending Parts after the refusal, want none", w.count("pending"))
	}
}

func TestReplacingTheOnlyPartOfADatasetWithoutRecordedMonthColumnsIsDecidedByTheNewPartAlone(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	only := w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 13.0})}))
	w.forgetMonthColumns()
	w.loseObjects() // the Part being replaced is not read: its values are going away
	p := w.put(upload{kind: dataset.ReplacePart, replaces: only.ID, name: "ogos.xlsx", file: shop(t, []any{"Tepung", 60.0, "OGOS"})})
	if want := [][]any{{"Tepung", 60.0, 8.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("stored rows = %v, want OGOS read as 8: the old 13 is going away", w.rows(p))
	}
	if !slices.Contains(p.Settings.Recipe.MonthColumns, "Bulan") {
		t.Fatalf("month columns = %v, want Bulan among them", p.Settings.Recipe.MonthColumns)
	}
}

func TestTheExactFileOfACurrentPartIsRefusedNamingThePartButOtherOverlapIsAccepted(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	same := shop(t, []any{"Beras", 120.0, 6.0}, []any{"Gula", 80.0, 6.0})
	before := w.objects()
	_, err := w.try(upload{kind: dataset.Add, name: "jun-lagi.xlsx", file: same})
	var refusal *dataset.SameFileError
	if !errors.As(err, &refusal) || refusal.File != "jun.xlsx" || refusal.Date != uploadDay {
		t.Fatalf("Add of the same file = %v, want a *SameFileError naming jun.xlsx dated %s", err, uploadDay)
	}
	var id int64
	w.exec(&id, `SELECT id FROM dataset_parts WHERE file_name = 'jun.xlsx'`)
	if refusal.ID != id || w.count("pending") != 0 || w.objects() != before {
		t.Fatalf("refusal names Part %d (want %d); %d pending Parts, %d stored objects (was %d), want none new", refusal.ID, id, w.count("pending"), w.objects(), before)
	}
	if err := w.ds.SameFile(context.Background(), w.dashboard, dataset.Add, 0, sha256.Sum256(same)); !errors.As(err, &refusal) {
		t.Fatalf("SameFile = %v, want the same refusal without keeping anything", err)
	}

	// A different file that repeats nine rows in ten of the Part's is no duplicate.
	var rows [][]any
	for i := range 9 {
		rows = append(rows, []any{"Barang", float64(10 * (i + 1)), 6.0})
	}
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "sembilan.xlsx", file: shop(t, append(slices.Clone(rows), []any{"Beras", 5.0, 6.0})...)}))
	w.put(upload{kind: dataset.Add, name: "sepuluh.xlsx", file: shop(t, append(slices.Clone(rows), []any{"Gula", 5.0, 6.0})...)})
}

func TestAReplaceOfOnePartMayRepeatThatPartsOwnFileButNotAnotherPartsAndReplaceDataMayRepeatAny(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	julyFile := shop(t, []any{"Tepung", 60.0, 7.0})
	july := w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: julyFile}))

	// jun.xlsx's own bytes to replace June, as when the preview posts the same file with other choices.
	w.put(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0}, []any{"Gula", 80.0, 6.0})})
	// julai.xlsx's bytes to replace June: the matching Part is July.
	_, err := w.try(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "julai-lagi.xlsx", file: julyFile})
	var refusal *dataset.SameFileError
	if !errors.As(err, &refusal) || refusal.ID != july.ID {
		t.Fatalf("Replace with another Part's file = %v, want a refusal naming July's Part %d", err, july.ID)
	}
	// Replace data throws every Part away, so a file one of them came from is fine.
	w.put(upload{kind: dataset.ReplaceData, name: "julai-lagi.xlsx", file: julyFile})
}

// stock is a stock list with its date printed in cell H2, above the column names.
func stock(t *testing.T, day string) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Stock Balance Report"}, []any{nil, nil, nil, nil, nil, nil, nil, day}, []any{"Perkara", "Baki", "Unit"}, []any{"Gula", 3, "kg"})
}

func TestAPartIsDatedFromTheTitleRowsElseTheSheetNameElseTheFileNameElseTheUploadDayAndSaysWhere(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	sheetNamed := testutil.Workbook(t, testutil.Sheet{Name: "02.09.26", Rows: [][]any{{"Perkara", "Baki"}, {"Gula", 3}}})
	undated := testutil.XLSX(t, []string{"Perkara", "Baki"}, []any{"Gula", 3})
	for _, c := range []struct {
		name       string
		file       []byte
		date, from string
	}{
		{"stok_20260702.xlsx", stock(t, "01.09.2026"), "2026-09-01", lab.DateFromCell},
		{"Report_20260702091058.xlsx", sheetNamed, "2026-09-02", lab.DateFromSheet},
		{"Stock_Balance_History_Report_20260702091058.xlsx", undated, "2026-07-02", lab.DateFromFile},
		{"kehadiran_062026.xlsx", undated, "2026-06-01", lab.DateFromFile},
		{"stok.xlsx", undated, uploadDay, lab.DateFromUpload},
	} {
		p := w.put(upload{kind: dataset.ReplaceData, name: c.name, file: c.file})
		if p.PartDate != c.date || p.DateFrom != c.from {
			t.Errorf("%s: dated %s from %s, want %s from %s", c.name, p.PartDate, p.DateFrom, c.date, c.from)
		}
		if got := w.dates(p); !slices.Equal(got, []string{c.date}) {
			t.Errorf("%s: stored rows carry %v, want %s", c.name, got, c.date)
		}
	}
}

func TestThePartDatesUploadDayFallbackFollowsTheClockInUTC(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	file := testutil.XLSX(t, []string{"Perkara", "Baki"}, []any{"Gula", 3})
	w.now = time.Date(2027, 1, 1, 0, 30, 0, 0, time.FixedZone("MYT", 8*3600)) // 31 December in UTC
	if p := w.put(upload{kind: dataset.ReplaceData, name: "stok.xlsx", file: file}); p.PartDate != "2026-12-31" || p.DateFrom != lab.DateFromUpload {
		t.Fatalf("dated %s from %s, want 2026-12-31 from the upload day", p.PartDate, p.DateFrom)
	}
}

func TestARefusedReserveLeavesNoPendingPartAndNoStoredRowsAndKeepsAnEarlierPending(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	full := errors.New("full")
	_, err := w.try(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0}),
		reserve: func(context.Context, *sql.Tx, int64) error { return full }})
	if !errors.Is(err, full) {
		t.Fatalf("Upload with a refused reserve = %v, want the reserve's error", err)
	}
	if w.count("pending") != 0 || w.objects() != 0 {
		t.Fatalf("%d pending Parts and %d stored objects after the refusal, want none", w.count("pending"), w.objects())
	}

	// An earlier pending Part stays as it was, and so do its rows.
	earlier := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
	_, err = w.try(upload{kind: dataset.ReplaceData, name: "julai.xlsx", file: shop(t, []any{"Gula", 80.0, 7.0}),
		reserve: func(context.Context, *sql.Tx, int64) error { return full }})
	if !errors.Is(err, full) || w.pendingKey() != earlier.Key || w.objects() != 1 {
		t.Fatalf("after a second refusal: err = %v, pending Part %q, %d stored objects, want the earlier Part %q alone", err, w.pendingKey(), w.objects(), earlier.Key)
	}
}

func TestTheReserveIsGivenTheBytesTheUploadAddsAndRunsBeforeAnyRowIsLocked(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})}) // a pending Part to lock
	var added int64
	var locks, stillThere error
	var pending int
	reserve := func(ctx context.Context, tx *sql.Tx, bytes int64) error {
		added = bytes
		// Another connection can take the Learner's lock and the pending Part's at once: the module holds none yet,
		// so a reserve that locks the Learner is always the first to.
		other, err := w.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer other.Rollback()
		_, locks = other.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE NOWAIT`, w.learner)
		if locks == nil {
			_, locks = other.ExecContext(ctx, `SELECT 1 FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending' FOR UPDATE NOWAIT`, w.dashboard)
		}
		// And the pending Part it will replace is still there in the module's own transaction.
		stillThere = tx.QueryRowContext(ctx, `SELECT count(*) FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending'`, w.dashboard).Scan(&pending)
		return nil
	}
	p := w.put(upload{kind: dataset.ReplaceData, name: "julai.xlsx", file: shop(t, []any{"Gula", 80.0, 7.0}), reserve: reserve})
	if locks != nil || stillThere != nil || pending != 1 {
		t.Fatalf("inside the reserve: lock errors %v and %v, %d pending Parts, want a row free to lock and the earlier Part not yet deleted", locks, stillThere, pending)
	}
	if added != p.Size {
		t.Fatalf("reserve was given %d bytes, want the Part's saved size %d", added, p.Size)
	}
}

func TestADashboardThatCavesBuildsRefusesATotalPastFiftyThousandRowsButAPageDashboardHasNoLimit(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	w.exec(nil, `UPDATE dataset_parts SET row_count = 49999 WHERE id = $1`, june.ID)
	w.put(upload{kind: dataset.Add, name: "satu.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})}) // 50,000 in all

	var problem lab.Problem
	_, err := w.try(upload{kind: dataset.Add, name: "dua.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0}, []any{"Garam", 70.0, 7.0})})
	if !errors.As(err, &problem) || problem.Key != "lab.err.add-too-many-rows" {
		t.Fatalf("Add past the limit = %v, want add-too-many-rows", err)
	}
	// A Replace counts the rows of the others only.
	w.confirm(w.put(upload{kind: dataset.Add, name: "tiga.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})}))
	w.exec(nil, `UPDATE dataset_parts SET row_count = 1 WHERE id = $1`, june.ID)
	w.exec(nil, `UPDATE dataset_parts SET row_count = 49999 WHERE file_name = 'tiga.xlsx'`)
	_, err = w.try(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun-betul.xlsx", file: shop(t, []any{"Garam", 70.0, 6.0}, []any{"Lada", 20.0, 6.0})})
	if !errors.As(err, &problem) || problem.Key != "lab.err.replace-too-many-rows" {
		t.Fatalf("Replace past the limit = %v, want replace-too-many-rows", err)
	}

	w.exec(nil, `INSERT INTO dashboard_pages (dashboard_id, storage_key, size, current) VALUES ($1, 'page-1', 10, true)`, w.dashboard)
	w.put(upload{kind: dataset.Add, name: "empat.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0}, []any{"Garam", 70.0, 7.0})})
}
