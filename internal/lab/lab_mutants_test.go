package lab

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestDateFormatTellsDateNumberFormatsFromTheRest(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	style := func(s excelize.Style) int {
		id, err := f.NewStyle(&s)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// Built-in formats that show a date; the numbers just outside each run are none.
	dates := []int{14, 15, 16, 17, 22, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 50, 51, 52, 53, 54, 55, 56, 57, 58}
	for n := 0; n <= 60; n++ {
		if got, want := dateFormat(f, style(excelize.Style{NumFmt: n})), slices.Contains(dates, n); got != want {
			t.Errorf("dateFormat(built-in format %d) = %t, want %t", n, got, want)
		}
	}
	for format, want := range map[string]bool{
		"dd/mm/yyyy":         true,
		"yyyy":               true,
		"d":                  true,
		"DD-MMM":             true,
		"h:mm:ss":            false, // minutes alone are not a date
		"0.00":               false,
		`0.00 "day"`:         false, // a d inside quotes is text
		`[Red]0.00`:          false, // and so is one inside brackets
		`"year" 0`:           false,
		`[$-409]d/m/yy`:      true,
		`0.00 "year" yyyy`:   true,
		`[Red]"day" h:mm`:    false,
		`[$RM-43E] #,##0.00`: false,
	} {
		if got := dateFormat(f, style(excelize.Style{CustomNumFmt: &format})); got != want {
			t.Errorf("dateFormat(custom format %q) = %t, want %t", format, got, want)
		}
	}
	if dateFormat(f, 9999) {
		t.Error("a style the workbook does not have shows a date")
	}
}

// zipOf is a zip of the parts, in order.
func zipOf(t *testing.T, parts ...[2]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, p := range parts {
		f, err := w.Create(p[0])
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(p[1]))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// archive is a zip of the parts, in order, as sharedText reads it.
func archive(t *testing.T, parts ...[2]string) *zip.Reader {
	t.Helper()
	b := zipOf(t, parts...)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return z
}

// sharedStrings is a shared strings part holding the strings; a string starting "<r>" is rich text, the runs inside it.
func sharedStrings(texts ...string) [2]string {
	var b strings.Builder
	b.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range texts {
		if run, ok := strings.CutPrefix(s, "<r>"); ok {
			fmt.Fprintf(&b, "<si>%s</si>", run)
		} else {
			fmt.Fprintf(&b, "<si><t>%s</t></si>", s)
		}
	}
	b.WriteString("</sst>")
	return [2]string{"xl/sharedStrings.xml", b.String()}
}

// sheetOf is a worksheet part with one cell per attribute set and value.
func sheetOf(name string, cells ...string) [2]string {
	var b strings.Builder
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1">`)
	for _, c := range cells {
		b.WriteString(c)
	}
	b.WriteString("</row></sheetData></worksheet>")
	return [2]string{"xl/worksheets/" + name + ".xml", b.String()}
}

func shared(v string) string { return `<c t="s"><v>` + v + `</v></c>` }

func TestSharedTextCountsTheTextEachCellPointingAtASharedStringIsGiven(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	ten, forty, sixty := strings.Repeat("a", 10), strings.Repeat("b", 40), strings.Repeat("c", 60)
	for _, c := range []struct {
		name  string
		parts [][2]string
		limit int
		want  int
	}{
		{"each cell gets its own copy", [][2]string{sharedStrings(forty, ten),
			sheetOf("sheet1", shared("0"), shared("0"), shared("1"))}, 1000, 90},
		{"the first string counts", [][2]string{sharedStrings(forty), sheetOf("sheet1", shared("0"))}, 1000, 40},
		{"the runs of rich text add up", [][2]string{sharedStrings("<r><r><t>abc</t></r><r><t>de</t></r>"), sheetOf("sheet1", shared("0"))}, 1000, 5},
		{"a number in the cell's v is read trimmed", [][2]string{sharedStrings(ten, forty), sheetOf("sheet1", shared(" 1 "))}, 1000, 40},
		{"an index with no string, or no index, counts nothing", [][2]string{sharedStrings(ten, forty),
			sheetOf("sheet1", shared("2"), shared("-1"), shared("x"), shared(""), shared("1"))}, 1000, 40},
		{"only cells of type s", [][2]string{sharedStrings(ten), sheetOf("sheet1", `<c t="str"><v>0</v></c>`, `<c><v>0</v></c>`, `<f t="s"><v>0</v></f>`)}, 1000, 0},
		{"no shared strings", [][2]string{sheetOf("sheet1", shared("0"))}, 1000, 0},
		// The count stops once past the limit, having read the cell that took it past.
		{"the cell that passes the limit is counted", [][2]string{sharedStrings(forty, sixty, ten),
			sheetOf("sheet1", shared("0"), shared("1"), shared("2"), shared("2"))}, 100, 110},
		{"a total at the limit reads on", [][2]string{sharedStrings(forty, sixty, ten),
			sheetOf("sheet1", shared("0"), shared("1"), shared("2"))}, 100, 110},
		{"a total at the limit reads on in the next part", [][2]string{sharedStrings(sixty, forty, ten),
			sheetOf("sheet1", shared("0"), shared("1")), sheetOf("sheet2", shared("2"))}, 100, 110},
	} {
		if got := sharedText(f, archive(t, c.parts...), c.limit); got != c.want {
			t.Errorf("%s: sharedText = %d, want %d", c.name, got, c.want)
		}
	}
}

// workbookWith is a small workbook of the cells of Sheet1 written by fill, as a zip.Reader's bytes.
func workbookWith(t *testing.T, fill func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	fill(f)
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// problemKey is the key of err if it is a Problem, else its text.
func problemKey(err error) string {
	var p Problem
	if errors.As(err, &p) {
		return p.Key
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// padded is file with one more part of zeros, so that its parts unzip to exactly total bytes.
func padded(t *testing.T, file []byte, total uint64) []byte {
	t.Helper()
	in, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	out := zip.NewWriter(&b)
	var size uint64
	for _, e := range in.File {
		w, err := out.CreateHeader(&zip.FileHeader{Name: e.Name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.Open()
		if err != nil {
			t.Fatal(err)
		}
		n, err := io.Copy(w, r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		size += uint64(n)
	}
	w, err := out.CreateHeader(&zip.FileHeader{Name: "padding.bin", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(make([]byte, total-size)); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestOpenWorkbookRefusesAFileOnlyPastTheUnzippedLimit(t *testing.T) {
	small := workbookWith(t, func(f *excelize.File) { f.SetCellValue("Sheet1", "A1", "Tarikh") })
	for total, want := range map[uint64]string{MaxUnzipped: "", MaxUnzipped + 1: "lab.err.too-big-unzipped"} {
		f, err := openWorkbook(bytes.NewReader(padded(t, small, total)))
		if got := problemKey(err); got != want {
			t.Errorf("a file unzipping to %d bytes: error %q, want %q", total, got, want)
		}
		if f != nil {
			f.Close()
		}
	}
}

func TestOpenWorkbookRefusesSharedTextOnlyPastTheUnzippedLimit(t *testing.T) {
	// 50 cells share a string just under 1 MB, a few KB in the file; one more cell holds a string of its own, whose
	// length is what brings the text the cells are given to exactly MaxUnzipped, or one byte more.
	build := func(extra int) []byte {
		file := workbookWith(t, func(f *excelize.File) {
			for c := 1; c <= 50; c++ {
				name, _ := excelize.ColumnNumberToName(c)
				f.SetCellValue("Sheet1", name+"1", "Column "+name)
				f.SetCellValue("Sheet1", name+"2", "PLACEHOLDER")
			}
			f.SetCellValue("Sheet1", "A3", "EXTRA")
		})
		in, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
		if err != nil {
			t.Fatal(err)
		}
		var parts [][2]string
		for _, e := range in.File {
			r, _ := e.Open()
			part, _ := io.ReadAll(r)
			r.Close()
			if e.Name == "xl/sharedStrings.xml" {
				part = bytes.Replace(part, []byte("PLACEHOLDER"), bytes.Repeat([]byte("x"), 1<<20-100), 1)
				part = bytes.Replace(part, []byte("EXTRA"), bytes.Repeat([]byte("y"), extra), 1)
			}
			parts = append(parts, [2]string{e.Name, string(part)})
		}
		return zipOf(t, parts...)
	}
	counted := func(file []byte) int {
		z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
		if err != nil {
			t.Fatal(err)
		}
		return sharedText(excelize.NewFile(), z, 1<<60)
	}
	extra := MaxUnzipped - counted(build(1)) + 1
	for n, want := range map[int]string{extra: "", extra + 1: "lab.err.too-big-unzipped"} {
		file := build(n)
		if got, want := counted(file), MaxUnzipped+n-extra; got != want {
			t.Fatalf("the shared text counts %d, want %d", got, want)
		}
		f, err := openWorkbook(bytes.NewReader(file))
		if got := problemKey(err); got != want {
			t.Errorf("shared text of %d bytes: error %q, want %q", MaxUnzipped+n-extra, got, want)
		}
		if f != nil {
			f.Close()
		}
	}
}

func TestOpenWorkbookTellsAnOldXlsFileFromOneThatIsNoWorkbook(t *testing.T) {
	good := workbookWith(t, func(f *excelize.File) { f.SetCellValue("Sheet1", "A1", "Tarikh") })
	for name, c := range map[string]struct {
		file []byte
		want string
	}{
		"xls":                       {append(slices.Clone(oldExcel), "rest of the file"...), "lab.err.xls"},
		"text":                      {[]byte("Tarikh,Jumlah"), "lab.err.not-workbook"},
		"empty":                     {nil, "lab.err.not-workbook"},
		"a zip that is no workbook": {zipOf(t, [2]string{"notes.txt", "not a workbook"}), "lab.err.not-workbook"},
		"workbook":                  {good, ""},
	} {
		f, err := openWorkbook(bytes.NewReader(c.file))
		if got := problemKey(err); got != c.want {
			t.Errorf("%s: error %q, want %q", name, got, c.want)
		}
		if f != nil {
			f.Close()
		}
	}
}

func TestSheetRowsReadsASheetOfExactlyTheMostCellsAndTrimsTrailingEmptyRows(t *testing.T) {
	// A cell in the last column pads its row out to excelize.MaxColumns cells, so the sheet is small in the file
	// and large as read: the header, 305 rows of 16,384, and a last row of 2,879 come to maxSheetCells.
	file := workbookWith(t, func(f *excelize.File) {
		f.SetCellValue("Sheet1", "A1", "Tarikh")
		for r := 2; r <= 306; r++ {
			f.SetCellValue("Sheet1", fmt.Sprint("XFD", r), 1)
		}
		last, _ := excelize.CoordinatesToCellName(2879, 307)
		f.SetCellValue("Sheet1", last, 1)
	})
	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := sheetRows(f, "Sheet1")
	if err != nil || len(rows) != 307 {
		t.Fatalf("sheetRows of %d cells = %d rows, %v; want 307 rows", maxSheetCells, len(rows), err)
	}

	f2 := excelize.NewFile()
	defer f2.Close()
	f2.SetCellValue("Sheet1", "A1", "Tarikh")
	f2.SetCellValue("Sheet1", "A2", "x")
	f2.SetRowHeight("Sheet1", 4, 30) // a row with no cell
	rows, err = sheetRows(f2, "Sheet1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("sheetRows = %q, %v; want the 2 rows with a cell", rows, err)
	}
}

func TestParseReadsADateSerialNumberFromTheFirstDayToTheLastExcelHas(t *testing.T) {
	for in, want := range map[string]string{"1": "1900-01-01", "2958465": "9999-12-31", "45000": "2023-03-15"} {
		if got, ok := parse(cell{v: in}, Date, false); !ok || got != want {
			t.Errorf("parse(%s) = %v, %t; want %s", in, got, ok, want)
		}
	}
	for _, in := range []string{"0", "0.5", "2958466", "-1", "abc", ""} {
		if got, ok := parse(cell{v: in}, Date, false); ok {
			t.Errorf("parse(%q) = %v, want no date", in, got)
		}
	}
}

func TestThousandsPutsACommaBeforeEachThreeDigitsButTheFirst(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 100: "100", 999: "999", 1000: "1,000", 12345: "12,345", 123456: "123,456", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestColumnsOfNamesTheColumnsAndRefusesTheFirstBlankOrRepeatedName(t *testing.T) {
	cols, err := columnsOf([]string{" Tarikh ", "Jumlah"})
	if err != nil || !slices.Equal(cols, []Column{{Name: "Tarikh"}, {Name: "Jumlah"}}) {
		t.Fatalf("columnsOf = %v, %v", cols, err)
	}
	for header, want := range map[string]Problem{
		"a, ,b":  {"lab.err.unnamed-column", []any{"B"}},
		"a,b,":   {"lab.err.unnamed-column", []any{"C"}},
		"a,b,B":  {"lab.err.same-name", []any{"B"}},
		"a,b, A": {"lab.err.same-name", []any{"A"}},
	} {
		_, err := columnsOf(strings.Split(header, ","))
		var p Problem
		if !errors.As(err, &p) || p.Key != want.Key || fmt.Sprint(p.Args) != fmt.Sprint(want.Args) {
			t.Errorf("columnsOf(%q) = %v, want %v", header, err, want)
		}
	}
}

func TestTypeWordIsTheTextKeyOfAType(t *testing.T) {
	if got := typeWord(Number); got.Key != "lab.type.number" || got.Args != nil {
		t.Errorf("typeWord(Number) = %+v", got)
	}
}
