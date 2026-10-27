package lab_test

import (
	"bytes"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// roster is a days-across grid sheet: a title, a header of Staff and days 1 to 31, then a blank column and side
// counts, each staff's codes on given days, and a legend below a blank row.
func roster(name, title string, codes map[string]map[int]string) testutil.Sheet {
	header := []any{"Staff"}
	for d := 1; d <= 31; d++ {
		header = append(header, d)
	}
	header = append(header, nil, "Jumlah P", "Jumlah M")
	rows := [][]any{{title}, header}
	staff := make([]string, 0, len(codes))
	for s := range codes {
		staff = append(staff, s)
	}
	slices.Sort(staff)
	for _, s := range staff {
		r := make([]any, 35)
		r[0] = s
		for d, c := range codes[s] {
			r[d] = c
		}
		r[33], r[34] = 2, 1
		rows = append(rows, r)
	}
	rows = append(rows, nil, []any{"P = Pagi, M = Malam"}, []any{"Disediakan oleh: Penyelia"})
	return testutil.Sheet{Name: name, Rows: rows}
}

func TestCleanUpUnpivotsADaysAcrossGridToStaffDateCodeRows(t *testing.T) {
	file := testutil.Workbook(t, roster("Jadual", "Jadual Tugas Oktober 2026", map[string]map[int]string{
		"Ali":  {1: "P", 2: "M", 31: "P"},
		"Siti": {5: "M"},
	}))
	res := clean(t, file)

	wantCols := []lab.Column{{"Staff", lab.Text}, {"Date", lab.Date}, {"Code", lab.Text}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	wantRows := [][]any{
		{"Ali", "2026-10-01", "P"}, {"Ali", "2026-10-02", "M"}, {"Ali", "2026-10-31", "P"},
		{"Siti", "2026-10-05", "M"},
	}
	if !reflect.DeepEqual(res.Dataset.Rows, wantRows) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, wantRows)
	}
	if res.Month != 10 || res.Year != 2026 {
		t.Fatalf("month %d/%d, want 10/2026", res.Month, res.Year)
	}
	en, ms := changesIn(res, "en"), changesIn(res, "ms")
	if !slices.Contains(en, "Turned the days across the columns into 4 rows, one per staff per day, dated 10/2026. Empty days, counts beside the grid and notes below it were left out.") {
		t.Fatalf("changes = %q", en)
	}
	if !slices.Contains(ms, "Hari merentas lajur ditukar menjadi 4 baris, satu bagi setiap staf setiap hari, bertarikh 10/2026. Hari kosong, kiraan di sebelah grid dan nota di bawahnya tidak diambil.") {
		t.Fatalf("changes = %q", ms)
	}
}

func TestCleanUpReadsTheGridMonthFromTheSheetNameOrTheLearnerElseThisMonth(t *testing.T) {
	codes := map[string]map[int]string{"Ali": {3: "P"}}
	at := func(res *lab.CleanUp) any { return res.Dataset.Rows[0][1] }

	if got := at(clean(t, testutil.Workbook(t, roster("Mac 2025", "Jadual Tugas", codes)))); got != "2025-03-03" {
		t.Fatalf("Malay month in the sheet name: %v", got)
	}
	if got := at(clean(t, testutil.Workbook(t, roster("Roster", "Duty roster, February", codes)))); got != time.Date(time.Now().Year(), 2, 3, 0, 0, 0, 0, time.UTC).Format("2006-01-02") {
		t.Fatalf("English month in the title, this year: %v", got)
	}
	if got := at(clean(t, testutil.Workbook(t, roster("Roster", "Jadual Tugas", codes)))); got != time.Date(time.Now().Year(), time.Now().Month(), 3, 0, 0, 0, 0, time.UTC).Format("2006-01-02") {
		t.Fatalf("no month named, this month: %v", got)
	}
	res := cleanWith(t, testutil.Workbook(t, roster("Oktober", "Jadual Tugas Oktober 2026", codes)), lab.Settings{Month: 1, Year: 2027})
	if got := at(res); got != "2027-01-03" {
		t.Fatalf("the Learner's month: %v", got)
	}
}

