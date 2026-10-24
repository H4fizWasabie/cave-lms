package lab_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

func finance(t *testing.T) *lab.Template {
	t.Helper()
	tpl := lab.TemplateByKey("finance")
	if tpl == nil {
		t.Fatal("no Finance Template")
	}
	return tpl
}

func readSample(t *testing.T, tpl *lab.Template) *lab.Dataset {
	t.Helper()
	d, err := lab.Read(bytes.NewReader(tpl.Sample), tpl)
	if err != nil {
		t.Fatalf("read %s Sample: %v", tpl.Name, err)
	}
	return d
}

func TestReadsTheFinanceSample(t *testing.T) {
	d := readSample(t, finance(t))
	want := []lab.Column{{"Tarikh", lab.Date}, {"Perkara", lab.Text}, {"Kategori", lab.Text}, {"Jenis", lab.Text}, {"Jumlah", lab.Number}}
	if !reflect.DeepEqual(d.Columns, want) {
		t.Fatalf("columns = %v, want %v", d.Columns, want)
	}
	if len(d.Rows) != 65 {
		t.Fatalf("got %d rows, want 65", len(d.Rows))
	}
	first := []any{"2026-01-01", "Sewa pejabat", "Sewa", "Keluar", 3200.0}
	last := []any{"2026-09-26", "Jualan modul", "Jualan", "Masuk", 493.7}
	if !reflect.DeepEqual(d.Rows[0], first) || !reflect.DeepEqual(d.Rows[64], last) {
		t.Fatalf("first row %v, last row %v; want %v and %v", d.Rows[0], d.Rows[64], first, last)
	}
}

func TestGuessFillsEveryFinanceBlankFromTheSample(t *testing.T) {
	fin := finance(t)
	got := fin.Guess(readSample(t, fin))
	want := lab.Choices{"date": "Tarikh", "amount": "Jumlah", "category": "Kategori", "type": "Jenis", "description": "Perkara"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guesses = %v, want %v", got, want)
	}
}

