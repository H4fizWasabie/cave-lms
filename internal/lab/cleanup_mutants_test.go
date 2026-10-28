package lab_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// cleanErr is the Problem Clean gives for a file read with s.
func cleanErr(t *testing.T, file []byte, s lab.Settings) lab.Problem {
	t.Helper()
	_, err := lab.Clean(bytes.NewReader(file), s)
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want a Problem", err)
	}
	return p
}

func TestCleanUpReadsAWorkbookOfExactly10000MergedRanges(t *testing.T) {
	var merges strings.Builder
	for i := range lab.MaxMerges {
		fmt.Fprintf(&merges, `<mergeCell ref="E%d:F%d"/>`, 10+2*i, 11+2*i)
	}
	file := handMade(t, "xl/worksheets/sheet1.xml", groupedRows, fmt.Sprintf(`<mergeCells count="%d">%s</mergeCells>`, lab.MaxMerges, merges.String()))
	if res := clean(t, file); len(res.Dataset.Rows) != 2 {
		t.Fatalf("%d rows, want 2", len(res.Dataset.Rows))
	}
}

func TestCleanUpReadsAPartFromTheRecipesSheetOrTheLearnersNotTheFirstThatFits(t *testing.T) {
	cols := []lab.Column{{Name: "Perkara", Type: lab.Text}, {Name: "Jumlah", Type: lab.Number}}
	file := testutil.Workbook(t,
		sheet("Lain", row("X", "Y"), row("a", 1)),
		sheet("Jualan", row("Perkara", "Jumlah"), row("Beras", 5)),
	)
	used := func(s lab.Settings) []string { return cleanWith(t, file, s).Used }

	// The Learner's sheet, the first, wins over the Recipe's.
	if got := used(lab.Settings{Sheet: "Lain", Recipe: &lab.Recipe{Sheet: "Jualan", Columns: cols}}); !reflect.DeepEqual(got, []string{"Lain"}) {
		t.Errorf("the Learner's first sheet: used = %v, want [Lain]", got)
	}
	// The Recipe's sheet by name, whatever its case, even when it is the first and another sheet has the columns.
	if got := used(lab.Settings{Recipe: &lab.Recipe{Sheet: "lain", Columns: cols}}); !reflect.DeepEqual(got, []string{"Lain"}) {
		t.Errorf("the Recipe's first sheet: used = %v, want [Lain]", got)
	}
	// With no sheet of that name, the first with the Dataset's columns.
	if got := used(lab.Settings{Recipe: &lab.Recipe{Sheet: "Hilang", Columns: cols}}); !reflect.DeepEqual(got, []string{"Jualan"}) {
		t.Errorf("no sheet by the Recipe's name: used = %v, want [Jualan]", got)
	}
}

func TestCleanUpReadsFromTheHeaderRowTheLearnerNamesEvenRow1(t *testing.T) {
	file := testutil.Workbook(t, sheet("Data", row("Laporan"), row("Perkara", "Jumlah"), row("Sewa", 1000)))
	if res := clean(t, file); res.HeaderRow != 2 {
		t.Fatalf("found header row %d, want 2", res.HeaderRow)
	}
	// Row 1 is a title, not names for both columns: the Learner's choice is not replaced by the row Cave would find.
	if p := cleanErr(t, file, lab.Settings{HeaderRow: 1}); p.Key != "lab.err.header-unnamed" {
		t.Fatalf("problem = %v, want lab.err.header-unnamed", p)
	}
}

func TestCleanUpGivesNoRowsAtTheHeaderWhenEveryRowBelowItIsBlank(t *testing.T) {
	file := testutil.Workbook(t, sheet("Data", row("Laporan"), row("Perkara", "Jumlah"), row(" ", " ")))
	p := cleanErr(t, file, lab.Settings{HeaderRow: 2})
	if p.Key != "lab.err.no-rows" || !reflect.DeepEqual(p.Args, []any{2}) {
		t.Fatalf("problem = %v %v, want lab.err.no-rows for row 2", p.Key, p.Args)
	}
}

func TestCleanUpRefusesAPartDateColumnInTheFirstPlaceToo(t *testing.T) {
	file := testutil.Workbook(t, sheet("Data", row("Part date", "Perkara"), row("2026-01-01", "Beras")))
	if p := cleanErr(t, file, lab.Settings{}); p.Key != "lab.err.part-date-column" {
		t.Fatalf("problem = %v, want lab.err.part-date-column", p)
	}
}

