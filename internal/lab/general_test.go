package lab_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// general cleans a workbook of header and rows and opens General on it.
func general(t *testing.T, header []string, rows ...[]any) *lab.Config {
	t.Helper()
	return lab.General.Open(nil, clean(t, testutil.XLSX(t, header, rows...), lab.General).Dataset)
}

// kinds counts the Config's blocks of each kind.
func kinds(cfg *lab.Config) map[string]int {
	n := map[string]int{}
	for _, b := range cfg.Blocks {
		n[b.Kind]++
	}
	return n
}

func TestGeneralMakesAtMostFourCards(t *testing.T) {
	var header []string
	for i := range 11 {
		header = append(header, fmt.Sprintf("Kolum %d", i+1))
	}
	var rows [][]any
	for r := range 20 {
		var row []any
		for i := range 11 {
			row = append(row, float64(r*i+r%3))
		}
		rows = append(rows, row)
	}
	if n := kinds(general(t, header, rows...))["card"]; n != 4 {
		t.Fatalf("%d cards, want 4", n)
	}
}

// grouped are the columns the Config's charts group by, split by or draw over.
func grouped(cfg *lab.Config) []string {
	var out []string
	for _, b := range cfg.Blocks {
		for _, c := range []string{b.Group, b.Split, b.X} {
			if c != "" {
				out = append(out, c)
			}
		}
	}
	return out
}

// valueOf is the value of the Config's block labelled label, or nil.
func valueOf(cfg *lab.Config, label string) *lab.Value {
	for _, b := range cfg.Blocks {
		if b.Label == label {
			return b.Value
		}
	}
	return nil
}

// stockList is a stock report: a unique SKU and product name, a code column, one branch, three categories, and
// a price, a quantity and a running balance.
func stockList(n int) (header []string, rows [][]any) {
	header = []string{"SKU", "Product Name", "Kod Lokasi", "Branch", "Category", "Unit Price", "Quantity", "Balance", "Remarks"}
	for i := range n {
		rows = append(rows, []any{fmt.Sprintf("SKU-%03d", i), fmt.Sprintf("Item %d", i), fmt.Sprintf("L%d", i%20),
			"Ipoh", []string{"Food", "Drink", "Snack"}[i%3], float64(2 + i%5), float64(i), float64(100 - i),
			fmt.Sprintf("Checked by store keeper on the morning shift, shelf %d rearranged.", i%4)})
	}
	return header, rows
}

func TestGeneralNeverChartsAnIdentifierConstantOrFreeText(t *testing.T) {
	h, rows := stockList(30)
	cfg := general(t, h, rows...)
	for _, g := range grouped(cfg) {
		if g != "Category" {
			t.Errorf("a chart groups by %s; only Category may group", g)
		}
	}
	if !slices.Contains(grouped(cfg), "Category") {
		t.Errorf("no chart by Category: %+v", cfg.Blocks)
	}
	if slices.ContainsFunc(cfg.FilterBar, func(c string) bool { return c != "Category" }) {
		t.Errorf("filter bar = %v, want Category only", cfg.FilterBar)
	}
}

func TestGeneralAveragesPricesAndBalancesAndAddsAPriceTimesQuantityCard(t *testing.T) {
	h, rows := stockList(30)
	cfg := general(t, h, rows...)
	for label, want := range map[string]lab.Value{
		"Quantity":              {Agg: "sum", Col: "Quantity"},
		"Quantity × Unit Price": {Agg: "calc", A: "Quantity", Op: "x", B: "Unit Price", Then: "sum"},
		"Average Unit Price":    {Agg: "avg", Col: "Unit Price"},
		"Average Balance":       {Agg: "avg", Col: "Balance"},
	} {
		if got := valueOf(cfg, label); got == nil || *got != want {
			t.Errorf("%s card = %+v, want %+v; blocks %+v", label, got, want, cfg.Blocks)
		}
	}
	for _, b := range cfg.Blocks {
		if v := b.Value; v != nil && v.Agg == "sum" && (v.Col == "Unit Price" || v.Col == "Balance") {
			t.Errorf("%s adds up %s", b.Label, v.Col)
		}
	}
	cost := general(t, []string{"Cost", "Stock Level", "Score %"}, []any{1.5, 3.0, 50.0}, []any{2.5, 4.0, 70.0})
	for _, label := range []string{"Average Cost", "Average Stock Level", "Average Score %"} {
		if v := valueOf(cost, label); v == nil || v.Agg != "avg" {
			t.Errorf("no %s card: %+v", label, cost.Blocks)
		}
	}
}