func TestGuessFallsBackToTypeForRequiredBlanksOnly(t *testing.T) {
	fin := finance(t)
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Bila", "Nota", "Berapa"}, []any{day, "Kopi", 12.5})
	d, err := lab.Read(bytes.NewReader(file), fin)
	if err != nil {
		t.Fatal(err)
	}
	if want := []any{"2026-03-05", "Kopi", 12.5}; !reflect.DeepEqual(d.Rows[0], want) {
		t.Fatalf("row = %v, want %v", d.Rows[0], want)
	}
	got := fin.Guess(d)
	if want := (lab.Choices{"date": "Bila", "amount": "Berapa"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("guesses = %v, want %v", got, want)
	}
}

func TestGenerateWritesThePageAndTheDatasetAsDataJSON(t *testing.T) {
	fin := finance(t)
	d := readSample(t, fin)
	index, data, err := lab.Generate(fin, "Kewangan Kedai", fin.Guess(d), d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	page := string(index)
	for _, want := range []string{"<title>Kewangan Kedai</title>", `<script src="charts.js"></script>`, `fetch("data.json")`,
		`"name": "Jumlah"`, "Money in", "Balance", "Spending by category", "Latest transactions"} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	if strings.Contains(page, `"Total"`) {
		t.Errorf("index.html has the Total card although Type is chosen")
	}

	if !bytes.HasPrefix(data, []byte(`[`+"\n"+`  {"Tarikh": "2026-01-01", "Perkara": "Sewa pejabat", "Kategori": "Sewa", "Jenis": "Keluar", "Jumlah": 3200}`)) {
		t.Fatalf("data.json does not start with the first row in column order:\n%.300s", data)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	totals := map[any]float64{}
	for _, r := range rows {
		totals[r["Jenis"]] += r["Jumlah"].(float64)
	}
	// The Sample's totals, summed from its sheet XML outside Go.
	if len(rows) != 65 || math.Round(totals["Masuk"]*100) != 8836647 || math.Round(totals["Keluar"]*100) != 3742391 {
		t.Fatalf("data.json has %d rows, totals %v; want 65 rows, Masuk 88366.47, Keluar 37423.91", len(rows), totals)
	}
}

func TestSkippingAnOptionalBlankLeavesOutTheBlocksThatNeedIt(t *testing.T) {
	fin := finance(t)
	d := readSample(t, fin)
	c := fin.Guess(d)
	delete(c, "type")
	index, _, err := lab.Generate(fin, "Kewangan", c, d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	page := string(index)
	for _, gone := range []string{"Money in", "Money out", "Balance", "Money in by source"} {
		if strings.Contains(page, `"`+gone+`"`) {
			t.Errorf("index.html still has %q without a Type column", gone)
		}
	}
	for _, kept := range []string{`"Total"`, `"Transactions"`, `"By month"`, `"Spending by category"`, `"Latest transactions"`} {
		if !strings.Contains(page, kept) {
			t.Errorf("index.html lacks %s", kept)
		}
	}
}

func TestGenerateRefusesChoicesThatDoNotFit(t *testing.T) {
	fin := finance(t)
	d := readSample(t, fin)
	for name, c := range map[string]lab.Choices{
		"required blank skipped": {"date": "Tarikh"},
		"no such column":         {"date": "Tarikh", "amount": "Harga"},
		"wrong type":             {"date": "Tarikh", "amount": "Kategori"},
	} {
		if _, _, err := lab.Generate(fin, "Kewangan", c, d, "data.json"); err == nil {
			t.Errorf("%s: Generate accepted %v", name, c)
		}
	}
}

func TestGenerateRefusesARequiredColumnWithBlankCells(t *testing.T) {
	fin := finance(t)
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	d, err := lab.Read(bytes.NewReader(testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Lain"},
		[]any{day, 1, 2}, []any{nil, nil, nil}, []any{day, 3, nil})), fin)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = lab.Generate(fin, "Kewangan", lab.Choices{"date": "Tarikh", "amount": "Lain"}, d, "data.json")
	var got lab.Problem
	if !errors.As(err, &got) || got.In("en") != "Row 4, Lain: empty, but this column is required." {
		t.Fatalf("err = %v, want the blank cell in Lain named by its Excel row", err)
	}
	if _, _, err := lab.Generate(fin, "Kewangan", lab.Choices{"date": "Tarikh", "amount": "Jumlah"}, d, "data.json"); err != nil {
		t.Fatalf("a blank in a column no required blank uses refused: %v", err)
	}
}

func TestGeneratedPageEscapesColumnNamesAndCellText(t *testing.T) {
	fin := finance(t)
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "</script><b>Kategori</b>", "Jumlah"}, []any{day, "<script>alert(1)</script>", 12.5})
	d, err := lab.Read(bytes.NewReader(file), fin)
	if err != nil {
		t.Fatal(err)
	}
	index, data, err := lab.Generate(fin, "<i>Mine</i>", lab.Choices{"date": "Tarikh", "amount": "Jumlah", "category": "</script><b>Kategori</b>"}, d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range [][]byte{index, data} {
		if bytes.Contains(file, []byte("<b>")) || bytes.Contains(file, []byte("<script>alert")) || bytes.Contains(file, []byte("<i>")) || bytes.Contains(file, []byte("</script><b>")) {
			t.Fatalf("hostile text left unescaped:\n%s", file)
		}
	}
}

// readProblem reads a file it expects Cave to refuse and returns the one Learner-worded message, in English.
func readProblem(t *testing.T, file []byte) string {
	t.Helper()
	_, err := lab.Read(bytes.NewReader(file), finance(t))
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("Read gave %v, want one lab.Problem", err)
	}
	return p.In("en")
}

func TestReadsEveryDateForm(t *testing.T) {
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Bila"},
		[]any{day, 1.0, "05/11/2026"},        // a real Excel date; day/month/year text in a column Cave has no name for
		[]any{"05/11/2026", 2.0, "5/1/2026"}, // day/month/year text: 5 November, not May 11
		[]any{45231, 3.0, "31/12/2026"},      // an Excel date serial number
	)
	d, err := lab.Read(bytes.NewReader(file), finance(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.Columns[2].Type != lab.Date {
		t.Fatalf("Bila read as %s, want date", d.Columns[2].Type)
	}
	want := [][]any{{"2026-11-05", 1.0, "2026-11-05"}, {"2026-11-05", 2.0, "2026-01-05"}, {"2023-11-01", 3.0, "2026-12-31"}}
	if !reflect.DeepEqual(d.Rows, want) {
		t.Fatalf("rows = %v, want %v", d.Rows, want)
	}
}

func TestReadsRinggitAmountsAsNumbers(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Harga"},
		[]any{day, "RM1,200.00", "RM 5"},
		[]any{day, "1,200", "-RM12.50"},
		[]any{day, "RM 50", 7},
	)
	d, err := lab.Read(bytes.NewReader(file), finance(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.Columns[2].Type != lab.Number {
		t.Fatalf("Harga read as %s, want number", d.Columns[2].Type)
	}
	want := [][]any{{"2026-03-05", 1200.0, 5.0}, {"2026-03-05", 1200.0, -12.5}, {"2026-03-05", 50.0, 7.0}}
	if !reflect.DeepEqual(d.Rows, want) {
		t.Fatalf("rows = %v, want %v", d.Rows, want)
	}
}

func TestAnUnreadableFileGivesOneMessageNamingTheFirstBadRow(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Kategori", "Jumlah"},
		[]any{day, nil, 10},             // a blank optional cell is fine
		[]any{day, "Sewa", "dua ratus"}, // row 3, the first bad row
		[]any{"31/02/2026", "Sewa", 5},  // row 4
		[]any{day, "Sewa", nil},         // row 5
		[]any{nil, "Sewa", 5, "lebih"},  // row 6
	)
	_, err := lab.Read(bytes.NewReader(file), finance(t))
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("Read gave %v, want one lab.Problem", err)
	}
	if got, want := p.In("en"), `Row 3, Jumlah: not a number ("dua ratus"). Type it as a plain number, like 200 or RM1,200.00.`; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if got, want := p.In("ms"), `Baris 3, Jumlah: bukan nombor ("dua ratus"). Taip sebagai nombor biasa, seperti 200 atau RM1,200.00.`; got != want {
		t.Fatalf("Malay message = %q, want %q", got, want)
	}
}

func TestEachUnreadableCellGivesItsOwnCauseAndRow(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		row  []any
		want string
	}{
		{[]any{"31/02/2026", "Sewa", 5}, `Row 3, Tarikh: not a date ("31/02/2026"). Type it as day/month/year, like 05/11/2026.`},
		{[]any{day, "Sewa", nil}, `Row 3, Jumlah: empty, but this column is required.`},
		{[]any{day, "Sewa", 5, "lebih"}, `Row 3 has "lebih" in column D, which has no name in row 1. Name the column or clear the cell.`},
	} {
		file := testutil.XLSX(t, []string{"Tarikh", "Kategori", "Jumlah"}, []any{day, "Sewa", 1}, c.row)
		if got := readProblem(t, file); got != c.want {
			t.Errorf("message = %q, want %q", got, c.want)
		}
	}
}

