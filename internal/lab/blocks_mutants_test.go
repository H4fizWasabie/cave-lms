package lab_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

func TestUsesAreTheColumnsTheBlocksNameEachOnceWithTheTypeAnyBlockNeedsOfIt(t *testing.T) {
	cfg := &lab.Config{Blocks: []lab.WorkBlock{
		{Kind: "card", Value: &lab.Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "card", Value: &lab.Value{Agg: "distinct", Col: "Kod"}},
		{Kind: "card", Value: &lab.Value{Agg: "calc", A: "Kuantiti", Op: "x", B: "Harga", Then: "sum"}},
		{Kind: "card", Value: &lab.Value{Agg: "count"}},
		{Kind: "line", X: "Tarikh", Split: "Jenis", Value: &lab.Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "bar", Group: "Cawangan"},
		// Cawangan is named with Text by the bar above and with no type by the table; Kod the other way round.
		{Kind: "table", Cols: []string{"Cawangan", "Tarikh", "Kod"}, Sort: "Jumlah"},
		{Kind: "bar", Group: "Kod", Filter: &lab.Filter{Col: "Status", Vals: []string{"Baru"}}},
		{Kind: "table", Where: []string{"Negeri", "Perak", "Johor"}, Minus: []string{"Daerah", "Ipoh"}, Low: []string{"Stok", "Baki"}},
		{Kind: "table"},
	}}
	want := []lab.Use{
		{"Jumlah", lab.Number}, {"Kod", lab.Text}, {"Kuantiti", lab.Number}, {"Harga", lab.Number}, {"Tarikh", lab.Date},
		{"Jenis", lab.Text}, {"Cawangan", lab.Text}, {"Status", lab.Text}, {"Negeri", lab.Text}, {"Daerah", lab.Text},
		{"Stok", lab.Number}, {"Baki", lab.Number},
	}
	if got := cfg.Uses(); !slices.Equal(got, want) {
		t.Errorf("Uses = %v\nwant   %v", got, want)
	}
}

func TestUsesKeepTheFirstTypeAnyBlockAsksOfAColumn(t *testing.T) {
	cfg := &lab.Config{Blocks: []lab.WorkBlock{
		{Kind: "bar", Group: "Cawangan"},
		{Kind: "table", Cols: []string{"Cawangan"}},
		{Kind: "card", Value: &lab.Value{Agg: "sum", Col: "Cawangan"}},
	}}
	if got, want := cfg.Uses(), []lab.Use{{"Cawangan", lab.Text}}; !slices.Equal(got, want) {
		t.Errorf("Uses = %v, want %v", got, want)
	}
	if got := (&lab.Config{}).Uses(); len(got) != 0 {
		t.Errorf("no blocks use %v", got)
	}
}

func TestUsesOfABlockWithNoWhereMinusOrFilterAreJustItsOwnColumns(t *testing.T) {
	cfg := &lab.Config{Blocks: []lab.WorkBlock{{Kind: "bar", Group: "Cawangan", Where: []string{}, Minus: nil}}}
	if got, want := cfg.Uses(), []lab.Use{{"Cawangan", lab.Text}}; !slices.Equal(got, want) {
		t.Errorf("Uses = %v, want %v", got, want)
	}
}

func TestAUseFitsADatasetWithTheColumnOfTheTypeTheBlocksNeed(t *testing.T) {
	d := &lab.Dataset{Columns: []lab.Column{{"Tarikh", lab.Date}, {"Jumlah", lab.Number}, {"Cawangan", lab.Text}, {"Jenis", lab.Text}}}
	for _, c := range []struct {
		use  lab.Use
		want bool
	}{
		{lab.Use{"Jumlah", lab.Number}, true},
		{lab.Use{"Jumlah", ""}, true},
		{lab.Use{"Jumlah", lab.Text}, false},
		{lab.Use{"Cawangan", lab.Text}, true},
		{lab.Use{"Cawangan", lab.Date}, false},
		{lab.Use{"Hilang", ""}, false},
		{lab.Use{"Hilang", lab.Text}, false},
		{lab.Use{"", ""}, false},
	} {
		if got := c.use.Fits(d); got != c.want {
			t.Errorf("%v.Fits = %t, want %t", c.use, got, c.want)
		}
	}
}

