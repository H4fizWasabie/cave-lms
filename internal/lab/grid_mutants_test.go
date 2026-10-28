package lab_test

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// gridRows is a grid's Date and Code rows as Clean-up gives them, with Staff first.
func gridRows(res *lab.CleanUp) [][]any { return res.Dataset.Rows }

func TestAGridMonthTheLearnerSetsToDecemberIsUsed(t *testing.T) {
	file := testutil.Workbook(t, roster("Jadual", "Jadual Tugas Oktober 2026", map[string]map[int]string{"Ali": {3: "P"}}))
	res := cleanWith(t, file, lab.Settings{Month: 12, Year: 2026})
	if want := [][]any{{"Ali", "2026-12-03", "P"}}; !reflect.DeepEqual(gridRows(res), want) || res.Month != 12 {
		t.Fatalf("month %d, rows %v; want 12 and %v", res.Month, gridRows(res), want)
	}
}

func TestAGridKeepsItsTitleRowChangeAndTheDateInItsTitle(t *testing.T) {
	file := testutil.Workbook(t, roster("Jadual", "Jadual Tugas 01/10/2026", map[string]map[int]string{"Ali": {3: "P"}}))
	res := cleanWith(t, file, lab.Settings{Month: 10, Year: 2026})
	if want := []string{"Skipped row 1 above the header in row 2."}; !slices.Contains(changesIn(res, "en"), want[0]) {
		t.Fatalf("changes = %q, want %q among them", changesIn(res, "en"), want)
	}
	if res.PartDate != "2026-10-01" || res.DateFrom != lab.DateFromCell || res.DateCell != "A1" {
		t.Fatalf("part date %q from %q at %q, want 2026-10-01 from the cell A1", res.PartDate, res.DateFrom, res.DateCell)
	}
}

func TestAGridSkipsBlankRowsUnderItsHeaderButEndsAtOneBelowStaff(t *testing.T) {
	file := testutil.Workbook(t, grid("Jadual", []any{"Nama"},
		[]any{},
		days([]any{"Ali"}, map[int]any{1: "P"}),
		[]any{},
		days([]any{"Siti"}, map[int]any{2: "M"}),
	))
	res := clean(t, file)
	if want := [][]any{{"Ali", "2026-10-01", "P"}}; !reflect.DeepEqual(gridRows(res), want) {
		t.Fatalf("rows = %v, want %v", gridRows(res), want)
	}
}

func TestAGridKeepsReadingPastAStaffRowWithNoCodes(t *testing.T) {
	file := testutil.Workbook(t, grid("Jadual", []any{"Nama"},
		days([]any{"Ali"}, map[int]any{1: "P"}),
		days([]any{"Siti"}, nil),
		days([]any{"Abu"}, map[int]any{3: "M"}),
	))
	res := clean(t, file)
	if want := [][]any{{"Ali", "2026-10-01", "P"}, {"Abu", "2026-10-03", "M"}}; !reflect.DeepEqual(gridRows(res), want) {
		t.Fatalf("rows = %v, want %v", gridRows(res), want)
	}
}

func TestAGridRowsKeepTheirRowNumbersInExcel(t *testing.T) {
	file := testutil.Workbook(t, roster("Jadual", "Jadual Tugas Oktober 2026", map[string]map[int]string{
		"Ali":  {1: "P", 2: "M", 31: "P"},
		"Siti": {5: "M"},
	}))
	if got, want := clean(t, file).Dataset.RowNumbers, []int{3, 3, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("row numbers = %v, want %v", got, want)
	}
}

func TestAGridWithNoStaffNamesGivesNoRowsAtItsHeaderRow(t *testing.T) {
	file := testutil.Workbook(t, grid("Jadual", []any{"Nama"}, days([]any{nil}, map[int]any{1: "Isn", 2: "Sel"})))
	_, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) || p.Key != "lab.err.no-rows" || !reflect.DeepEqual(p.Args, []any{2}) {
		t.Fatalf("err = %v, want lab.err.no-rows for row 2", err)
	}
}

func TestAGridWhoseHeaderIsInRow1HasNoTitleRows(t *testing.T) {
	header := []any{"Nama"}
	for d := 1; d <= 31; d++ {
		header = append(header, d)
	}
	file := testutil.Workbook(t, sheet("Jadual", header, days([]any{"Ali"}, map[int]any{1: "P"})))
	res := cleanWith(t, file, lab.Settings{Month: 10, Year: 2026})
	if want := [][]any{{"Ali", "2026-10-01", "P"}}; !reflect.DeepEqual(gridRows(res), want) || res.HeaderRow != 1 {
		t.Fatalf("header row %d, rows %v; want 1 and %v", res.HeaderRow, gridRows(res), want)
	}
}

func TestAGridLeavesOutDaysPastTheMonthsEnd(t *testing.T) {
	file := testutil.Workbook(t, roster("Jadual", "Jadual Tugas", map[string]map[int]string{"Ali": {28: "P", 29: "M", 30: "X", 31: "Y"}}))
	for month, want := range map[int][][]any{
		2: {{"Ali", "2026-02-28", "P"}},
		4: {{"Ali", "2026-04-28", "P"}, {"Ali", "2026-04-29", "M"}, {"Ali", "2026-04-30", "X"}},
		5: {{"Ali", "2026-05-28", "P"}, {"Ali", "2026-05-29", "M"}, {"Ali", "2026-05-30", "X"}, {"Ali", "2026-05-31", "Y"}},
	} {
		if got := gridRows(cleanWith(t, file, lab.Settings{Month: month, Year: 2026})); !reflect.DeepEqual(got, want) {
			t.Errorf("month %d: rows = %v, want %v", month, got, want)
		}
	}
}
