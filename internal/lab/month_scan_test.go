package lab_test

import (
	"reflect"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// A Number column is a month column when every filled value across all the Parts is a whole number from 1 to 12: a
// 13 or a 6.5 in any one Part, or no value at all, rules it out, and a Text column never is.
func TestAMonthScanFindsTheNumberColumnsOfWholeOneToTwelveAcrossTheParts(t *testing.T) {
	cols := []lab.Column{{"BULAN", lab.Number}, {"UMUR", lab.Number}, {"BAKI", lab.Number}, {"KOSONG", lab.Number}, {"NAMA", lab.Text}}
	scan := lab.NewMonthScan(cols)
	scan.Add(&lab.Dataset{Columns: cols, Rows: [][]any{{6.0, 12.0, 6.5, nil, "Mac"}, {7.0, nil, 1.0, nil, "Jun"}}})
	scan.Add(&lab.Dataset{Columns: cols, Rows: [][]any{{nil, 13.0, 2.0, nil, "Jul"}, {8.0, 1.0, 3.0, nil, "Ogos"}}})
	if got, want := scan.Columns(), []string{"BULAN"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("month columns = %v, want %v", got, want)
	}
	if got := lab.NewMonthScan(cols).Columns(); got == nil || len(got) != 0 {
		t.Fatalf("month columns of no Part = %#v, want empty and not nil", got)
	}
}
