package lab_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

func blockJSON(t *testing.T, cfg *lab.Config, label string) string {
	t.Helper()
	for _, b := range cfg.Blocks {
		if b.Label == label {
			out, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			return string(out)
		}
	}
	t.Fatalf("no block %q in %+v", label, cfg.Blocks)
	return ""
}

func TestOpeningFinanceGivesBlocksNamingTheChosenColumns(t *testing.T) {
	tpl := finance(t)
	res := sample(t, tpl)
	d := res.Dataset
	cfg := tpl.Open(res.Guess, d)

	for label, want := range map[string]string{
		"Spending by category": `{"kind":"bar","label":"Spending by category","value":{"agg":"sum","col":"Jumlah"},"group":"Kategori","top":8,"filter":{"col":"Jenis","vals":["Keluar"]}}`,
		"Transactions":         `{"kind":"card","label":"Transactions","icon":"check","value":{"agg":"count"}}`,
		"By month":             `{"kind":"line","label":"By month","value":{"agg":"sum","col":"Jumlah"},"x":"Tarikh","by":"month","split":"Jenis","wide":true}`,
		"Latest transactions":  `{"kind":"table","label":"Latest transactions","cols":["Tarikh","Perkara","Kategori","Jenis","Jumlah"],"sort":"Tarikh","limit":10,"wide":true}`,
		"Balance":              `{"kind":"card","label":"Balance","icon":"money","value":{"agg":"sum","col":"Jumlah"},"locked":true,"where":["Jenis","Masuk"],"minus":["Jenis","Keluar"]}`,
	} {
		if got := blockJSON(t, cfg, label); got != want {
			t.Errorf("%s =\n%s\nwant\n%s", label, got, want)
		}
	}
	for _, b := range cfg.Blocks {
		if b.Label == "Total" {
			t.Errorf("Total is left out once Type is chosen: %+v", cfg.Blocks)
		}
	}
	if want := []string{"Kategori", "Jenis", "Tarikh"}; !reflect.DeepEqual(cfg.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", cfg.FilterBar, want)
	}
	if want := []string{"Jumlah"}; !reflect.DeepEqual(cfg.Money, want) {
		t.Errorf("money = %v, want %v", cfg.Money, want)
	}
}

func TestOpeningInventoryKeepsLowStockLockedAndStockValueAsACalculation(t *testing.T) {
	tpl := lab.TemplateByKey("inventory")
	res := sample(t, tpl)
	d, c := res.Dataset, res.Guess
	cfg := tpl.Open(c, d)
	if got, want := blockJSON(t, cfg, "Stock value"), `{"kind":"card","label":"Stock value","icon":"money","value":{"agg":"calc","a":"`+c["quantity"]+`","op":"x","b":"`+c["price"]+`","then":"sum"}}`; got != want {
		t.Errorf("Stock value =\n%s\nwant\n%s", got, want)
	}
	if got := blockJSON(t, cfg, "Running low"); !strings.Contains(got, `"locked":true,"low":["`+c["quantity"]+`","`+c["reorder"]+`"]`) {
		t.Errorf("Running low = %s", got)
	}
}

func TestOpeningLeavesOutBlocksWhoseChosenColumnIsGoneOrRetyped(t *testing.T) {
	tpl := finance(t)
	res := sample(t, tpl)
	d, c := res.Dataset, res.Guess
	c["category"] = "Gone"  // no such column now
	c["type"] = c["amount"] // a Number column where Text is needed
	cfg := tpl.Open(c, d)
	for _, b := range cfg.Blocks {
		if b.Label == "Spending by category" || b.Label == "Balance" {
			t.Errorf("%s needs a column the Dataset no longer has: %+v", b.Label, b)
		}
	}
	if want := []string{c["date"]}; !reflect.DeepEqual(cfg.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", cfg.FilterBar, want)
	}
}

var pageConfig = regexp.MustCompile(`(?s)const dashboard = (\{.*?\n    \});`)

func TestGenerateWritesTheBlocksAndFilterBarIntoThePage(t *testing.T) {
	tpl := finance(t)
	res := sample(t, tpl)
	d := res.Dataset
	cfg := tpl.Open(res.Guess, d)
	index, data, err := lab.Generate(tpl, "Kedai <Ali>", "", cfg, d, "data/x")
	if err != nil {
		t.Fatal(err)
	}
	m := pageConfig.FindSubmatch(index)
	if m == nil {
		t.Fatalf("no config in\n%s", index)
	}
	var got struct {
		Columns map[string]struct {
			Name  string
			Type  lab.Type
			Money bool
		} `json:"columns"`
		Blocks  []lab.WorkBlock `json:"blocks"`
		Filters []string        `json:"filters"`
	}
	if err := json.Unmarshal(m[1], &got); err != nil {
		t.Fatalf("%v in\n%s", err, m[1])
	}
	if !reflect.DeepEqual(got.Blocks, cfg.Blocks) || !reflect.DeepEqual(got.Filters, cfg.FilterBar) {
		t.Fatalf("config = %+v", got)
	}
	if c := got.Columns["Jumlah"]; c.Name != "Jumlah" || c.Type != lab.Number || !c.Money || got.Columns["Perkara"].Money {
		t.Fatalf("columns = %+v", got.Columns)
	}
	if !strings.Contains(string(index), "Kedai &lt;Ali&gt;") || !strings.Contains(string(index), tpl.Question) {
		t.Fatalf("page names neither the Dashboard nor its question:\n%s", index)
	}
	if !strings.HasPrefix(string(data), "[\n  {\"Tarikh\": ") {
		t.Fatalf("data.json = %.80s", data)
	}
}