func TestGeneralChartsARosterWithNoNumbersByRowCounts(t *testing.T) {
	var rows [][]any
	for i := range 24 {
		rows = append(rows, []any{fmt.Sprintf("Staf %d", i%8), []string{"P", "M", "C"}[i%3], []string{"Wad 1", "Wad 2"}[i%2]})
	}
	cfg := general(t, []string{"Nama", "Syif", "Wad"}, rows...)
	if v := valueOf(cfg, "Rows"); v == nil || v.Agg != "count" {
		t.Fatalf("no Rows card: %+v", cfg.Blocks)
	}
	var bars []string
	for _, b := range cfg.Blocks {
		if b.Kind == "bar" || b.Kind == "donut" {
			bars = append(bars, b.Kind+" "+b.Group)
			if b.Value.Agg != "count" {
				t.Errorf("%s value %+v, want a row count", b.Label, b.Value)
			}
		}
	}
	if want := []string{"bar Nama", "bar Syif", "donut Wad"}; !slices.Equal(bars, want) {
		t.Errorf("charts %v, want %v", bars, want)
	}
}

// dated is a sales list of n rows a step apart from 1 March 2026.
func dated(n int, step time.Duration) (header []string, rows [][]any) {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		rows = append(rows, []any{day.Add(time.Duration(i) * step), []string{"Tunai", "Kad"}[i%2], float64(10 + i)})
	}
	return []string{"Tarikh", "Bayaran", "Jumlah"}, rows
}

func TestGeneralDrawsTheLineByDayOrMonthBySpanAndNoneWithoutTwoPeriods(t *testing.T) {
	line := func(h []string, rows [][]any) *lab.WorkBlock {
		cfg := general(t, h, rows...)
		for i, b := range cfg.Blocks {
			if b.Kind == "line" {
				return &cfg.Blocks[i]
			}
		}
		return nil
	}
	if l := line(dated(20, 24*time.Hour)); l == nil || l.By != "day" || l.X != "Tarikh" || l.Split != "Bayaran" || l.Value.Col != "Jumlah" {
		t.Errorf("over 20 days the line is %+v, want Jumlah by day split by Bayaran", l)
	}
	if l := line(dated(20, 7*24*time.Hour)); l == nil || l.By != "month" {
		t.Errorf("over 20 weeks the line is %+v, want by month", l)
	}
	if l := line(dated(1, 0)); l != nil {
		t.Errorf("one day has a line %+v", l)
	}
	h, rows := dated(20, 24*time.Hour)
	for _, r := range rows {
		r[0] = "Mac"
	}
	if l := line(h, rows); l != nil {
		t.Errorf("no dates, yet a line %+v", l)
	}
}

func TestGeneralKeepsADonutForTwoToSixValues(t *testing.T) {
	var rows [][]any
	for i := range 40 {
		rows = append(rows, []any{fmt.Sprintf("Cawangan %d", i%10), fmt.Sprintf("Produk %d", i%9), fmt.Sprintf("Staf %d", i%7), float64(i)})
	}
	if n := kinds(general(t, []string{"Cawangan", "Produk", "Staf", "Jualan"}, rows...))["donut"]; n != 0 {
		t.Errorf("%d donuts with no column of 2 to 6 values", n)
	}
}

func TestGeneralReadsAYearColumnAsAGroupingNotAnAmount(t *testing.T) {
	cfg := general(t, []string{"Tahun", "Jumlah"}, []any{2024.0, 5.0}, []any{2025.0, 7.0}, []any{2025.0, 1.0})
	for _, b := range cfg.Blocks {
		if v := b.Value; v != nil && v.Col == "Tahun" {
			t.Errorf("%s adds up the years", b.Label)
		}
	}
}

