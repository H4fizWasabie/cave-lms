package lab_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// Edge cases of what a Learner's workbook holds. Where the right behaviour is Abah's to decide, the test says
// "pinned" and holds what Clean-up does today.

const sheet1 = "xl/worksheets/sheet1.xml"

// inlineCell is a text cell written the way handMade workbooks write it.
func inlineCell(ref, s string) string {
	return `<c r="` + ref + `" t="inlineStr"><is><t>` + s + `</t></is></c>`
}

// namesAndAmounts is a Nama and Jumlah header and then rows of an inline name and the XML of a Jumlah cell.
func namesAndAmounts(amounts ...string) string {
	rows := `<row r="1">` + inlineCell("A1", "Nama") + inlineCell("B1", "Jumlah") + `</row>`
	for i, c := range amounts {
		rows += fmt.Sprintf(`<row r="%d">%s%s</row>`, i+2, inlineCell(fmt.Sprintf("A%d", i+2), string(rune('a'+i))), c)
	}
	return rows
}

func problemKey(t *testing.T, file []byte) string {
	t.Helper()
	_, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("err = %v, want one Problem", err)
	}
	return p.Key
}

func wantRows(t *testing.T, res *lab.CleanUp, cols []lab.Column, rows [][]any) {
	t.Helper()
	if !reflect.DeepEqual(res.Dataset.Columns, cols) {
		t.Errorf("columns = %v, want %v", res.Dataset.Columns, cols)
	}
	if !reflect.DeepEqual(res.Dataset.Rows, rows) {
		t.Errorf("rows = %v, want %v", res.Dataset.Rows, rows)
	}
}

func TestTwoColumnsWithTheSameHeaderNameAreRefused(t *testing.T) {
	file := testutil.XLSX(t, []string{"Jumlah", "Jumlah"}, []any{1, 2}, []any{3, 4})
	if got := problemKey(t, file); !strings.HasPrefix(got, "lab.err.") {
		t.Fatalf("problem = %q", got)
	}
	_, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	if !strings.Contains(err.Error(), "Two columns are called Jumlah in the header, row 1") {
		t.Fatalf("err = %v, want it to name the column and the row", err)
	}
}

func TestAColumnWithDataButNoHeaderNameIsRefusedNamingIt(t *testing.T) {
	file := testutil.XLSX(t, []string{"Nama", "", "Jumlah"}, []any{"a", "x", 2}, []any{"b", "y", 4})
	_, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	if err == nil || !strings.Contains(err.Error(), "Column B has data but no name in the header, row 1") {
		t.Fatalf("err = %v, want it to name column B", err)
	}
}

func TestASheetWithOnlyAHeaderRowIsRefused(t *testing.T) {
	file := testutil.XLSX(t, []string{"Nama", "Jumlah"})
	_, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	if err == nil || !strings.Contains(err.Error(), "found no data below it") {
		t.Fatalf("err = %v, want no data below the header", err)
	}
}

// Pinned: hidden rows and columns are read like any other, and a hidden sheet joins the sheets with its columns.
func TestHiddenRowsColumnsAndSheetsAreReadLikeVisibleOnes(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetRow("Sheet1", "A1", &[]any{"Nama", "Jumlah", "Kod"})
	f.SetSheetRow("Sheet1", "A2", &[]any{"a", 1, "x"})
	f.SetSheetRow("Sheet1", "A3", &[]any{"b", 2, "y"})
	f.SetRowVisible("Sheet1", 3, false)
	f.SetColVisible("Sheet1", "C", false)
	f.NewSheet("Rahsia")
	f.SetSheetRow("Rahsia", "A1", &[]any{"Nama", "Jumlah", "Kod"})
	f.SetSheetRow("Rahsia", "A2", &[]any{"h", 9, "q"})
	f.SetSheetVisible("Rahsia", false)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	res := clean(t, buf.Bytes())
	wantRows(t, res,
		[]lab.Column{{"Sheet", lab.Text}, {"Nama", lab.Text}, {"Jumlah", lab.Number}, {"Kod", lab.Text}},
		[][]any{{"Sheet1", "a", 1.0, "x"}, {"Sheet1", "b", 2.0, "y"}, {"Rahsia", "h", 9.0, "q"}})
}

func TestAFormulaCellGivesItsCachedValue(t *testing.T) {
	file := handMade(t, sheet1, namesAndAmounts(`<c r="B2"><f>1+1</f><v>2</v></c>`, `<c r="B3"><f>2+2</f><v>4</v></c>`), "")
	wantRows(t, clean(t, file), []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Number}}, [][]any{{"a", 2.0}, {"b", 4.0}})
}

// Pinned: a formula the file never calculated has no value to read, so its column is dropped as empty and the last
// row, now blank in Jumlah, goes with the totals and notes at the bottom.
func TestAFormulaCellWithNoCachedValueIsEmpty(t *testing.T) {
	file := handMade(t, sheet1, namesAndAmounts(`<c r="B2"><f>1+1</f></c>`, `<c r="B3"><f>2+2</f></c>`), "")
	res := clean(t, file)
	wantRows(t, res, []lab.Column{{"Nama", lab.Text}}, [][]any{{"a"}})
	want := []string{"Dropped row 3 at the bottom: a total or note, not data.", "Columns with no data dropped: 1."}
	if got := changesIn(res, "en"); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %q, want %q", got, want)
	}
}

