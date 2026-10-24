package lab_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func clean(t *testing.T, file []byte, tpl *lab.Template) *lab.CleanUp {
	t.Helper()
	res, err := lab.Clean(bytes.NewReader(file), tpl, lab.Settings{})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	return res
}

func changesIn(res *lab.CleanUp, lang string) []string {
	var out []string
	for _, c := range res.Changes {
		out = append(out, c.In(lang))
	}
	return out
}

func TestCleanUpFindsTheHeaderBelowTitleRowsAndListsTheSkip(t *testing.T) {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Kedai Runcit Ali"},
		[]any{"Laporan Kewangan Oktober 2026"},
		[]any{},
		[]any{"Tarikh", "Perkara", "Jumlah"},
		[]any{day, "Sewa", "RM1,200.00"},
		[]any{day, "Gaji", 300},
	)
	res := clean(t, file, finance(t))

	if res.HeaderRow != 4 {
		t.Fatalf("header row = %d, want 4", res.HeaderRow)
	}
	wantCols := []lab.Column{{"Tarikh", lab.Date}, {"Perkara", lab.Text}, {"Jumlah", lab.Number}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	wantRows := [][]any{{"2026-10-05", "Sewa", 1200.0}, {"2026-10-05", "Gaji", 300.0}}
	if !reflect.DeepEqual(res.Dataset.Rows, wantRows) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, wantRows)
	}
	if got, want := changesIn(res, "en"), []string{"Skipped rows 1 to 3 above the header in row 4."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
	if got, want := changesIn(res, "ms"), []string{"Baris 1 hingga 3 di atas tajuk lajur di baris 4 dilangkau."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
	wantGuess := lab.Choices{"date": "Tarikh", "amount": "Jumlah", "description": "Perkara"}
	if !reflect.DeepEqual(res.Guess, wantGuess) {
		t.Fatalf("guess = %v, want %v", res.Guess, wantGuess)
	}
	if len(res.Sheets) != 1 || res.Sheets[0].Rows != 2 || !reflect.DeepEqual(res.Used, []string{res.Sheets[0].Name}) {
		t.Fatalf("sheets = %+v, used %v", res.Sheets, res.Used)
	}
}

func TestCleanUpOfATidySheetChangesNothing(t *testing.T) {
	res := clean(t, finance(t).Sample, finance(t))
	if res.HeaderRow != 1 || len(res.Changes) != 0 {
		t.Fatalf("header row %d, changes %v; want 1 and none", res.HeaderRow, changesIn(res, "en"))
	}
	if len(res.Dataset.Rows) != len(readSample(t, finance(t)).Rows) {
		t.Fatalf("%d rows, want the Sample's", len(res.Dataset.Rows))
	}
}

func TestCleanUpOfAnUnreadableFileGivesOneProblem(t *testing.T) {
	_, err := lab.Clean(strings.NewReader("Tarikh,Jumlah\n05/11/2026,RM5\n"), finance(t), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) || !strings.Contains(p.In("en"), "CSV") {
		t.Fatalf("err = %v, want one Problem naming CSV", err)
	}
}

func row(cells ...any) []any { return cells }

func sheet(name string, rows ...[]any) testutil.Sheet { return testutil.Sheet{Name: name, Rows: rows} }

// sales is a sales sheet with n rows under the columns Perkara and Jumlah.
func sales(name string, n int) testutil.Sheet {
	s := sheet(name, row("Perkara", "Jumlah"))
	for i := range n {
		s.Rows = append(s.Rows, row(fmt.Sprintf("Item %d", i+1), 10*(i+1)))
	}
	return s
}

// sheetList is each sheet as "name rows" and, when it cannot join the picked sheet, the reason in English.
func sheetList(res *lab.CleanUp) []string {
	var out []string
	for _, s := range res.Sheets {
		line := fmt.Sprintf("%s %d", s.Name, s.Rows)
		if s.CanCombine {
			line += " can combine"
		}
		if s.Reason != "" {
			line += ": " + text.T("en", s.Reason)
		}
		out = append(out, line)
	}
	return out
}

func TestCleanUpListsEverySheetAndPicksAFlatListOverAGroupedNotesSheet(t *testing.T) {
	file := testutil.Workbook(t,
		sheet("Nota", row("Kumpulan", "Perkara"), row("Dapur", "Beras"), row(nil, "Gula"), row(nil, "Minyak")),
		sheet("Kosong"),
		sales("Jualan", 2),
	)
	res := clean(t, file, lab.General)
	want := []string{
		"Nota 3: Its columns differ from the picked sheet's.",
		"Kosong 0: Cave found no table with column names and rows below them.",
		"Jualan 2",
	}
	if got := sheetList(res); !reflect.DeepEqual(got, want) {
		t.Fatalf("sheets = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(res.Used, []string{"Jualan"}) {
		t.Fatalf("used = %v, want the flat list", res.Used)
	}
}

func cleanWith(t *testing.T, file []byte, s lab.Settings) *lab.CleanUp {
	t.Helper()
	res, err := lab.Clean(bytes.NewReader(file), lab.General, s)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	return res
}

func TestCleanUpCombinesSheetsWithTheSameColumnsAndAddsASheetColumn(t *testing.T) {
	file := testutil.Workbook(t,
		sheet("Kedai A", row("Laporan"), row("Perkara", "Jumlah"), row("Beras", 12)),
		sheet("Kedai B", row("  JUMLAH ", "perkara"), row(5, "Gula"), row(7, "Teh")), // other order, case and spaces; no title row
		sheet("Stok", row("Perkara", "Baki"), row("Beras", 3)),
	)
	res := cleanWith(t, file, lab.Settings{})
	want := []string{"Kedai A 1", "Kedai B 2 can combine", "Stok 1: Its columns differ from the picked sheet's."}
	if got := sheetList(res); !reflect.DeepEqual(got, want) {
		t.Fatalf("sheets = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(res.Used, []string{"Kedai A", "Kedai B"}) {
		t.Fatalf("used = %v", res.Used)
	}
	wantCols := []lab.Column{{"Sheet", lab.Text}, {"Perkara", lab.Text}, {"Jumlah", lab.Number}}
	wantRows := [][]any{{"Kedai A", "Beras", 12.0}, {"Kedai B", "Gula", 5.0}, {"Kedai B", "Teh", 7.0}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) || !reflect.DeepEqual(res.Dataset.Rows, wantRows) {
		t.Fatalf("dataset = %v %v, want %v %v", res.Dataset.Columns, res.Dataset.Rows, wantCols, wantRows)
	}
	if got := changesIn(res, "en"); !slices.Contains(got, "Combined 2 sheets into one Dataset, with each row's sheet name in a new Sheet column.") {
		t.Fatalf("changes = %q", got)
	}

	alone := cleanWith(t, file, lab.Settings{Sheet: "Kedai B", Alone: true})
	if !reflect.DeepEqual(alone.Used, []string{"Kedai B"}) || len(alone.Dataset.Columns) != 2 {
		t.Fatalf("alone: used %v, columns %v; want Kedai B and no Sheet column", alone.Used, alone.Dataset.Columns)
	}
}

func TestCleanUpTurnsDateSheetNamesIntoADateColumn(t *testing.T) {
	file := testutil.Workbook(t, sales("01.09.26", 1), sales("02.09.26", 2))
	res := cleanWith(t, file, lab.Settings{})
	if got := res.Dataset.Columns[0]; got != (lab.Column{"Date", lab.Date}) {
		t.Fatalf("first column = %v, want Date typed as a date", got)
	}
	var days []any
	for _, r := range res.Dataset.Rows {
		days = append(days, r[0])
	}
	if want := []any{"2026-09-01", "2026-09-02", "2026-09-02"}; !reflect.DeepEqual(days, want) {
		t.Fatalf("dates = %v, want %v", days, want)
	}
}

func TestANewSameColumnSheetIsTickedAndAMissingPickedSheetFallsBack(t *testing.T) {
	remembered := lab.Settings{Sheet: "Jan"}
	file := testutil.Workbook(t, sales("Jan", 1), sales("Feb", 1), sales("Mac", 1))
	if res := cleanWith(t, file, remembered); !reflect.DeepEqual(res.Used, []string{"Jan", "Feb", "Mac"}) {
		t.Fatalf("used = %v, want the new sheets ticked", res.Used)
	}
	file = testutil.Workbook(t, sheet("Nota", row("Kumpulan", "Perkara"), row("A", "B"), row(nil, "C")), sales("Apr", 1))
	if res := cleanWith(t, file, remembered); !reflect.DeepEqual(res.Used, []string{"Apr"}) {
		t.Fatalf("used = %v, want the first readable flat sheet", res.Used)
	}
	file = testutil.Workbook(t, sales("Jan", 1), sales("Feb", 1), sales("Mac", 1))
	if res := cleanWith(t, file, lab.Settings{Sheet: "Feb", Ticked: []string{"Mac"}}); !reflect.DeepEqual(res.Used, []string{"Feb", "Mac"}) {
		t.Fatalf("used = %v, want the picked sheet and the ticked one", res.Used)
	}
}

func TestAHeaderRowChangeAppliesToEveryTickedSheet(t *testing.T) {
	// Row 1 is wide enough to pass for a header; the Learner says row 2.
	file := testutil.Workbook(t,
		sheet("A", row("Kedai", "Ali"), row("Perkara", "Jumlah"), row("Beras", 1)),
		sheet("B", row("Kedai", "Abu"), row("Perkara", "Jumlah"), row("Gula", 2)),
	)
	res := cleanWith(t, file, lab.Settings{HeaderRow: 2})
	if !reflect.DeepEqual(res.Used, []string{"A", "B"}) || res.HeaderRow != 2 || len(res.Dataset.Rows) != 2 {
		t.Fatalf("used %v, header %d, rows %v", res.Used, res.HeaderRow, res.Dataset.Rows)
	}
}

func cleanProblem(t *testing.T, file []byte) string {
	t.Helper()
	_, err := lab.Clean(bytes.NewReader(file), lab.General, lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want a Problem", err)
	}
	return p.In("en")
}

func TestCleanUpRefusesMoreThan50000CombinedRowsAnd100Columns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 50,001-row workbook")
	}
	file := testutil.Workbook(t, sales("A", 25000), sales("B", 25001))
	if got, want := cleanProblem(t, file), "The 2 ticked sheets have 50,001 rows together. The Lab reads up to 50,000 rows. Tick fewer sheets and upload again."; got != want {
		t.Fatalf("problem = %q, want %q", got, want)
	}
	if res := cleanWith(t, file, lab.Settings{Sheet: "B", Alone: true}); len(res.Dataset.Rows) != 25001 {
		t.Fatalf("%d rows, want B's 25,001", len(res.Dataset.Rows))
	}

	wide := make([]any, 101)
	for i := range wide {
		wide[i] = fmt.Sprintf("C%d", i+1)
	}
	if got, want := cleanProblem(t, testutil.Workbook(t, sheet("Lebar", wide, row(1)))), "The sheet has 101 columns. The Lab reads up to 100 columns."; got != want {
		t.Fatalf("problem = %q, want %q", got, want)
	}
}
