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

func TestCleanUpWithARecipeUsesItsHeaderRowWhenTheLearnerTypedIt(t *testing.T) {
	recipe := lab.Recipe{Sheet: "Data", HeaderRow: 3, HeaderTyped: true, Columns: salesColumns}
	// Row 2 looks like a header of its own; the recipe says the header is row 3.
	file := testutil.Workbook(t, sheet("Data", row("Laporan"), row("Perkara", "Jumlah"), row("Perkara", "Jumlah"), row("Kopi", 5)))
	res := cleanWith(t, file, lab.Settings{Recipe: &recipe})
	if want := [][]any{{"Kopi", 5.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) || res.HeaderRow != 3 {
		t.Fatalf("rows = %v, header = %d, want %v read from row 3", res.Dataset.Rows, res.HeaderRow, want)
	}
}

// A header row Cave found for the first Part is not reused: every Part's is found again, as its title rows may differ.
func TestCleanUpWithARecipeFindsTheHeaderRowAgainWhenTheLearnerDidNotTypeIt(t *testing.T) {
	recipe := lab.Recipe{Sheet: "Data", HeaderRow: 3, Columns: salesColumns}
	file := testutil.Workbook(t, sheet("Data", row("Laporan"), row("Perkara", "Jumlah"), row("Kopi", 5)))
	res := cleanWith(t, file, lab.Settings{Recipe: &recipe})
	if want := [][]any{{"Kopi", 5.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) || res.HeaderRow != 2 {
		t.Fatalf("rows = %v, header = %d, want %v read from the header Cave found on row 2", res.Dataset.Rows, res.HeaderRow, want)
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

// A sheet with extra columns is offered, and picked, when it has every one of the Dataset's columns.
func TestCleanUpWithARecipeOffersTheSheetsThatHaveEveryDatasetColumnWhateverElseTheyHave(t *testing.T) {
	recipe := lab.Recipe{Sheet: "Data asal", Columns: salesColumns}
	wide := sheet("Jan", row("Nota", "Jumlah", "Perkara"), row("x", 5, "Kopi"))
	narrow := sheet("Feb", row("Perkara"), row("Teh"))
	res := cleanWith(t, testutil.Workbook(t, narrow, wide), lab.Settings{Recipe: &recipe})
	var fits []string
	for _, s := range res.Sheets {
		if s.Fits {
			fits = append(fits, s.Name)
		}
	}
	if !reflect.DeepEqual(fits, []string{"Jan"}) || res.PickedSheet != "Jan" {
		t.Fatalf("fits = %v, picked %q, want only Jan", fits, res.PickedSheet)
	}
}

// A Dataset that Cave built from several sheets has a Sheet or Date column no single sheet has: a Part read from one
// sheet gets it from that sheet's name, so it is not refused as missing a column.
func TestCleanUpWithARecipeFillsCavesSheetAndDateColumnsFromTheSheetsName(t *testing.T) {
	bySheet := lab.Recipe{Sheet: "Jan", Columns: append([]lab.Column{{Name: "Sheet", Type: lab.Text}}, salesColumns...)}
	res := cleanWith(t, testutil.Workbook(t, sales("Jan", 2)), lab.Settings{Recipe: &bySheet})
	if want := [][]any{{"Jan", "Item 1", 10.0}, {"Jan", "Item 2", 20.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if !res.Sheets[0].Fits {
		t.Fatal("the sheet does not fit a Dataset with Cave's Sheet column")
	}

	byDate := lab.Recipe{Sheet: "01.09.26", Columns: append([]lab.Column{{Name: "Date", Type: lab.Date}}, salesColumns...)}
	res = cleanWith(t, testutil.Workbook(t, sales("02.09.26", 1)), lab.Settings{Recipe: &byDate})
	if want := [][]any{{"2026-09-02", "Item 1", 10.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}

	// A Date column and a sheet name that is no date: nothing to fill it from, so the sheet does not fit.
	res = cleanWith(t, testutil.Workbook(t, sales("Jan", 1)), lab.Settings{Recipe: &byDate})
	if res.Sheets[0].Fits {
		t.Fatal("a sheet named Jan fits a Date column")
	}
}

func TestCleanUpWithoutARecipeStillCombinesAndSaysWhichSheetItPicked(t *testing.T) {
	res := cleanWith(t, testutil.Workbook(t, sales("Jan", 1), sales("Feb", 1)), lab.Settings{})
	if !reflect.DeepEqual(res.Used, []string{"Jan", "Feb"}) || res.PickedSheet != "Jan" {
		t.Fatalf("used = %v, picked = %q, want both sheets with Jan picked", res.Used, res.PickedSheet)
	}
}
