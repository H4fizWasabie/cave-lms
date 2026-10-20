package lab_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
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
	index, data, err := lab.Generate(fin, "Kewangan Kedai", fin.Guess(d), d)
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
	index, _, err := lab.Generate(fin, "Kewangan", c, d)
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
		if _, _, err := lab.Generate(fin, "Kewangan", c, d); err == nil {
			t.Errorf("%s: Generate accepted %v", name, c)
		}
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
	index, data, err := lab.Generate(fin, "<i>Mine</i>", lab.Choices{"date": "Tarikh", "amount": "Jumlah", "category": "</script><b>Kategori</b>"}, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range [][]byte{index, data} {
		if bytes.Contains(file, []byte("<b>")) || bytes.Contains(file, []byte("<script>alert")) || bytes.Contains(file, []byte("<i>")) || bytes.Contains(file, []byte("</script><b>")) {
			t.Fatalf("hostile text left unescaped:\n%s", file)
		}
	}
}
