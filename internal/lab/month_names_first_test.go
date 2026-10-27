package lab_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Numbers and month names together in a column of whole 1 to 12: the names are read as numbers too, and the change is
// its own, not counted among the cells left empty.
func TestAFirstPartsMonthColumnReadsMonthNamesAmongItsNumbers(t *testing.T) {
	file := testutil.XLSX(t, []string{"Kelas", "BULAN"},
		[]any{"a", 6}, []any{"b", "Julai"}, []any{"c", "JUL"}, []any{"d", "July"}, []any{"e", "Mac"}, []any{"f", "Disember"}, []any{"g", "okt"}, []any{"h", "-"},
	)
	res := clean(t, file)
	if res.Dataset.Columns[1].Type != lab.Number {
		t.Fatalf("columns = %v, want BULAN a number column", res.Dataset.Columns)
	}
	var got []any
	for _, r := range res.Dataset.Rows {
		got = append(got, r[1])
	}
	if want := []any{6.0, 7.0, 7.0, 7.0, 3.0, 12.0, 10.0, nil}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BULAN = %v, want %v", got, want)
	}
	changes := changesIn(res, "en")
	if !slices.Contains(changes, "Month names in BULAN read as numbers: 6.") {
		t.Fatalf("changes = %q, want the month names note", changes)
	}
	for _, c := range changes {
		if strings.Contains(c, "not numbers") {
			t.Fatalf("changes = %q, month names counted among the cells left empty", changes)
		}
	}
}

// Month names mixed with other text stay text; an amount column with a stray month name, or a value outside whole 1 to
// 12, is not a month column, so the name is left empty and listed as before.
func TestAFirstPartsColumnIsNotAMonthColumnWhenAValueIsNeitherAMonthNameNorWholeOneToTwelve(t *testing.T) {
	words := clean(t, testutil.XLSX(t, []string{"Kelas", "Catatan"}, []any{"a", "OGOS"}, []any{"b", "Mac"}, []any{"c", "lulus"}))
	if words.Dataset.Columns[1].Type != lab.Text || !reflect.DeepEqual(words.Dataset.Rows[0], []any{"a", "OGOS"}) || words.MonthColumns != nil {
		t.Fatalf("mixed with text: columns = %v, rows = %v, want text kept as written", words.Dataset.Columns, words.Dataset.Rows)
	}
	for _, other := range []any{13, 0, 2.5} {
		rows := [][]any{{"a", 4}, {"b", 9}, {"c", "Mac"}, {"d", other}}
		for i := 0; i < 20; i++ {
			rows = append(rows, []any{"x", 4})
		}
		amounts := clean(t, testutil.XLSX(t, []string{"Perkara", "Jumlah"}, rows...))
		if amounts.MonthColumns != nil || amounts.Dataset.Rows[2][1] != nil {
			t.Fatalf("with %v: month columns = %v, Mac = %v, want no month column and Mac left empty", other, amounts.MonthColumns, amounts.Dataset.Rows[2][1])
		}
		if changes := changesIn(amounts, "en"); !slices.Contains(changes, "Cells in Jumlah that were not numbers, left empty: 1.") {
			t.Fatalf("with %v: changes = %q, want Mac listed as left empty", other, changes)
		}
	}
}
