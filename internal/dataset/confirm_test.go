package dataset_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// confirmOn confirms the pending Part with the date the Learner chose, hook and reserve; the reserve is noConfirmReserve
// when nil.
func (w *world) confirmOn(date string, reserve dataset.ConfirmReserve, hook dataset.Hook) (*dataset.Confirmed, error) {
	w.t.Helper()
	if reserve == nil {
		reserve = noConfirmReserve
	}
	return w.ds.Confirm(context.Background(), w.dashboard, date, reserve, hook)
}

// names are the Perkara of every row of the Dataset's Parts, Part after Part in their order.
func (w *world) names() []string {
	w.t.Helper()
	keys, _ := w.keys()
	var out []string
	for _, key := range keys {
		data, err := w.store.Get(context.Background(), key)
		if err != nil {
			w.t.Fatal(err)
		}
		var objects []map[string]any
		if err := json.Unmarshal(append(append([]byte("["), data...), ']'), &objects); err != nil {
			w.t.Fatal(err)
		}
		for _, o := range objects {
			out = append(out, o["Perkara"].(string))
		}
	}
	return out
}

// queued is how many times the store object under key is queued for deletion.
func (w *world) queued(key string) int {
	w.t.Helper()
	var n int
	w.exec(&n, `SELECT count(*) FROM stored_file_deletions WHERE storage_key = $1`, key)
	return n
}

// recipe is the recipe the Dashboard's next Add is read with.
func (w *world) recipe() *lab.Recipe {
	w.t.Helper()
	r, err := w.ds.Recipe(context.Background(), w.dashboard, dataset.Add)
	if err != nil {
		w.t.Fatal(err)
	}
	return r
}

func (w *world) pending() *dataset.Pending {
	w.t.Helper()
	p, err := w.ds.Pending(context.Background(), w.dashboard)
	if err != nil {
		w.t.Fatal(err)
	}
	return p
}

// threeParts are june (Beras, Gula), julai (Tepung) and ogos (Garam), confirmed in that order; the IDs are june's, julai's
// and ogos's.
func (w *world) threeParts() (june, julai, ogos int64) {
	w.t.Helper()
	june = w.junePart().ID
	julai = w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(w.t, []any{"Tepung", 60.0, 7.0})})).ID
	ogos = w.confirm(w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(w.t, []any{"Garam", 70.0, 8.0})})).ID
	return june, julai, ogos
}

func TestAnAddConfirmsAsTheLastPartAndKeepsTheOthers(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	p := w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})})

	c, err := w.confirmOn("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != p.ID || c.Key != p.Key || !slices.Equal(columnNames(c.Columns), []string{"Perkara", "Jumlah", "Bulan"}) || !reflect.DeepEqual(c.Dataset.Rows, [][]any{{"Tepung", 60.0, 7.0}}) {
		t.Fatalf("confirmed = %+v, want the Part, its key, the Dataset's columns and its one row", c)
	}
	if got := w.names(); !slices.Equal(got, []string{"Beras", "Gula", "Tepung"}) {
		t.Fatalf("the Dataset = %v, want June's rows then July's", got)
	}
	if w.count("pending") != 0 || w.pending() != nil {
		t.Fatal("the Part is still pending after the confirm")
	}
}

