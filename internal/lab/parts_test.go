package lab_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// A Part is stored as its rows with the Part date on each, and reads back as the same Dataset.
func TestAPartKeepsItsRowsAndCarriesItsDateOnEachRow(t *testing.T) {
	d := &lab.Dataset{Columns: []lab.Column{{Name: "Barang", Type: lab.Text}, {Name: "Baki", Type: lab.Number}}, Rows: [][]any{{"Gula", 12.5}, {"Tepung", nil}}}
	data, err := lab.PartRows(d, "2026-07-02")
	if err != nil {
		t.Fatal(err)
	}
	want := "  {\"Barang\": \"Gula\", \"Baki\": 12.5, \"Part date\": \"2026-07-02\"},\n  {\"Barang\": \"Tepung\", \"Baki\": null, \"Part date\": \"2026-07-02\"}"
	if string(data) != want {
		t.Fatalf("Part = %q, want %q", data, want)
	}
	back, err := lab.PartDataset(data, d.Columns)
	if err != nil || !reflect.DeepEqual(back.Rows, d.Rows) {
		t.Fatalf("read back %v (%v), want %v", back.Rows, err, d.Rows)
	}
}

// A Part is read for the Dataset's columns; it is refused if it lacks one, or if the Dataset would pass the row limit.
func TestAddPartNeedsTheDatasetsColumnsAndKeepsToTheRowLimit(t *testing.T) {
	cols := []lab.Column{{Name: "Barang", Type: lab.Text}, {Name: "Baki", Type: lab.Number}}
	more := &lab.Dataset{Columns: []lab.Column{{Name: "Barang", Type: lab.Text}}, Rows: [][]any{{"Gula"}}}
	var p lab.Problem
	if _, _, err := lab.AddPart(cols, 1, more, lab.MaxCleanRows); !errors.As(err, &p) || p.Key != "lab.err.add-columns" {
		t.Fatalf("err = %v, want lab.err.add-columns", err)
	}
	full := &lab.Dataset{Columns: cols, Rows: [][]any{{"Gula", 1.0}}}
	if _, _, err := lab.AddPart(cols, lab.MaxCleanRows, full, lab.MaxCleanRows); !errors.As(err, &p) || p.Key != "lab.err.add-too-many-rows" {
		t.Fatalf("err = %v, want lab.err.add-too-many-rows", err)
	}
	// A limit of 0 is no row total: the Dataset behind a Learner's own page.
	if _, _, err := lab.AddPart(cols, lab.MaxCleanRows, full, 0); err != nil {
		t.Fatalf("err = %v, want none without a row limit", err)
	}
}

// A Part with extra columns goes in as the Dataset's columns only; the extras are dropped and listed, and a Part
// lacking a column is refused naming it, whatever the order.
func TestAddPartDropsExtraColumnsAndListsThemAndAcceptsAnyOrder(t *testing.T) {
	cols := []lab.Column{{Name: "Barang", Type: lab.Text}, {Name: "Baki", Type: lab.Number}}
	more := &lab.Dataset{
		Columns: []lab.Column{{Name: "Nota", Type: lab.Text}, {Name: "baki", Type: lab.Number}, {Name: "Rak", Type: lab.Text}, {Name: "Barang", Type: lab.Text}, {Name: "Warna", Type: lab.Text}},
		Rows:    [][]any{{"n", 3.0, "r", "Gula", "putih"}},
	}
	out, changes, err := lab.AddPart(cols, 1, more, lab.MaxCleanRows)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"Gula", 3.0}}; !reflect.DeepEqual(out.Rows, want) || !reflect.DeepEqual(out.Columns, cols) {
		t.Fatalf("Part = %v in %v, want %v in %v", out.Rows, out.Columns, want, cols)
	}
	if len(changes) != 1 || changes[0].Key != "lab.clean.extra-columns" || changes[0].Args[0] != 3 {
		t.Fatalf("changes = %+v, want the three extra columns dropped, as one change", changes)
	}
	if got, want := changes[0].In("en"), "Columns not in your data, dropped (3): Nota, Rak, Warna."; got != want {
		t.Fatalf("change = %q, want %q", got, want)
	}
	if got, want := changes[0].In("ms"), "Lajur yang tiada dalam data anda, dibuang (3): Nota, Rak, Warna."; got != want {
		t.Fatalf("change = %q, want %q", got, want)
	}

	short := &lab.Dataset{Columns: []lab.Column{{Name: "Barang", Type: lab.Text}}, Rows: [][]any{{"Gula"}}}
	_, _, err = lab.AddPart(append(cols, lab.Column{Name: "Rak", Type: lab.Text}), 1, short, 0)
	var p lab.Problem
	if !errors.As(err, &p) || !strings.Contains(p.In("en"), "Missing: Baki, Rak.") {
		t.Fatalf("err = %v, want a refusal naming Baki and Rak", err)
	}
}

