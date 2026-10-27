package lab_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// A first Part's column whose every filled cell is a month name becomes a number column of 1 to 12, with a note.
func TestACleanedColumnOfMonthNamesOnlyBecomesANumberColumnWithANote(t *testing.T) {
	file := testutil.XLSX(t, []string{"Kelas", "BULAN"},
		[]any{"1 Amanah", "OGOS"}, []any{"4 Dedikasi", nil}, []any{"2 Bestari", "ogos"}, []any{"3 Cekal", " Aug "},
	)
	res := clean(t, file)
	if want := []lab.Column{{"Kelas", lab.Text}, {"BULAN", lab.Number}}; !reflect.DeepEqual(res.Dataset.Columns, want) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, want)
	}
	if want := [][]any{{"1 Amanah", 8.0}, {"4 Dedikasi", nil}, {"2 Bestari", 8.0}, {"3 Cekal", 8.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if got := changesIn(res, "en"); !slices.Contains(got, "Month names in BULAN read as numbers: 3.") || len(got) != 1 {
		t.Fatalf("changes = %q, want only the month names note", got)
	}
	if got := changesIn(res, "ms"); !slices.Contains(got, "Nama bulan dalam BULAN dibaca sebagai nombor: 3.") {
		t.Fatalf("changes = %q, want the note in Malay", got)
	}
}