func TestReplacingOnePartConfirmsInItsPlaceAndQueuesItsRowsForDeletion(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june, _, _ := w.threeParts()
	oldKeys, _ := w.keys()
	p := w.put(upload{kind: dataset.ReplacePart, replaces: june, name: "jun-betul.xlsx", file: shop(t, []any{"Kicap", 90.0, 6.0})})

	if _, err := w.confirmOn("", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := w.names(); !slices.Equal(got, []string{"Kicap", "Tepung", "Garam"}) {
		t.Fatalf("the Dataset = %v, want the new Part first, where June was", got)
	}
	keys, _ := w.keys()
	if len(keys) != 3 || keys[0] != p.Key || !slices.Equal(keys[1:], oldKeys[1:]) {
		t.Fatalf("keys = %v, want the new Part's key then the other two as they were (%v)", keys, oldKeys)
	}
	if w.queued(oldKeys[0]) != 1 {
		t.Fatalf("the replaced Part's rows are queued %d times for deletion, want once", w.queued(oldKeys[0]))
	}
}

func TestReplacingDataConfirmsAsTheOnlyPartWithTheUploadsColumns(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.threeParts()
	w.addPart("before-parts", "earlier", 0, 1)
	oldKeys, _ := w.keys()
	p := w.put(upload{kind: dataset.ReplaceData, name: "markah.xlsx", file: testutil.XLSX(t, []string{"Nama", "Markah"}, []any{"Ali", 80.0})})

	c, err := w.confirmOn("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := w.keys()
	if !slices.Equal(keys, []string{p.Key}) || w.count("current") != 1 || w.count("earlier") != 0 {
		t.Fatalf("keys = %v with %d current and %d earlier Parts, want only the new one", keys, w.count("current"), w.count("earlier"))
	}
	if !slices.Equal(columnNames(c.Columns), []string{"Nama", "Markah"}) || !slices.Equal(columnNames(w.recipe().Columns), []string{"Nama", "Markah"}) {
		t.Fatalf("columns = %v, want the upload's now the Dataset's", columnNames(c.Columns))
	}
	for _, key := range append(oldKeys, "before-parts") {
		if w.queued(key) != 1 {
			t.Fatalf("the rows under %s are queued %d times for deletion, want once", key, w.queued(key))
		}
	}
}

func TestADateTheLearnerKeepsLeavesTheRowsWhereTheyAre(t *testing.T) {
	t.Parallel()
	for _, date := range []string{"", "  ", uploadDay} {
		w := newWorld(t)
		p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
		c, err := w.confirmOn(date, nil, nil)
		if err != nil {
			t.Fatalf("date %q: %v", date, err)
		}
		if c.Key != p.Key || w.queued(p.Key) != 0 || !slices.Equal(w.dates(p), []string{uploadDay}) {
			t.Fatalf("date %q: key %q (was %q), queued %d, dates %v, want the rows as uploaded", date, c.Key, p.Key, w.queued(p.Key), w.dates(p))
		}
	}
}

func TestAChangedDateWritesTheRowsAgainUnderANewKeyAndQueuesTheOldRows(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0}, []any{"Gula", 80.0, 6.0})})

	c, err := w.confirmOn(" 2026-06-30 ", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Key == p.Key {
		t.Fatal("the confirm kept the key although the date changed")
	}
	moved := &dataset.Pending{Key: c.Key, Preview: dataset.Preview{Columns: c.Columns}}
	if got := w.dates(moved); !slices.Equal(got, []string{"2026-06-30", "2026-06-30"}) {
		t.Fatalf("Part date on the rewritten rows = %v, want the chosen date on both", got)
	}
	if !reflect.DeepEqual(w.rows(moved), w.rows(p)) {
		t.Fatalf("rewritten rows = %v, want the same as uploaded %v", w.rows(moved), w.rows(p))
	}
	if keys, _ := w.keys(); !slices.Equal(keys, []string{c.Key}) {
		t.Fatalf("keys = %v, want the Part switched to the new key", keys)
	}
	if w.queued(p.Key) != 1 {
		t.Fatalf("the old rows are queued %d times for deletion, want once", w.queued(p.Key))
	}
	var source string
	w.exec(&source, `SELECT date_source FROM dataset_parts WHERE id = $1`, c.ID)
	if source != lab.DateFromChosen {
		t.Fatalf("the date's source = %q, want %q", source, lab.DateFromChosen)
	}
}

func TestAReplaceOfOnePartIsReadWithTheRecipeAndConfirmingItKeepsTheRecipe(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})}))
	before := w.recipe()

	file := testutil.Workbook(t, rowsIn("SHEET1", []any{"Kicap", 90.0, 6.0}), rowsIn("Subset", []any{"Gula", 80.0, 6.0}))
	p := w.put(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun-betul.xlsx", file: file})
	if want := [][]any{{"Kicap", 90.0, 6.0}}; !reflect.DeepEqual(w.rows(p), want) {
		t.Fatalf("rows = %v, want only the recipe's sheet, %v", w.rows(p), want)
	}
	w.confirm(p)
	if after := w.recipe(); after.Sheet != before.Sheet || after.HeaderRow != before.HeaderRow || !slices.Equal(after.MonthColumns, before.MonthColumns) {
		t.Fatalf("recipe after the Replace = %+v, want it as it was, %+v", after, before)
	}
	if got := w.names(); !slices.Equal(got, []string{"Kicap", "Tepung"}) {
		t.Fatalf("the Dataset = %v, want the new Part where June was", got)
	}
}