func TestAMissingRequiredColumnIsNamed(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	got := readProblem(t, testutil.XLSX(t, []string{"Tarikh", "Kategori"}, []any{day, "Sewa"}))
	want := "Column Jumlah is missing. The Finance Template needs it for Amount, a column of numbers. Did you rename it? Rename it back in row 1 and upload again."
	if got != want {
		t.Fatalf("errors = %v, want %v", got, want)
	}
}

func TestAHeaderBelowRow1IsNamedByItsRow(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.Workbook(t, testutil.Sheet{Name: "Sheet1", Rows: [][]any{
		{}, {"Akaun Mac 2026"}, {}, {"Date", "Description", "Amount"}, {day, "Sewa", 5},
	}})
	_, err := lab.Read(bytes.NewReader(file), finance(t))
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("Read gave %v, want one lab.Problem", err)
	}
	if got, want := p.In("en"), "Your column names seem to be on row 4, not row 1. Delete the rows above it, or use a Work Dashboard, and upload again."; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if got, want := p.In("ms"), "Nama lajur anda nampaknya di baris 4, bukan baris 1. Padam baris di atasnya, atau guna Dashboard Kerja, dan muat naik semula."; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}

	// A title in A1 reads as a text column, so HR's text blanks are not missing; the header's cells are stray.
	file = testutil.Workbook(t, testutil.Sheet{Name: "Sheet1", Rows: [][]any{{"Senarai Staf"}, {}, {"Nama", "Jabatan"}, {"Ali", "Dapur"}}})
	if _, err := lab.Read(bytes.NewReader(file), lab.TemplateByKey("hr")); err == nil || !strings.HasPrefix(err.Error(), "Your column names seem to be on row 3,") {
		t.Fatalf("HR: Read gave %v, want the header named on row 3", err)
	}

	// One name in a data row, like a Total row, is not a header.
	got := readProblem(t, testutil.XLSX(t, []string{"Tarikh", "Kategori"}, []any{day, "Sewa"}, []any{nil, "Total"}))
	if !strings.HasPrefix(got, "Column Jumlah is missing.") {
		t.Fatalf("message = %q, want the missing column", got)
	}
}

