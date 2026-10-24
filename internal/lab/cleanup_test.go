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

func TestCleanUpOfAnUnreadableFileGivesOneProblem(t *testing.T) {
	_, err := lab.Clean(strings.NewReader("Tarikh,Jumlah\n05/11/2026,RM5\n"), finance(t), lab.Settings{})
	var p lab.Problem
	if !errors.As(err, &p) || !strings.Contains(p.In("en"), "CSV") {
		t.Fatalf("err = %v, want one Problem naming CSV", err)
	}
}
