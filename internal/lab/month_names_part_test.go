package lab_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// The real case: June and July have BULAN as 6 and 7, August writes OGOS on every row. The first Part's cleaning records
// BULAN as a month column (the amount column, 1 to 12 or not, is told apart only by its values); the added Part, cleaned
// under the recipe, gives BULAN = 8 on every row once added, with the note.
func TestAddingAPartOfMonthNamesToADatasetWhoseMonthColumnHeldNumbers(t *testing.T) {
	first := clean(t, testutil.XLSX(t, []string{"KELAS", "BULAN", "JUMLAH"}, []any{"1 Amanah", 6, 120.5}, []any{"2 Bestari", 7, 99}))
	if want := []string{"BULAN"}; !reflect.DeepEqual(first.MonthColumns, want) {
		t.Fatalf("month columns = %v, want %v", first.MonthColumns, want)
	}
	recipe := lab.Recipe{Sheet: "Sheet1", Columns: first.Dataset.Columns, MonthColumns: first.MonthColumns}
	// 21 of 22 cells in BULAN are numbers: a mix that Clean-up alone would type as numbers and lose the name from.
	rows := [][]any{{"1 Amanah", "OGOS", 10}}
	for range 20 {
		rows = append(rows, []any{"x", 8, 10})
	}
	august := cleanWith(t, testutil.XLSX(t, []string{"KELAS", "BULAN", "JUMLAH"}, rows...), lab.Settings{Recipe: &recipe})
	if got := changesIn(august, "en"); len(got) != 0 {
		t.Fatalf("changes = %q, want none before the Part is added", got)
	}
	out, changes, err := lab.AddPart(first.Dataset.Columns, 2, august.Dataset, lab.MaxCleanRows, recipe.MonthColumns...)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range out.Rows {
		if r[1] != 8.0 {
			t.Fatalf("row %d BULAN = %v, want 8", i, r[1])
		}
	}
	if want := []lab.Change{{Key: "lab.clean.month-names", Args: []int{1}, Column: "BULAN"}}; !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
}

// A first Part's column of amounts that happens to hold 1 to 12 only is recorded too, by its values alone: the rule is
// the values', never the column's name. What keeps a stray month name from reading is the column holding 13 or more.
func TestAFirstPartRecordsEveryColumnOfWholeOneToTwelveRegardlessOfItsName(t *testing.T) {
	res := clean(t, testutil.XLSX(t, []string{"Kelas", "Markah", "Jumlah"}, []any{"a", 3, 130}, []any{"b", 12, 7}))
	if want := []string{"Markah"}; !slices.Equal(res.MonthColumns, want) {
		t.Fatalf("month columns = %v, want %v", res.MonthColumns, want)
	}
}