func TestADateOutsideNineteenHundredToTwentyOneHundredIsRefusedAndChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
	before := w.objects()
	for _, date := range []string{"1899-12-31", "2101-01-01", "tomorrow", "2026-13-01"} {
		if _, err := w.confirmOn(date, nil, nil); !errors.Is(err, dataset.ErrPartDate) {
			t.Fatalf("date %q = %v, want ErrPartDate", date, err)
		}
	}
	if got := w.pending(); got == nil || got.Key != p.Key || w.count("current") != 0 || w.objects() != before {
		t.Fatalf("after the refusals pending = %v, %d current Parts, %d stored objects (was %d), want it all as it was", got, w.count("current"), w.objects(), before)
	}
	for _, date := range []string{"1900-01-01", "2100-12-31"} {
		w := newWorld(t)
		w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
		if _, err := w.confirmOn(date, nil, nil); err != nil {
			t.Fatalf("date %q: %v, want it accepted", date, err)
		}
	}
}

func TestAConfirmThatFailsAfterTheDateChangedLeavesTheOldRowsAndNoNewObject(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 6.0})})
	before := w.objects()
	full := errors.New("storage full")
	_, err := w.confirmOn("2026-06-30", func(context.Context, *sql.Tx, *dataset.Pending) error { return full }, nil)
	if !errors.Is(err, full) {
		t.Fatalf("Confirm = %v, want the reserve's error", err)
	}
	got := w.pending()
	if got == nil || got.Key != p.Key || !slices.Equal(w.dates(got), []string{uploadDay}) || w.objects() != before || w.queued(p.Key) != 0 {
		t.Fatalf("after the failure pending = %+v, %d stored objects (was %d), want the old rows, no new object and none queued", got, w.objects(), before)
	}
}

// monthColumns are the month columns the Dashboard's recipe records.
func (w *world) monthColumns() []string {
	w.t.Helper()
	return w.recipe().MonthColumns
}

func TestAFirstPartsOwnValuesDecideTheMonthColumns(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart() // Bulan is 6 in both rows; Jumlah is not 1 to 12
	if got := w.monthColumns(); !slices.Equal(got, []string{"Bulan"}) {
		t.Fatalf("month columns = %v, want Bulan", got)
	}
}

func TestAConfirmedPartWithANumberOutsideOneToTwelveInAMonthColumnEndsIt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 13.0})}))
	if got := w.monthColumns(); got == nil || len(got) != 0 {
		t.Fatalf("month columns = %#v, want none, recorded as found", got)
	}
	// A later Part that writes a month name there gets it left empty, as in a column that never was a month column.
	p := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, "Ogos"})})
	if c := changeOf(p, "lab.clean.not-numbers"); c == nil || c.Column != "Bulan" || changeOf(p, "lab.clean.month-names") != nil {
		t.Fatalf("changes = %+v, want the name left empty in Bulan and not read as a month", p.Changes)
	}
}

func TestADatasetFromBeforeMonthColumnsGetsThemFoundByTheFirstAddAndConfirmedWithIt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.forgetMonthColumns()
	if got := w.monthColumns(); got != nil {
		t.Fatalf("month columns = %#v, want never found", got)
	}
	p := w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, "Ogos"})})
	if c := changeOf(p, "lab.clean.month-names"); c == nil || c.Column != "Bulan" {
		t.Fatalf("changes = %+v, want the month name read in Bulan", p.Changes)
	}
	w.confirm(p)
	if got := w.monthColumns(); !slices.Equal(got, []string{"Bulan"}) {
		t.Fatalf("month columns after the confirm = %v, want Bulan recorded", got)
	}
}

func TestADatasetWithNoMonthColumnGetsNoneRecordedByTheFirstAdd(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 13.0})}))
	w.forgetMonthColumns()
	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 14.0})}))
	if got := w.monthColumns(); got == nil || len(got) != 0 {
		t.Fatalf("month columns = %#v, want none, recorded as found", got)
	}
}

