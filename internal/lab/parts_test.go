package lab_test

import (
	"errors"
	"reflect"
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
	if _, _, err := lab.AddPart(cols, 1, more); !errors.As(err, &p) || p.Key != "lab.err.add-columns" {
		t.Fatalf("err = %v, want lab.err.add-columns", err)
	}
	full := &lab.Dataset{Columns: cols, Rows: [][]any{{"Gula", 1.0}}}
	if _, _, err := lab.AddPart(cols, lab.MaxCleanRows, full); !errors.As(err, &p) || p.Key != "lab.err.add-too-many-rows" {
		t.Fatalf("err = %v, want lab.err.add-too-many-rows", err)
	}
}