func TestRefusesMoreThan5000Rows(t *testing.T) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	rows := make([][]any, 5001)
	for i := range rows {
		rows[i] = []any{day, 1}
	}
	got := readProblem(t, testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, rows...))
	if want := "The file has 5,001 rows. The Lab reads up to 5,000 rows. Row 5,002 is past the limit. Delete some rows and upload again."; got != want {
		t.Fatalf("errors = %v, want %v", got, want)
	}
	if _, err := lab.Read(bytes.NewReader(testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, rows[:5000]...)), finance(t)); err != nil {
		t.Fatalf("5,000 rows refused: %v", err)
	}
}

// zipBomb is a small valid workbook carrying 60 MB of zeros, which zip down to almost nothing.
func zipBomb(t *testing.T) []byte {
	t.Helper()
	small := testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, []any{time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), 1})
	in, err := zip.NewReader(bytes.NewReader(small), int64(len(small)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	out := zip.NewWriter(&buf)
	for _, f := range in.File {
		if err := out.Copy(f); err != nil {
			t.Fatal(err)
		}
	}
	pad, err := out.Create("xl/media/pad.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pad.Write(make([]byte, 60<<20)); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAWorkbookThatUnzipsPast50MBIsRefusedUnread(t *testing.T) {
	file := zipBomb(t)
	if len(file) > 1<<20 {
		t.Fatalf("the zip bomb is %d bytes; it should fit well under the 10 MB upload limit", len(file))
	}
	got := readProblem(t, file)
	if want := ("This workbook is over 50 MB once unpacked. The Lab reads workbooks up to 50 MB unpacked. Delete sheets, pictures or rows you do not need, and upload again."); got != want {
		t.Fatalf("errors = %v, want %v", got, want)
	}
}

func TestACSVIsRefusedWithWhyXLSX(t *testing.T) {
	got := readProblem(t, []byte("Tarikh,Jumlah\n05/11/2026,RM5\n"))
	if !strings.Contains(got, "A CSV file stores everything as plain text") {
		t.Fatalf("errors = %v", got)
	}
}

// farOutWorkbook has a header and rows rows, each with one cell at column XFD, the last there is. excelize pads
// each such row out to 16,384 cells, so a small file holds millions of them.
func farOutWorkbook(t *testing.T, rows int) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "Tarikh")
	for n := 2; n <= rows+1; n++ {
		f.SetCellValue("Sheet1", "XFD"+strconv.Itoa(n), 1)
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRefusesASheetOfTooManyCells(t *testing.T) {
	file := farOutWorkbook(t, 400)
	_, readErr := lab.Read(bytes.NewReader(file), finance(t))
	_, cleanErr := lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{})
	for name, err := range map[string]error{"Read": readErr, "Clean": cleanErr} {
		var p lab.Problem
		if !errors.As(err, &p) || p.Key != "lab.err.too-many-cells" {
			t.Errorf("%s: got %v, want lab.err.too-many-cells", name, err)
		}
	}
}