func TestReplacingTheOnlyPartLetsTheNewPartDecideTheMonthColumns(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: shop(t, []any{"Beras", 120.0, 13.0})}))
	if got := w.monthColumns(); got == nil || len(got) != 0 {
		t.Fatalf("month columns = %#v, want none", got)
	}
	w.forgetMonthColumns()
	p := w.put(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, "Ogos"})})
	if c := changeOf(p, "lab.clean.month-names"); c == nil || c.Column != "Bulan" {
		t.Fatalf("changes = %+v, want the month name read in Bulan", p.Changes)
	}
	w.confirm(p)
	if got := w.monthColumns(); !slices.Equal(got, []string{"Bulan"}) {
		t.Fatalf("month columns after the Replace = %v, want Bulan from the new Part's values", got)
	}
	if got := w.rows(p); !reflect.DeepEqual(got, [][]any{{"Garam", 70.0, 8.0}}) {
		t.Fatalf("rows = %v, want Ogos stored as 8", got)
	}
}

func TestAFirstPartAndReplaceDataSetTheRecipeAndAnAddKeepsIt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	titled := func(item string) []byte {
		return testutil.XLSX(t, []string{"Kedai Ali"}, []any{"Perkara", "Jumlah", "Bulan"}, []any{item, 100.0, 6.0})
	}
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: titled("Beras"), settings: lab.Settings{HeaderRow: 2}})
	if w.recipe() != nil {
		t.Fatal("the recipe is set before the first Part is confirmed")
	}
	w.confirm(p)
	r := w.recipe()
	if r == nil || r.Sheet != p.PickedSheet || r.HeaderRow != 2 || !r.HeaderTyped || !slices.Equal(r.MonthColumns, []string{"Bulan"}) {
		t.Fatalf("recipe = %+v, want the first Part's sheet, the typed header row 2 and Bulan", r)
	}

	w.confirm(w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: titled("Gula")}))
	if again := w.recipe(); again.HeaderRow != 2 || !again.HeaderTyped || again.Sheet != r.Sheet {
		t.Fatalf("recipe after an Add = %+v, want it as it was", again)
	}

	// Replace data reads the file afresh and sets a new recipe: the header row is found, not typed.
	w.confirm(w.put(upload{kind: dataset.ReplaceData, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, 8.0})}))
	if fresh := w.recipe(); fresh.HeaderRow == 2 || fresh.HeaderTyped {
		t.Fatalf("recipe after Replace data = %+v, want the new file's own", fresh)
	}
}

func TestAConfirmThatLostToANewerUploadIsErrLostRaceAndChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})})
	var newer *dataset.Pending
	_, err := w.confirmOn("", func(context.Context, *sql.Tx, *dataset.Pending) error {
		// The Learner uploads again after Confirm read the pending Part and before it locks it.
		newer = w.put(upload{kind: dataset.Add, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, 8.0})})
		return nil
	}, nil)
	if !errors.Is(err, dataset.ErrLostRace) {
		t.Fatalf("Confirm = %v, want ErrLostRace", err)
	}
	if got := w.pending(); got == nil || got.ID != newer.ID || !slices.Equal(w.names(), []string{"Beras", "Gula"}) {
		t.Fatalf("after the lost race pending = %+v, Dataset %v, want the newer upload pending and the Dataset as it was", got, w.names())
	}
	if _, err := w.confirmOn("", nil, nil); err != nil || !slices.Equal(w.names(), []string{"Beras", "Gula", "Garam"}) {
		t.Fatalf("confirming the newer upload = %v, Dataset %v", err, w.names())
	}
}

func TestAConfirmThatLostToADeleteOfTheReplacedPartIsErrLostRaceAndChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june, _, _ := w.threeParts()
	w.put(upload{kind: dataset.ReplacePart, replaces: june, name: "jun-betul.xlsx", file: shop(t, []any{"Kicap", 90.0, 6.0})})
	_, err := w.confirmOn("", func(ctx context.Context, _ *sql.Tx, _ *dataset.Pending) error {
		return w.ds.Delete(ctx, w.dashboard, june)
	}, nil)
	if !errors.Is(err, dataset.ErrLostRace) {
		t.Fatalf("Confirm = %v, want ErrLostRace", err)
	}
	if got := w.names(); !slices.Equal(got, []string{"Tepung", "Garam"}) || w.pending() != nil {
		t.Fatalf("after the lost race Dataset = %v, pending %v, want the delete's result and no Part waiting", got, w.pending())
	}
}

func TestAConfirmWithNothingPendingIsErrLostRace(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	if _, err := w.confirmOn("", nil, nil); !errors.Is(err, dataset.ErrLostRace) {
		t.Fatalf("Confirm of nothing = %v, want ErrLostRace", err)
	}
}

