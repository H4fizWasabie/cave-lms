package lab_test

import (
	"reflect"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// labels are cfg's blocks' labels, in order.
func labels(cfg *lab.Config) []string {
	var l []string
	for _, b := range cfg.Blocks {
		l = append(l, b.Label)
	}
	return l
}

func TestGeneralBuildsBlocksFromTheWorkbookByColumnRoles(t *testing.T) {
	res := clean(t, testdata(t, "general.xlsx"))
	d := res.Dataset
	if want := []lab.Column{{"Tarikh", lab.Date}, {"Cawangan", lab.Text}, {"Produk", lab.Text}, {"Unit", lab.Number}, {"Jualan", lab.Number}}; !reflect.DeepEqual(d.Columns, want) || len(d.Rows) != 81 {
		t.Fatalf("columns %v, %d rows; want %v, 81 rows", d.Columns, len(d.Rows), want)
	}
	cfg := lab.Build(d)
	// The first card is drawn largest, so it holds the main measure.
	want := []string{"Jualan", "Unit", "Rows", "Jualan by month", "Jualan by Produk", "Jualan by Cawangan", "Latest rows"}
	if got := labels(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(cfg.FilterBar, []string{"Tarikh", "Produk", "Cawangan"}) {
		t.Errorf("filter bar %v, want Tarikh, Produk and Cawangan", cfg.FilterBar)
	}
}