// A Part whose columns are named otherwise is read as the Dataset's: a column whose name is the same ignoring case,
// spaces, underscores and dashes matches by itself, and the ones the Learner chose are mapped; the Dataset's names
// and types win, and what is left over is dropped and listed as ever.
func TestMapColumnsReadsRenamedColumnsAsTheDatasetsAndListsWhatItMapped(t *testing.T) {
	cols := []lab.Column{{Name: "PPD", Type: lab.Text}, {Name: "KOD_INSTITUSI", Type: lab.Text}, {Name: "NAMA_PENUH_INSTITUSI", Type: lab.Text}, {Name: "JUMHADIR", Type: lab.Number}}
	more := &lab.Dataset{
		Columns: []lab.Column{{Name: "JPN", Type: lab.Text}, {Name: "PPD", Type: lab.Text}, {Name: "KOD INSTITUSI", Type: lab.Text}, {Name: "NAMA INSTITUSI", Type: lab.Text}, {Name: "Jumlah  Hadir", Type: lab.Text}},
		Rows:    [][]any{{"JOHOR", "Batu Pahat", "JBA1", "SK Maju", "30"}, {"JOHOR", "Muar", "JBA2", "SK Jaya", "x"}},
	}
	chosen := map[string]string{"NAMA INSTITUSI": "NAMA_PENUH_INSTITUSI", "jumlah hadir": "JUMHADIR"}
	mapped, ms, changes := lab.MapColumns(cols, more, chosen)
	out, dropped, err := lab.AddPart(cols, 1, mapped, lab.MaxCleanRows)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"Batu Pahat", "JBA1", "SK Maju", 30.0}, {"Muar", "JBA2", "SK Jaya", nil}}; !reflect.DeepEqual(out.Rows, want) {
		t.Fatalf("rows = %v, want %v", out.Rows, want)
	}
	want := []lab.Mapping{{File: "KOD INSTITUSI", Column: "KOD_INSTITUSI"}, {File: "NAMA INSTITUSI", Column: "NAMA_PENUH_INSTITUSI", Chosen: true}, {File: "Jumlah  Hadir", Column: "JUMHADIR", Chosen: true}}
	if !reflect.DeepEqual(ms, want) {
		t.Fatalf("mapped = %+v, want %+v", ms, want)
	}
	var got []string
	for _, c := range append(changes, dropped...) {
		got = append(got, c.In("en"))
	}
	wantText := []string{
		"Columns matched to your data's by name (1): KOD INSTITUSI → KOD_INSTITUSI.",
		"Columns mapped to your data's (2): NAMA INSTITUSI → NAMA_PENUH_INSTITUSI, Jumlah  Hadir → JUMHADIR.",
		"Columns not in your data, dropped (1): JPN.",
		"Cells in JUMHADIR that were not numbers, left empty: 1.",
	}
	if !reflect.DeepEqual(got, wantText) {
		t.Fatalf("changes = %q, want %q", got, wantText)
	}

	// A column still unmapped is refused, naming it.
	unmapped, _, _ := lab.MapColumns(cols, more, nil)
	_, _, err = lab.AddPart(cols, 1, unmapped, 0)
	var p lab.Problem
	if !errors.As(err, &p) || !strings.Contains(p.In("en"), "Missing: NAMA_PENUH_INSTITUSI, JUMHADIR.") {
		t.Fatalf("err = %v, want a refusal naming the two still unmapped", err)
	}
}