func TestAUseOffersTheColumnsOfTheTypeTheBlocksNeedInOrder(t *testing.T) {
	d := &lab.Dataset{Columns: []lab.Column{{"Tarikh", lab.Date}, {"Jumlah", lab.Number}, {"Cawangan", lab.Text}, {"Kuantiti", lab.Number}, {"Jenis", lab.Text}}}
	for _, c := range []struct {
		use  lab.Use
		want []string
	}{
		{lab.Use{"x", lab.Text}, []string{"Cawangan", "Jenis"}},
		{lab.Use{"x", lab.Number}, []string{"Jumlah", "Kuantiti"}},
		{lab.Use{"x", lab.Date}, []string{"Tarikh"}},
		{lab.Use{"x", ""}, []string{"Tarikh", "Jumlah", "Cawangan", "Kuantiti", "Jenis"}},
	} {
		if got := c.use.Options(d); !slices.Equal(got, c.want) {
			t.Errorf("%v.Options = %v, want %v", c.use, got, c.want)
		}
	}
	if got := (lab.Use{"x", lab.Date}).Options(&lab.Dataset{}); len(got) != 0 {
		t.Errorf("a Dataset with no columns offers %v", got)
	}
}

// renameable is a Config with every place a column may be named holding A or B or neither.
func renameable() *lab.Config {
	return &lab.Config{
		Blocks: []lab.WorkBlock{
			{Kind: "card", Label: "A", Value: &lab.Value{Agg: "calc", Col: "A", A: "B", Op: "x", B: "C", Then: "sum"}},
			{Kind: "line", X: "A", Split: "B", Group: "C", Sort: "A", Value: &lab.Value{Agg: "count"}},
			{Kind: "bar", Group: "B", Filter: &lab.Filter{Col: "A", Vals: []string{"A", "B"}}},
			{Kind: "table", Cols: []string{"A", "C", "B"}, Low: []string{"B", "A"}, Where: []string{"A", "B"}, Minus: []string{"B", "A"}},
			{Kind: "table"},
		},
		FilterBar: []string{"A", "C"},
		Money:     []string{"B"},
	}
}

func TestRenameSwapsColumnsInEveryPlaceAtOnceLeavingValuesAndLabelsAlone(t *testing.T) {
	cfg := renameable()
	cfg.Rename(map[string]string{"A": "B", "B": "A", "": "Z"})
	want := &lab.Config{
		Blocks: []lab.WorkBlock{
			{Kind: "card", Label: "A", Value: &lab.Value{Agg: "calc", Col: "B", A: "A", Op: "x", B: "C", Then: "sum"}},
			{Kind: "line", X: "B", Split: "A", Group: "C", Sort: "B", Value: &lab.Value{Agg: "count"}},
			{Kind: "bar", Group: "A", Filter: &lab.Filter{Col: "B", Vals: []string{"A", "B"}}},
			{Kind: "table", Cols: []string{"B", "C", "A"}, Low: []string{"A", "B"}, Where: []string{"B", "B"}, Minus: []string{"A", "A"}},
			{Kind: "table"},
		},
		FilterBar: []string{"B", "C"},
		Money:     []string{"A"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Rename gave\n%+v\nwant\n%+v", cfg, want)
	}
}

func TestRenameLeavesTheBlocksTheCallerHeldBeforeAsTheyWere(t *testing.T) {
	cfg := renameable()
	before := cfg.Blocks
	cfg.Rename(map[string]string{"A": "X", "B": "Y"})
	if !reflect.DeepEqual(before, renameable().Blocks) {
		t.Errorf("Rename changed the blocks held before it:\n%+v", before)
	}
	if &before[0] == &cfg.Blocks[0] {
		t.Error("Rename put its changes in the same blocks")
	}
}