func TestAFailingHookRollsTheWholeConfirmBack(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june, _, _ := w.threeParts()
	w.addPart("before-parts", "earlier", 0, 1)
	oldKeys, _ := w.keys()
	oldColumns := columnNames(w.recipe().Columns)
	broken := errors.New("blocks could not be built")

	for name, u := range map[string]upload{
		"Replace data":     {kind: dataset.ReplaceData, name: "markah.xlsx", file: testutil.XLSX(t, []string{"Nama", "Markah"}, []any{"Ali", 80.0})},
		"Replace one Part": {kind: dataset.ReplacePart, replaces: june, name: "jun-betul.xlsx", file: shop(t, []any{"Kicap", 90.0, 6.0})},
		"Add":              {kind: dataset.Add, name: "sep.xlsx", file: shop(t, []any{"Minyak", 50.0, 9.0})},
	} {
		p := w.put(u)
		objects := w.objects()
		_, err := w.confirmOn("2026-06-30", nil, func(ctx context.Context, tx *sql.Tx, c *dataset.Confirmed) error {
			var state string
			if err := tx.QueryRowContext(ctx, `SELECT state FROM dataset_parts WHERE id = $1`, c.ID).Scan(&state); err != nil || state != "current" {
				t.Errorf("%s: the Part is %q (%v) in the hook's transaction, want it already current", name, state, err)
			}
			if w.count("current") != 3 { // seen from outside the transaction
				t.Errorf("%s: %d current Parts outside the transaction, want it uncommitted", name, w.count("current"))
			}
			return broken
		})
		if !errors.Is(err, broken) {
			t.Fatalf("%s: Confirm = %v, want the hook's error", name, err)
		}
		keys, _ := w.keys()
		got := w.pending()
		if !slices.Equal(keys, oldKeys) || w.count("earlier") != 1 || got == nil || got.Key != p.Key || !slices.Equal(columnNames(w.recipe().Columns), oldColumns) ||
			w.objects() != objects || w.queued(p.Key) != 0 || w.queued(oldKeys[0]) != 0 {
			t.Fatalf("%s: after the failed hook keys %v (were %v), %d earlier Parts, pending %+v, columns %v, %d objects (were %d): want everything as it was",
				name, keys, oldKeys, w.count("earlier"), got, columnNames(w.recipe().Columns), w.objects(), objects)
		}
	}
}

func TestTheHookIsGivenTheConfirmedDataset(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	w.put(upload{kind: dataset.Add, name: "julai.xlsx", file: shop(t, []any{"Tepung", 60.0, 7.0})})
	var saw *dataset.Confirmed
	c, err := w.confirmOn("", nil, func(_ context.Context, _ *sql.Tx, c *dataset.Confirmed) error { saw = c; return nil })
	if err != nil || saw != c || !slices.Equal(columnNames(saw.Columns), []string{"Perkara", "Jumlah", "Bulan"}) || !reflect.DeepEqual(saw.Dataset.Rows, [][]any{{"Tepung", 60.0, 7.0}}) {
		t.Fatalf("hook saw %+v (confirm %v), want the confirmed Part's columns and rows", saw, err)
	}
}

func TestTheBytesAConfirmFreesAreTheBytesOfThePartsItDeletes(t *testing.T) {
	t.Parallel()
	for name, u := range map[string]func(june int64) upload{
		"Add": func(int64) upload {
			return upload{kind: dataset.Add, name: "sep.xlsx", file: shop(t, []any{"Minyak", 50.0, 9.0})}
		},
		"Replace one Part": func(june int64) upload {
			return upload{kind: dataset.ReplacePart, replaces: june, name: "jun-betul.xlsx", file: shop(t, []any{"Kicap", 90.0, 6.0})}
		},
		"Replace data": func(int64) upload {
			return upload{kind: dataset.ReplaceData, name: "sep.xlsx", file: shop(t, []any{"Minyak", 50.0, 9.0})}
		},
	} {
		w := newWorld(t)
		june, _, _ := w.threeParts()
		w.addPart("before-parts", "earlier", 0, 1)
		w.exec(nil, `UPDATE dataset_parts SET saved_size = 7 WHERE state = 'earlier'`)
		p := w.put(u(june))
		var present = func() map[int64]int64 { // the Parts that are not the pending one, with their sizes
			out := map[int64]int64{}
			rows, err := w.db.Query(`SELECT id, saved_size FROM dataset_parts WHERE dashboard_id = $1 AND id <> $2`, w.dashboard, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var id, size int64
				if err := rows.Scan(&id, &size); err != nil {
					t.Fatal(err)
				}
				out[id] = size
			}
			return out
		}
		before := present()

		var freed []dataset.Freed
		var added int64
		if _, err := w.confirmOn("", func(ctx context.Context, tx *sql.Tx, p *dataset.Pending) (err error) {
			added = p.Size
			freed, err = dataset.Frees(ctx, tx, w.dashboard, p.Kind, p.ReplacedPart)
			return err
		}, nil); err != nil {
			t.Fatal(err)
		}
		after := present()
		deleted := map[int64]int64{}
		for id, size := range before {
			if _, ok := after[id]; !ok {
				deleted[id] = size
			}
		}
		saw := map[int64]int64{}
		for _, f := range freed {
			saw[f.ID] = f.Size
		}
		if !reflect.DeepEqual(saw, deleted) || len(deleted) == 0 || added != p.Size {
			t.Fatalf("%s: the reserve was told %v freed and %d added, the confirm deleted %v and added %d", name, saw, added, deleted, p.Size)
		}
	}
}

