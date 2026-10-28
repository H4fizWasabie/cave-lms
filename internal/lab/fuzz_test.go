package lab_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// checkClean holds for any file: Clean gives an error or a Dataset, never more rows than the workbook has, and the
// data.json made from the Dataset is valid JSON that gives every cell back as the Dataset holds it.
func checkClean(t *testing.T, file []byte, rowsIn int) {
	t.Helper()
	res, err := lab.Clean(bytes.NewReader(file), lab.Settings{})
	if err != nil {
		return
	}
	d := res.Dataset
	if d == nil {
		t.Fatal("Clean gave neither an error nor a Dataset")
	}
	if rowsIn >= 0 && len(d.Rows) > rowsIn {
		t.Fatalf("%d rows out of %d rows in", len(d.Rows), rowsIn)
	}
	rows, err := lab.PartRows(d, "2026-07-02")
	if err != nil {
		t.Fatalf("PartRows: %v", err)
	}
	data := []byte("[\n" + string(rows) + "\n]\n")
	if !json.Valid(data) {
		t.Fatalf("data.json is not valid JSON:\n%s", data)
	}
	back, err := lab.PartDataset(rows, d.Columns)
	if err != nil {
		t.Fatalf("PartDataset: %v", err)
	}
	for i, row := range d.Rows {
		for j, v := range row {
			if !reflect.DeepEqual(back.Rows[i][j], v) {
				t.Fatalf("row %d column %q: %#v came back as %#v", i, d.Columns[j].Name, v, back.Rows[i][j])
			}
		}
	}
	if _, err := lab.Page("Fuzz", "", lab.Build(d), d.Columns, "data.json"); err != nil {
		t.Fatalf("Page: %v", err)
	}
}

// sheetRowsIn is how many rows the workbook's sheets hold in all, or -1 if excelize cannot say.
func sheetRowsIn(file []byte) int {
	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		return -1
	}
	defer f.Close()
	n := 0
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return -1
		}
		n += len(rows)
	}
	return n
}

func FuzzClean(f *testing.F) {
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	f.Add(testutil.XLSX(f, []string{"Tarikh", "Perkara", "Jumlah"}, []any{day, "Sewa", "RM1,200.00"}, []any{day, "Gaji", 300}))
	f.Add(testutil.Workbook(f,
		testutil.Sheet{Name: "Jan", Rows: [][]any{{"Kategori", "Jumlah"}, {"A", 1}, {"Jumlah", 1}}},
		testutil.Sheet{Name: "Feb", Rows: [][]any{{"Kategori", "Jumlah"}, {"B", 2}}}))
	f.Add(testutil.Merge(f, testutil.XLSX(f, []string{"Kategori", "Jumlah"}, []any{"A", 1}, []any{nil, 2}), "A2:A3"))
	f.Add(handMade(f, "xl/worksheets/sheet1.xml", `<row r="1"><c r="A1" t="inlineStr"><is><t>Nama</t></is></c></row><row r="2"><c r="A2"><f>1+1</f></c></row>`, ""))
	f.Add([]byte("not a zip"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, file []byte) {
		checkClean(t, file, sheetRowsIn(file))
	})
}

func FuzzCleanCells(f *testing.F) {
	f.Add("Tarikh\tPerkara\tJumlah\n05/03/2026\tSewa\tRM 1,200.50\n06/03/2026\tGaji\t(500)")
	f.Add("A\tA\n1\t2\n3\t4")
	f.Add("Nama\tJumlah\n<b>\t12%\n\"x\"\t#DIV/0!\nJumlah\t3")
	f.Add("Hari\t1\t2\t3\nAli\tx\t\tx")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		// Tab splits cells and newline splits rows; the first row is the header.
		if !utf8.ValidString(s) || len(s) > 20000 {
			t.Skip()
		}
		var lines [][]any
		for _, line := range strings.Split(s, "\n") {
			var row []any
			for _, c := range strings.Split(line, "\t") {
				row = append(row, c)
			}
			lines = append(lines, row)
		}
		header := make([]string, len(lines[0]))
		for i, c := range lines[0] {
			header[i] = c.(string)
		}
		checkClean(t, testutil.XLSX(t, header, lines[1:]...), len(lines))
	})
}
