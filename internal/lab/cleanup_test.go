package lab_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
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

func TestCleanUpReadsADashAsZeroInANumberColumn(t *testing.T) {
	file := testutil.XLSX(t, []string{"Barang", "Baki"},
		[]any{"Gula", 12},
		[]any{"Tepung", "-"},
		[]any{"Beras", " - "},
	)
	res := clean(t, file, template(t, "inventory"))

	if got := res.Dataset.Columns[1].Type; got != lab.Number {
		t.Fatalf("Baki is %s, want number", got)
	}
	want := [][]any{{"Gula", 12.0}, {"Tepung", 0.0}, {"Beras", 0.0}}
	if !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if got, want := changesIn(res, "en"), []string{"Cells holding - read as 0: 2."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestCleanUpReadsDateTimesISOAndShortDatesAsDates(t *testing.T) {
	file := testutil.XLSX(t, []string{"Created Date & Time", "Total Amount"},
		[]any{"05/11/2026 2:30 PM", 10},
		[]any{"5/11/2026 11:05:09 am", 10},
		[]any{"2026-11-06", 10},
		[]any{"2026-11-07T14:05:00", 10},
		[]any{"07.11.26", 10},
		[]any{"08/11/2026", 10},
	)
	res := clean(t, file, finance(t))

	if got := res.Dataset.Columns[0].Type; got != lab.Date {
		t.Fatalf("dates read as %s", got)
	}
	var got []any
	for _, row := range res.Dataset.Rows {
		got = append(got, row[0])
	}
	want := []any{"2026-11-05", "2026-11-05", "2026-11-06", "2026-11-07", "2026-11-07", "2026-11-08"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dates = %v, want %v", got, want)
	}
	if got, want := changesIn(res, "en"), []string{"Dates written another way (with a time, as 2026-11-05 or as 05.11.26) read as dates, times left out: 5."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestCleanUpDropsTotalAndLabelRowsAtTheBottomOnly(t *testing.T) {
	file := testutil.XLSX(t, []string{"Perkara", "Jumlah"},
		[]any{"Sewa", 1000},
		[]any{"Total", 1000},
		[]any{"Gaji", 300},
		[]any{"JUMLAH", 1300},
		[]any{"Disediakan oleh: Ali"},
	)
	res := clean(t, file, finance(t))

	want := [][]any{{"Sewa", 1000.0}, {"Total", 1000.0}, {"Gaji", 300.0}}
	if !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if got, want := changesIn(res, "en"), []string{"Dropped rows 5 to 6 at the bottom: totals or notes, not data."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestCleanUpStopsAtTheFirstBlankRowAndColumn(t *testing.T) {
	file := testutil.XLSX(t, []string{"Perkara", "Jumlah", "", "Kod", "Maksud"},
		[]any{"Sewa", 1000, nil, "S", "Sewa kedai"},
		[]any{"Gaji", 300, nil, "G", "Gaji pekerja"},
		[]any{"Air", 50, nil, "U", "Utiliti"},
		[]any{"Elektrik", 120},
		[]any{},
		[]any{"Nota: harga termasuk SST"},
		[]any{"Kod", "Kiraan"},
		[]any{"S", 1},
	)
	res := clean(t, file, finance(t))

	wantCols := []lab.Column{{"Perkara", lab.Text}, {"Jumlah", lab.Number}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	if len(res.Dataset.Rows) != 4 {
		t.Fatalf("rows = %v, want the 4 above the blank row", res.Dataset.Rows)
	}
	want := []string{
		"Stopped at the blank column 3: what is to its right (notes or another table) was not read.",
		"Stopped at the blank row 6: what is below it (notes or another table) was not read.",
	}
	if got := changesIn(res, "en"); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestCleanUpDropsColumnsWithNoData(t *testing.T) {
	file := testutil.XLSX(t, []string{"Perkara", "Catatan", "Jumlah", "Rujukan"},
		[]any{"Sewa", nil, 1000},
		[]any{"Gaji", nil, 300},
	)
	res := clean(t, file, finance(t))

	wantCols := []lab.Column{{"Perkara", lab.Text}, {"Jumlah", lab.Number}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	if want := [][]any{{"Sewa", 1000.0}, {"Gaji", 300.0}}; !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if got, want := changesIn(res, "en"), []string{"Columns with no data dropped: 2."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestCleanUpFillsMergedDataCellsDown(t *testing.T) {
	file := testutil.XLSX(t, []string{"Kategori", "Perkara", "Jumlah"},
		[]any{"Minuman", "Kopi", 5},
		[]any{nil, "Teh", 4},
		[]any{nil, "Susu", 6},
		[]any{"Makanan", "Roti", 3},
		[]any{nil, "Kek", 9},
	)
	res := clean(t, testutil.Merge(t, file, "A2:A4", "A5:A6"), finance(t))

	want := [][]any{
		{"Minuman", "Kopi", 5.0}, {"Minuman", "Teh", 4.0}, {"Minuman", "Susu", 6.0},
		{"Makanan", "Roti", 3.0}, {"Makanan", "Kek", 9.0},
	}
	if !reflect.DeepEqual(res.Dataset.Rows, want) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, want)
	}
	if got, want := changesIn(res, "en"), []string{"Merged cells filled down: 3."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestTheLearnerCanNameTheHeaderRow(t *testing.T) {
	file := testutil.XLSX(t, []string{"Laporan", "Oktober 2026"},
		[]any{},
		[]any{"Perkara", "Kategori", "Jumlah"},
		[]any{"Sewa", "Kedai", 1000},
	)
	res, err := lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{HeaderRow: 3})
	if err != nil {
		t.Fatal(err)
	}
	wantCols := []lab.Column{{"Perkara", lab.Text}, {"Kategori", lab.Text}, {"Jumlah", lab.Number}}
	if res.HeaderRow != 3 || !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("header row %d, columns %v; want 3 and %v", res.HeaderRow, res.Dataset.Columns, wantCols)
	}
	want := []string{"Read row 3 as the header, as you chose.", "Skipped rows 1 to 2 above the header in row 3."}
	if got := changesIn(res, "en"); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestATwoRowHeaderJoinsItsRowsIntoOneNamePerColumn(t *testing.T) {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Jualan Oktober 2026"},
		[]any{"Tarikh", "Jualan", nil, "Staf"},
		[]any{nil, "Tunai", "Kad"},
		[]any{day, 120, 80, "Ali"},
		[]any{day, 60, 0, "Siti"},
	)
	file = testutil.Merge(t, file, "A2:A3", "B2:C2", "D2:D3")
	res, err := lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{TwoRowHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	wantCols := []lab.Column{{"Tarikh", lab.Date}, {"Jualan Tunai", lab.Number}, {"Jualan Kad", lab.Number}, {"Staf", lab.Text}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	if want := []any{"2026-10-05", 60.0, 0.0, "Siti"}; len(res.Dataset.Rows) != 2 || !reflect.DeepEqual(res.Dataset.Rows[1], want) {
		t.Fatalf("rows = %v, want 2 ending %v", res.Dataset.Rows, want)
	}
	want := []string{"Skipped row 1 above the header in row 2.", "Joined header rows 2 and 3 into one name per column."}
	if got := changesIn(res, "en"); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

func TestWorkGuessingMatchesNamesLooselyAndNeverByTypeAlone(t *testing.T) {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"No. Resit", "Created Date & Time", "Total Amount (RM)", "Bilangan"},
		[]any{"R001", day, 12.5, 2},
	)
	if got, want := clean(t, file, finance(t)).Guess, (lab.Choices{"date": "Created Date & Time", "amount": "Total Amount (RM)"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("guess = %v, want %v", got, want)
	}

	file = testutil.XLSX(t, []string{"Bila", "Siapa", "Berapa"}, []any{day, "Ali", 12.5})
	if got := clean(t, file, finance(t)).Guess; len(got) != 0 {
		t.Fatalf("guess = %v, want nothing: no name matches", got)
	}
}

func TestInventoryGuessesTheClosingBalanceForQuantity(t *testing.T) {
	file := testutil.XLSX(t, []string{"Nama Barang", "Purchase Quantity", "Opening Balance", "Stock Movement", "Closing Balance"},
		[]any{"Gula", 10, 5, -3, 12},
	)
	got := clean(t, file, template(t, "inventory")).Guess
	if want := (lab.Choices{"item": "Nama Barang", "quantity": "Closing Balance"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("guess = %v, want %v", got, want)
	}
}

func TestCleanUpOfAnUnreadableFileGivesOneProblem(t *testing.T) {
	_, err := lab.Clean(strings.NewReader("Tarikh,Jumlah\n05/11/2026,RM5\n"), finance(t), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) || !strings.Contains(p.In("en"), "CSV") {
		t.Fatalf("err = %v, want one Problem naming CSV", err)
	}
}
