package lab_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"runtime"
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

// allocated is how many bytes fn allocates.
func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// sample is tpl's Sample cleaned, as the Lab cleans it at start-up.
func sample(t *testing.T, tpl *lab.Template) *lab.CleanUp {
	t.Helper()
	return clean(t, tpl.Sample, tpl)
}

func TestGenerateWritesThePageAndTheDatasetAsDataJSON(t *testing.T) {
	fin := finance(t)
	res := sample(t, fin)
	d := res.Dataset
	index, data, err := lab.Generate(fin, "Kewangan Kedai", "", fin.Open(res.Guess, d), d, "data.json")
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

func TestGeneratedPageEscapesColumnNamesAndCellText(t *testing.T) {
	fin := finance(t)
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "</script><b>Kategori</b>", "Jumlah"}, []any{day, "<script>alert(1)</script>", 12.5})
	d := clean(t, file, fin).Dataset
	index, data, err := lab.Generate(fin, "<i>Mine</i>", "", fin.Open(lab.Choices{"date": "Tarikh", "amount": "Jumlah", "category": "</script><b>Kategori</b>"}, d), d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range [][]byte{index, data} {
		if bytes.Contains(file, []byte("<b>")) || bytes.Contains(file, []byte("<script>alert")) || bytes.Contains(file, []byte("<i>")) || bytes.Contains(file, []byte("</script><b>")) {
			t.Fatalf("hostile text left unescaped:\n%s", file)
		}
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
	_, err := lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) {
		t.Fatalf("Clean gave %v, want one lab.Problem", err)
	}
	if want := "This workbook is over 50 MB once unpacked. The Lab reads workbooks up to 50 MB unpacked. Delete sheets, pictures or rows you do not need, and upload again."; p.In("en") != want {
		t.Fatalf("errors = %v, want %v", p.In("en"), want)
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
	_, err := lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) || p.Key != "lab.err.too-many-cells" {
		t.Fatalf("got %v, want lab.err.too-many-cells", err)
	}
}

// sharedStringBomb has a header of cells columns and one row whose cells all point at the same shared string of
// size bytes, so the file stays a few KB however long the string.
func sharedStringBomb(t *testing.T, cells, size int) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for c := 1; c <= cells; c++ {
		name, _ := excelize.ColumnNumberToName(c)
		f.SetCellValue("Sheet1", name+"1", "Column "+name)
		f.SetCellValue("Sheet1", name+"2", "PLACEHOLDER")
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	in, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	out := zip.NewWriter(&buf)
	for _, entry := range in.File {
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		part, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == "xl/sharedStrings.xml" {
			part = bytes.Replace(part, []byte("PLACEHOLDER"), bytes.Repeat([]byte("x"), size), 1)
		}
		w, err := out.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(part)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAWorkbookWhoseSharedTextPasses50MBOnceGivenToEachCellIsRefusedUnread(t *testing.T) {
	file := sharedStringBomb(t, 100, 1<<20) // 100 cells of the same 1 MB string: 100 MB of text
	if len(file) > 64<<10 {
		t.Fatalf("the file is %d bytes; it should be a few KB", len(file))
	}
	var err error
	if n := allocated(func() { _, err = lab.Clean(bytes.NewReader(file), finance(t), lab.Settings{}) }); n > 32<<20 {
		t.Errorf("Clean allocated %d MB; it should refuse the file before giving the string to each cell", n>>20)
	}
	var p lab.Problem
	if !errors.As(err, &p) || p.Key != "lab.err.too-big-unzipped" {
		t.Fatalf("got %v, want lab.err.too-big-unzipped", err)
	}
}

func TestAWorkbookWhoseCellsShareShortTextIsRead(t *testing.T) {
	file := sharedStringBomb(t, 3, 1000)
	if _, err := lab.Clean(bytes.NewReader(file), lab.General, lab.Settings{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
}