func TestTheTableShowsTheDateNewestFirstAnIdentifierAndTheMainColumns(t *testing.T) {
	h, rows := stockList(30)
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := range rows {
		rows[i] = append(rows[i], day.AddDate(0, 0, i))
	}
	cfg := general(t, append(h, "Tarikh"), rows...)
	table := cfg.Blocks[len(cfg.Blocks)-1]
	if want := []string{"Tarikh", "SKU", "Category", "Quantity"}; table.Kind != "table" || table.Sort != "Tarikh" || !slices.Equal(table.Cols[:4], want) {
		t.Fatalf("table = %+v, want sorted by Tarikh with %v first", table, want)
	}
	if !slices.Equal(cfg.FilterBar, []string{"Tarikh", "Category"}) {
		t.Errorf("filter bar = %v, want Tarikh and Category", cfg.FilterBar)
	}
}

func TestCleanUpNotesEachColumnLeftOutOfChartsOrAveragedInBothLanguages(t *testing.T) {
	h, rows := stockList(30)
	res := clean(t, testutil.XLSX(t, h, rows...), lab.General)
	var en, ms []string
	for _, n := range res.Notes {
		en, ms = append(en, n.In("en")), append(ms, n.In("ms"))
	}
	want := []string{
		"SKU: an ID or code, so it's only in the table.",
		"Product Name: different on every row, so it's only in the table.",
		"Kod Lokasi: an ID or code, so it's only in the table.",
		"Branch: one value on every row, so it isn't charted.",
		"Unit Price: a price, rate, balance or level, so it's averaged, not added up.",
		"Balance: a price, rate, balance or level, so it's averaged, not added up.",
		"Remarks: notes or long text, so it's only in the table.",
	}
	if !slices.Equal(en, want) {
		t.Errorf("notes =\n%s\nwant\n%s", strings.Join(en, "\n"), strings.Join(want, "\n"))
	}
	if len(ms) == 0 || ms[1] != "Product Name: berbeza pada setiap baris, jadi ia hanya dalam jadual." {
		t.Errorf("Malay notes = %v", ms)
	}
}

func TestGeneralNeverAddsUpADateKeptAsNumbers(t *testing.T) {
	var rows [][]any
	for i := range 20 {
		rows = append(rows, []any{float64(i + 1), float64(45900 + i), float64(100 * i)})
	}
	cfg := general(t, []string{"Date", "Tarikh Bayar", "Total Amount"}, rows...)
	for _, b := range cfg.Blocks {
		if v := b.Value; v != nil && (v.Col == "Date" || v.Col == "Tarikh Bayar") {
			t.Errorf("%s adds up %s", b.Label, v.Col)
		}
	}
}

func TestGeneralNeverGroupsByANumberedColumnEvenWithFewValues(t *testing.T) {
	var rows [][]any
	for i := range 30 {
		rows = append(rows, []any{fmt.Sprintf("B%d", i%4), []string{"P", "M", "C"}[i%3], []string{"Wad 1", "Wad 2"}[i%2]})
	}
	cfg := general(t, []string{"Batch No.", "Kod Syif", "Wad"}, rows...)
	if g := grouped(cfg); slices.Contains(g, "Batch No.") || !slices.Contains(g, "Kod Syif") {
		t.Errorf("charts group by %v, want Kod Syif and never Batch No.", g)
	}
}

func TestWithNothingAddedUpThePriceTimesQuantityCardComesFirst(t *testing.T) {
	cfg := general(t, []string{"Cost Price", "Actual Stock"}, []any{2.0, 3.0}, []any{4.0, 5.0})
	if first := cfg.Blocks[0]; first.Value == nil || first.Value.Agg != "calc" {
		t.Errorf("first card %+v, want the calculation", first)
	}
}

func TestANewCardAveragesAPriceAndANewBarCountsAsGeneralDoes(t *testing.T) {
	var rows [][]any
	for i := range 12 {
		rows = append(rows, []any{fmt.Sprintf("SKU-%02d", i), []string{"Food", "Drink", "Snack"}[i%3], float64(2 + i%4)})
	}
	d := clean(t, testutil.XLSX(t, []string{"SKU", "Category", "Unit Price"}, rows...), lab.General).Dataset
	cfg := lab.General.Open(nil, d)
	for kind, want := range map[string]lab.Value{"card": {Agg: "avg", Col: "Unit Price"}, "bar": {Agg: "count"}} {
		at, err := cfg.Add(kind, d, "en")
		if err != nil {
			t.Fatal(err)
		}
		if b := cfg.Blocks[at]; *b.Value != want || (kind == "bar" && b.Group != "Category") {
			t.Errorf("a new %s is %+v by %q, want %+v", kind, *b.Value, b.Group, want)
		}
	}
}