func TestTheReserveRunsBeforeAnyRowIsLocked(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	june := w.junePart()
	w.put(upload{kind: dataset.ReplacePart, replaces: june.ID, name: "jun-betul.xlsx", file: shop(t, []any{"Kicap", 90.0, 6.0})})
	ran := false
	_, err := w.confirmOn("", func(context.Context, *sql.Tx, *dataset.Pending) error {
		ran = true
		// From another connection, a lock is taken at once only if Confirm's transaction holds none of these.
		if _, err := w.db.Exec(`SELECT 1 FROM learners WHERE id = $1 FOR UPDATE NOWAIT`, w.learner); err != nil {
			t.Errorf("the Learner is locked when the reserve runs: %v", err)
		}
		if _, err := w.db.Exec(`SELECT 1 FROM dataset_parts WHERE dashboard_id = $1 FOR UPDATE NOWAIT`, w.dashboard); err != nil {
			t.Errorf("a Part is locked when the reserve runs: %v", err)
		}
		return nil
	}, nil)
	if err != nil || !ran {
		t.Fatalf("Confirm = %v (reserve ran: %v)", err, ran)
	}
}

func TestAReserveThatRefusesChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.junePart()
	p := w.put(upload{kind: dataset.ReplaceData, name: "ogos.xlsx", file: shop(t, []any{"Garam", 70.0, 8.0})})
	full := errors.New("full")
	if _, err := w.confirmOn("", func(context.Context, *sql.Tx, *dataset.Pending) error { return full }, nil); !errors.Is(err, full) {
		t.Fatalf("Confirm = %v, want the reserve's error", err)
	}
	if got := w.pending(); got == nil || got.ID != p.ID || !slices.Equal(w.names(), []string{"Beras", "Gula"}) {
		t.Fatalf("after the refusal pending = %+v, Dataset %v, want the Part still pending and the Dataset as it was", got, w.names())
	}
}

func TestRoutineIsWhetherCleanUpMadeOnlyKindsOfChangeThatTheLatestPartHad(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	if ok, err := w.ds.Routine(ctx, w.dashboard, nil); err != nil || !ok {
		t.Fatalf("Routine with no Part and no changes = %v, %v, want true", ok, err)
	}
	if ok, err := w.ds.Routine(ctx, w.dashboard, []lab.Change{{Key: "lab.clean.title-row"}}); err != nil || ok {
		t.Fatalf("Routine with no Part and a change = %v, %v, want false", ok, err)
	}
	titled := testutil.XLSX(t, []string{"Kedai Ali"}, []any{"Perkara", "Jumlah", "Bulan"}, []any{"Beras", 100.0, 6.0})
	p := w.put(upload{kind: dataset.ReplaceData, name: "jun.xlsx", file: titled})
	w.confirm(p)
	if len(p.Changes) == 0 {
		t.Fatalf("Clean-up made no change to a file with a title row: %+v", p)
	}
	own := p.Changes[:1]
	if ok, err := w.ds.Routine(ctx, w.dashboard, own); err != nil || !ok {
		t.Fatalf("Routine with a kind the Part had = %v, %v, want true", ok, err)
	}
	if ok, err := w.ds.Routine(ctx, w.dashboard, []lab.Change{{Key: "lab.clean.something-else"}}); err != nil || ok {
		t.Fatalf("Routine with another kind = %v, %v, want false", ok, err)
	}
}