// Pinned: an error cell is its text, so a column with one reads as text.
func TestErrorCellsAreKeptAsTheirTextAndMakeTheColumnText(t *testing.T) {
	file := handMade(t, sheet1, namesAndAmounts(
		`<c r="B2" t="e"><v>#DIV/0!</v></c>`, `<c r="B3" t="e"><v>#N/A</v></c>`, `<c r="B4" t="e"><v>#REF!</v></c>`, `<c r="B5"><v>7</v></c>`), "")
	wantRows(t, clean(t, file), []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Text}},
		[][]any{{"a", "#DIV/0!"}, {"b", "#N/A"}, {"c", "#REF!"}, {"d", "7"}})
}

// jumlah is the Jumlah column of a sheet whose cells are all text.
func jumlah(t *testing.T, cells ...string) *lab.CleanUp {
	t.Helper()
	rows := make([][]any, len(cells))
	for i, c := range cells {
		rows[i] = []any{string(rune('a' + i)), c}
	}
	return clean(t, testutil.XLSX(t, []string{"Nama", "Jumlah"}, rows...))
}

// Pinned: an accounting negative in brackets is not read as a number, so -500 is the Learner's to write with a minus.
func TestABracketedAmountIsNotANumber(t *testing.T) {
	res := jumlah(t, "(500)", "10", "(20)")
	wantRows(t, res, []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Text}}, [][]any{{"a", "(500)"}, {"b", "10"}, {"c", "(20)"}})
}

func TestRMWithASpaceAndThousandsIsANumber(t *testing.T) {
	res := jumlah(t, "RM 1,234.50", "RM 10")
	wantRows(t, res, []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Number}}, [][]any{{"a", 1234.5}, {"b", 10.0}})
}

// Pinned: a percent sign keeps the column text.
func TestAPercentIsNotANumber(t *testing.T) {
	res := jumlah(t, "12%", "10")
	wantRows(t, res, []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Text}}, [][]any{{"a", "12%"}, {"b", "10"}})
}

func TestNumbersStoredAsTextAreNumbers(t *testing.T) {
	res := jumlah(t, "100", "250.5", "-30")
	wantRows(t, res, []lab.Column{{"Nama", lab.Text}, {"Jumlah", lab.Number}}, [][]any{{"a", 100.0}, {"b", 250.5}, {"c", -30.0}})
}

// dateBook is a workbook whose Tarikh cells are the serial numbers shown as dates, in the 1904 date system or not.
func dateBook(t *testing.T, d1904 bool, serials ...float64) []byte {
	t.Helper()
	f := excelize.NewFile()
	if d1904 {
		on := true
		if err := f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &on}); err != nil {
			t.Fatal(err)
		}
	}
	style, err := f.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		t.Fatal(err)
	}
	f.SetSheetRow("Sheet1", "A1", &[]any{"Tarikh", "Jumlah"})
	for i, s := range serials {
		a, b := fmt.Sprintf("A%d", i+2), fmt.Sprintf("B%d", i+2)
		f.SetCellValue("Sheet1", a, s)
		f.SetCellStyle("Sheet1", a, a, style)
		f.SetCellValue("Sheet1", b, i)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Pinned: the 1904 date system (Excel for Mac, old files) is read as 1900, so its dates come out 1,462 days early.
func TestA1904WorkbookIsReadWithThe1900DateSystem(t *testing.T) {
	res := clean(t, dateBook(t, true, 46000, 46001))
	wantRows(t, res, []lab.Column{{"Tarikh", lab.Date}, {"Jumlah", lab.Number}}, [][]any{{"2025-12-09", 0.0}, {"2025-12-10", 1.0}})
}

// Pinned: serial 60, Excel's 29 February 1900 that never was, reads as 28 February 1900; the days before it are a day early.
func TestExcelsFake29February1900ReadsAs28February(t *testing.T) {
	res := clean(t, dateBook(t, false, 59, 60, 61))
	wantRows(t, res, []lab.Column{{"Tarikh", lab.Date}, {"Jumlah", lab.Number}},
		[][]any{{"1900-02-27", 0.0}, {"1900-02-28", 1.0}, {"1900-03-01", 2.0}})
}

// Pinned: a date written in text with Malay month names stays text; only day/month/year numbers are dates.
func TestATextDateWithAMalayMonthNameStaysText(t *testing.T) {
	file := testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, []any{"5 Mac 2026", 1}, []any{"12 Mei 2026", 2}, []any{"1 Ogos 2026", 3})
	wantRows(t, clean(t, file), []lab.Column{{"Tarikh", lab.Text}, {"Jumlah", lab.Number}},
		[][]any{{"5 Mac 2026", 1.0}, {"12 Mei 2026", 2.0}, {"1 Ogos 2026", 3.0}})
}

func TestAFileThatIsNotAZipRenamedXLSXIsNotAWorkbook(t *testing.T) {
	if got := problemKey(t, []byte("Tarikh,Jumlah\n05/11/2026,RM5\n")); got != "lab.err.not-workbook" {
		t.Fatalf("problem = %q", got)
	}
}

func TestAZipWithNoWorkbookIsNotAWorkbook(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("hello.txt")
	w.Write([]byte("hi"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if got := problemKey(t, buf.Bytes()); got != "lab.err.not-workbook" {
		t.Fatalf("problem = %q", got)
	}
}

func TestATruncatedXLSXIsNotAWorkbook(t *testing.T) {
	file := testutil.XLSX(t, []string{"Nama", "Jumlah"}, []any{"a", 1})
	if got := problemKey(t, file[:len(file)/2]); got != "lab.err.not-workbook" {
		t.Fatalf("problem = %q", got)
	}
}
