package lab_test

import (
	"reflect"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

var salesColumns = []lab.Column{{"Perkara", lab.Text}, {"Jumlah", lab.Number}}

func TestCleanUpWithARecipeReadsOnlyItsSheetFoundIgnoringCase(t *testing.T) {
	recipe := lab.Recipe{Sheet: "DATA ASAL", Columns: salesColumns}
	file := testutil.Workbook(t, sales("Data asal", 3), sales("Subset", 2))
	res := cleanWith(t, file, lab.Settings{Recipe: &recipe})
	if got := len(res.Dataset.Rows); got != 3 {
		t.Fatalf("rows = %d, want only the recipe sheet's 3", got)
	}
	if !reflect.DeepEqual(res.Used, []string{"Data asal"}) || res.PickedSheet != "Data asal" {
		t.Fatalf("used = %v, picked = %q, want just Data asal", res.Used, res.PickedSheet)
	}
	for _, s := range res.Sheets {
		if s.CanCombine {
			t.Fatalf("sheet %s can combine under a recipe", s.Name)
		}
	}
}

func TestCleanUpWithARecipeUsesItsHeaderRow(t *testing.T) {
	recipe := lab.Recipe{Sheet: "Data", HeaderRow: 3, Columns: salesColumns}
	// Row 2 looks like a header of its own; the recipe says the header is row 3.
	file := testutil.Workbook(t, sheet("Data", row("Laporan"), row("Perkara", "Jumlah"), row("Perkara", "Jumlah"), row("Kopi", 5)))
	res := cleanWith(t, file, lab.Settings{Recipe: &recipe})
	if want := [][]any{{"Kopi", 5.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) || res.HeaderRow != 3 {
		t.Fatalf("rows = %v, header = %d, want %v read from row 3", res.Dataset.Rows, res.HeaderRow, want)
	}
}

func TestCleanUpWithARecipeWhoseSheetIsGoneOffersTheSheetsWithTheDatasetsColumns(t *testing.T) {
	recipe := lab.Recipe{Sheet: "Data asal", Columns: salesColumns}
	other := sheet("Nota", row("Kumpulan", "Perkara"), row("A", "B"), row("C", "D"))
	file := testutil.Workbook(t, other, sales("Jan", 1), sales("Feb", 2))
	res := cleanWith(t, file, lab.Settings{Recipe: &recipe})
	var fits []string
	for _, s := range res.Sheets {
		if s.Fits {
			fits = append(fits, s.Name)
		}
	}
	if want := []string{"Jan", "Feb"}; !reflect.DeepEqual(fits, want) {
		t.Fatalf("sheets that fit = %v, want %v", fits, want)
	}
	if res.PickedSheet != "Jan" || len(res.Dataset.Rows) != 1 {
		t.Fatalf("picked %q with %d rows, want the first fitting sheet Jan", res.PickedSheet, len(res.Dataset.Rows))
	}
	picked := cleanWith(t, file, lab.Settings{Recipe: &recipe, Sheet: "Feb"})
	if picked.PickedSheet != "Feb" || len(picked.Dataset.Rows) != 2 || len(picked.Used) != 1 {
		t.Fatalf("picked %q with %d rows from %v, want only Feb's 2", picked.PickedSheet, len(picked.Dataset.Rows), picked.Used)
	}
}

func TestCleanUpWithoutARecipeStillCombinesAndSaysWhichSheetItPicked(t *testing.T) {
	res := cleanWith(t, testutil.Workbook(t, sales("Jan", 1), sales("Feb", 1)), lab.Settings{})
	if !reflect.DeepEqual(res.Used, []string{"Jan", "Feb"}) || res.PickedSheet != "Jan" {
		t.Fatalf("used = %v, picked = %q, want both sheets with Jan picked", res.Used, res.PickedSheet)
	}
}