func TestDepartmentGridsAreEachUnpivotedThenCombined(t *testing.T) {
	file := testutil.Workbook(t,
		roster("Dapur", "Jadual Tugas November 2026", map[string]map[int]string{"Ali": {1: "P"}}),
		roster("Kaunter", "Jadual Tugas November 2026", map[string]map[int]string{"Siti": {2: "M"}, "Abu": {30: "P"}}),
	)
	res := clean(t, file)

	wantCols := []lab.Column{{"Sheet", lab.Text}, {"Staff", lab.Text}, {"Date", lab.Date}, {"Code", lab.Text}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) {
		t.Fatalf("columns = %v, want %v", res.Dataset.Columns, wantCols)
	}
	wantRows := [][]any{
		{"Dapur", "Ali", "2026-11-01", "P"},
		{"Kaunter", "Abu", "2026-11-30", "P"}, {"Kaunter", "Siti", "2026-11-02", "M"},
	}
	if !reflect.DeepEqual(res.Dataset.Rows, wantRows) {
		t.Fatalf("rows = %v, want %v", res.Dataset.Rows, wantRows)
	}
}

// grid is a days-across sheet: a title, a header of lead names then days 1 to 31, and rows of lead cells then
// each row's cells by day.
func grid(name string, lead []any, rows ...[]any) testutil.Sheet {
	header := slices.Clone(lead)
	for d := 1; d <= 31; d++ {
		header = append(header, d)
	}
	return testutil.Sheet{Name: name, Rows: append([][]any{{"Jadual Tugas Oktober 2026"}, header}, rows...)}
}

// days is one grid row: its lead cells, then a value on given days.
func days(lead []any, by map[int]any) []any {
	r := append(slices.Clone(lead), make([]any, 31)...)
	for d, v := range by {
		r[len(lead)+d-1] = v
	}
	return r
}

// A grid's staff column is the names one, always called Staff, beside only Date and Code; count and total rows
// right under the staff rows are not shifts.
func TestAGridGivesStaffDateCodeAndLeavesOutCountRowsUnderIt(t *testing.T) {
	file := testutil.Workbook(t, grid("Jadual", []any{"Bil", "Nama Pekerja"},
		days([]any{1, "Ali"}, map[int]any{1: "P"}),
		days([]any{2, "Siti"}, map[int]any{2: "M"}),
		days([]any{nil, "Bilangan Pagi"}, map[int]any{1: 1, 2: 0}),
		days([]any{nil, "Jumlah"}, map[int]any{1: 1, 2: 1}),
	))
	res := clean(t, file)

	wantCols := []lab.Column{{"Staff", lab.Text}, {"Date", lab.Date}, {"Code", lab.Text}}
	wantRows := [][]any{{"Ali", "2026-10-01", "P"}, {"Siti", "2026-10-02", "M"}}
	if !reflect.DeepEqual(res.Dataset.Columns, wantCols) || !reflect.DeepEqual(res.Dataset.Rows, wantRows) {
		t.Fatalf("dataset = %v %v, want %v %v", res.Dataset.Columns, res.Dataset.Rows, wantCols, wantRows)
	}
}

// A grid far down a sheet, under a title and 200,000 blank rows, still takes its month from the title and cleans
// in well under a second; reading the title rows once took seconds, growing with the square of the rows.
func TestAGridUnderManyBlankRowsCleansQuicklyAndKeepsItsTitleMonth(t *testing.T) {
	const blank = 200000
	f := excelize.NewFile()
	header, codes := []any{"Staff"}, []any{"Ali"}
	for d := 1; d <= 31; d++ {
		header, codes = append(header, d), append(codes, "M")
	}
	f.SetCellValue("Sheet1", "A1", "Jadual Tugas Oktober 2026")
	f.SetSheetRow("Sheet1", "A"+strconv.Itoa(blank+2), &header)
	f.SetSheetRow("Sheet1", "A"+strconv.Itoa(blank+3), &codes)
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res := clean(t, b.Bytes())
	if took := time.Since(start); took > time.Second {
		t.Fatalf("Clean took %v, want under 1s", took)
	}
	if res.Month != 10 || res.Year != 2026 || len(res.Dataset.Rows) != 31 {
		t.Fatalf("month %d/%d and %d rows, want 10/2026 and 31", res.Month, res.Year, len(res.Dataset.Rows))
	}
}
