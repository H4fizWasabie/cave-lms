package lab_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

var attendanceColumns = []lab.Column{{Name: "KELAS", Type: lab.Text}, {Name: "BULAN", Type: lab.Number}, {Name: "JUMLAH", Type: lab.Number}}

// An added Part writes BULAN as text, OGOS on every row: under the Dataset's month column it is 8 on every row, with a
// note of its own that is not counted among the cells left empty.
func TestAnAddedPartsMonthNamesAreReadAsNumbersInTheDatasetsMonthColumn(t *testing.T) {
	more := &lab.Dataset{
		Columns: []lab.Column{{Name: "KELAS", Type: lab.Text}, {Name: "BULAN", Type: lab.Text}, {Name: "JUMLAH", Type: lab.Number}},
		Rows:    [][]any{{"1 Amanah", "OGOS", 10.0}, {"2 Bestari", "ogos", 12.0}, {"3 Cekal", " Aug ", 11.0}, {"4 Dedikasi", "8", 9.0}, {"5 Efisien", nil, 9.0}},
	}
	out, changes, err := lab.AddPart(attendanceColumns, 2, more, lab.MaxCleanRows, []string{"BULAN"})
	if err != nil {
		t.Fatal(err)
	}
	var got []any
	for _, r := range out.Rows {
		got = append(got, r[1])
	}
	if want := []any{8.0, 8.0, 8.0, 8.0, nil}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BULAN = %v, want %v", got, want)
	}
	if want := []lab.Change{{Key: "lab.clean.month-names", Args: []int{3}, Column: "BULAN"}}; !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
}

// The Dataset's month columns are found by name as sheets are matched, and a Part whose own numbers are not all whole 1
// to 12, or a column the Dataset did not record as a month column, leaves a month name empty and lists it as before.
func TestAnAddedPartsMonthNameStaysEmptyWhereTheColumnIsNotAMonthColumn(t *testing.T) {
	part := func(bulan ...any) *lab.Dataset {
		more := &lab.Dataset{Columns: []lab.Column{{Name: "KELAS", Type: lab.Text}, {Name: "BULAN", Type: lab.Text}, {Name: "JUMLAH", Type: lab.Text}}}
		for _, b := range bulan {
			more.Rows = append(more.Rows, []any{"x", b, "Mac"})
		}
		return more
	}
	for name, c := range map[string]struct {
		more   *lab.Dataset
		months []string
	}{
		"not recorded":       {part("OGOS"), nil},
		"another column":     {part("OGOS"), []string{"KELAS"}},
		"a number beyond 12": {part("OGOS", "13"), []string{"BULAN"}},
		"a fraction":         {part("OGOS", "2.5"), []string{"BULAN"}},
	} {
		out, changes, err := lab.AddPart(attendanceColumns, 1, c.more, 0, c.months)
		if err != nil {
			t.Fatal(name, err)
		}
		if out.Rows[0][1] != nil {
			t.Fatalf("%s: BULAN = %v, want OGOS left empty", name, out.Rows[0][1])
		}
		var texts []string
		for _, ch := range changes {
			texts = append(texts, ch.In("en"))
		}
		if !slices.Contains(texts, "Cells in BULAN that were not numbers, left empty: 1.") || slices.ContainsFunc(texts, func(s string) bool { return strings.Contains(s, "Month names") }) {
			t.Fatalf("%s: changes = %q, want the month names left empty and listed as such", name, texts)
		}
	}
}

// JUMLAH is a number column too, but a month name in it is never read, even when the Dataset's BULAN is a month column.
func TestAnAddedPartsAmountColumnNeverReadsAMonthName(t *testing.T) {
	more := &lab.Dataset{
		Columns: []lab.Column{{Name: "KELAS", Type: lab.Text}, {Name: "BULAN", Type: lab.Number}, {Name: "JUMLAH", Type: lab.Text}},
		Rows:    [][]any{{"x", 8.0, "Mac"}, {"y", 8.0, "Jun"}, {"z", 8.0, "5"}},
	}
	out, changes, err := lab.AddPart(attendanceColumns, 1, more, 0, []string{"BULAN"})
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"x", 8.0, nil}, {"y", 8.0, nil}, {"z", 8.0, 5.0}}; !reflect.DeepEqual(out.Rows, want) {
		t.Fatalf("rows = %v, want %v", out.Rows, want)
	}
	if len(changes) != 1 || changes[0].Key != "lab.clean.not-numbers" || changes[0].Args[0] != 2 || changes[0].Column != "JUMLAH" {
		t.Fatalf("changes = %v, want JUMLAH's 2 cells left empty", changes)
	}
}