func TestCleanUpKeepsSheetsWhoseColumnsWithTheSheetColumnCameToExactly100(t *testing.T) {
	wide, data := make([]any, 99), make([]any, 99)
	for i := range wide {
		wide[i], data[i] = fmt.Sprintf("C%d", i+1), i
	}
	res := cleanWith(t, testutil.Workbook(t, sheet("A", wide, data), sheet("B", wide, data)), lab.Settings{})
	if len(res.Dataset.Columns) != 100 || res.Dataset.Columns[0].Name != "Sheet" {
		t.Fatalf("%d columns, the first %q; want 100 and Sheet", len(res.Dataset.Columns), res.Dataset.Columns[0].Name)
	}
}

func TestCleanUpAddsUpTheCountedChangesOfSheetsThatAreCombined(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "A")
	f.NewSheet("B")
	for _, s := range []string{"A", "B"} {
		f.SetSheetRow(s, "A1", &[]any{"Kategori", "Jumlah"})
		f.SetSheetRow(s, "A2", &[]any{"Minuman", 5})
		f.SetCellValue(s, "B3", 4)
		f.SetSheetRow(s, "A4", &[]any{"Makanan", 6})
		f.SetCellValue(s, "B5", 7)
		if err := f.MergeCell(s, "A2", "A3"); err != nil {
			t.Fatal(err)
		}
		if err := f.MergeCell(s, "A4", "A5"); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	res := clean(t, b.Bytes())
	if want := []lab.Change{{Key: "lab.clean.merged-down", Args: []int{4}}, {Key: "lab.clean.combined", Args: []int{2}}}; !reflect.DeepEqual(res.Changes, want) {
		t.Fatalf("changes = %v, want %v", res.Changes, want)
	}
}

func TestCleanUpListsTheTotalRowDroppedFromTwoSheetsAsOneChangeForBoth(t *testing.T) {
	total := func(name string) testutil.Sheet {
		return sheet(name, row("Perkara", "Jumlah"), row("Beras", 5), row("Gula", 6), row("Total", 11))
	}
	res := clean(t, testutil.Workbook(t, total("A"), total("B")))
	want := "Dropped the rows at the bottom of each of 2 sheets: totals or notes, not data."
	if got := changesIn(res, "en"); !slices.Contains(got, want) {
		t.Fatalf("changes = %q, want %q among them", got, want)
	}
}

func TestCleanUpTypesAYearColumnAsTextEvenWithABlankInIt(t *testing.T) {
	file := testutil.Workbook(t, sheet("Data", row("Tahun", "Jumlah"), row(2024, 5), row(2025, 6), row(nil, 7)))
	if got := clean(t, file).Dataset.Columns[0]; got.Type != lab.Text {
		t.Fatalf("column = %v, want Text", got)
	}
}

func TestCleanUpKeepsSmallNumbersInADateNamedColumnAsNumbers(t *testing.T) {
	file := testutil.Workbook(t, sheet("Data", row("Tarikh", "Jumlah"), row(5, 1), row(10, 2)))
	res := clean(t, file)
	if got := res.Dataset.Columns[0]; got.Type != lab.Number {
		t.Fatalf("column = %v, want Number: 5 and 10 are not date serials", got)
	}
	if len(res.Changes) != 0 {
		t.Fatalf("changes = %v, want none", changesIn(res, "en"))
	}
}

func TestCleanUpKeepsToTheRowLimitExactlyAndNamesTheSheetOverIt(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 50,001-row workbook")
	}
	// A has 50,001 rows under its header; reading its row 2 or 3 as the header leaves 50,000 or 49,999. B has the
	// columns A's row 3 gives, and one row under them.
	file := testutil.Workbook(t, sales("A", 50001), sheet("B", row("t"), row("t"), row("Item 2", 20), row("z", 5)))

	if got := cleanErr(t, file, lab.Settings{}).In("en"); !strings.Contains(got, "50,001 rows") || strings.Contains(got, "ticked") {
		t.Errorf("A alone over the limit: problem = %q, want the one sheet's 50,001 rows", got)
	}
	if res := cleanWith(t, file, lab.Settings{HeaderRow: 2}); len(res.Dataset.Rows) != 50000 {
		t.Errorf("A alone at the limit: %d rows, want 50,000", len(res.Dataset.Rows))
	}
	// 49,999 of A and 1 of B: the 50,000 the sheets come to together are allowed.
	if res := cleanWith(t, file, lab.Settings{HeaderRow: 3}); len(res.Dataset.Rows) != 50000 || !reflect.DeepEqual(res.Used, []string{"A", "B"}) {
		t.Errorf("A and B at the limit: %d rows from %v, want 50,000 from A and B", len(res.Dataset.Rows), res.Used)
	}
}
