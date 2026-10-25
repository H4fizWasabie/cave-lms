package lab_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
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
	d := readSample(t, tpl)
	cfg := tpl.Open(tpl.Guess(d), d)

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
	d := readSample(t, tpl)
	cfg := tpl.Open(tpl.Guess(d), d)
	c := tpl.Guess(d)
	if got, want := blockJSON(t, cfg, "Stock value"), `{"kind":"card","label":"Stock value","icon":"money","value":{"agg":"calc","a":"`+c["quantity"]+`","op":"x","b":"`+c["price"]+`","then":"sum"}}`; got != want {
		t.Errorf("Stock value =\n%s\nwant\n%s", got, want)
	}
	if got := blockJSON(t, cfg, "Running low"); !strings.Contains(got, `"locked":true,"low":["`+c["quantity"]+`","`+c["reorder"]+`"]`) {
		t.Errorf("Running low = %s", got)
	}
}

func TestOpeningLeavesOutBlocksWhoseChosenColumnIsGoneOrRetyped(t *testing.T) {
	tpl := finance(t)
	d := readSample(t, tpl)
	c := tpl.Guess(d)
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

func TestGenerateWorkWritesTheBlocksAndFilterBarIntoThePage(t *testing.T) {
	tpl := finance(t)
	d := readSample(t, tpl)
	cfg := tpl.Open(tpl.Guess(d), d)
	index, data, err := lab.GenerateWork(tpl, "Kedai <Ali>", "", cfg, d, "data/x")
	if err != nil {
		t.Fatal(err)
	}
	m := pageConfig.FindSubmatch(index)
	if m == nil {
		t.Fatalf("no config in\n%s", index)
	}
	var got struct {
		Work    bool `json:"work"`
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
	if !got.Work || !reflect.DeepEqual(got.Blocks, cfg.Blocks) || !reflect.DeepEqual(got.Filters, cfg.FilterBar) {
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

func TestACardShowsAnIconFromTheSetSuitedToItsValueUntilPicked(t *testing.T) {
	tpl := finance(t)
	d := readSample(t, tpl)
	cfg := tpl.Open(tpl.Guess(d), d)
	at, err := cfg.Add("card", d, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Blocks[at].Icon; got != "people" {
		t.Fatalf("a new card counting rows shows %q, want people", got)
	}
	// While the Learner has not picked one, the icon follows the value.
	b := cfg.Blocks[at]
	for _, c := range []struct {
		v    lab.Value
		want string
	}{
		{lab.Value{Agg: "sum", Col: "Jumlah"}, "money"},
		{lab.Value{Agg: "avg", Col: "Jumlah"}, "money"},
		{lab.Value{Agg: "distinct", Col: "Perkara"}, "tag"},
		{lab.Value{Agg: "calc", A: "Jumlah", Op: "/", B: "Jumlah", Then: "sum"}, "chart-up"},
	} {
		old := b
		b.Value = &c.v
		b.FollowIcon(old, cfg.Money)
		if b.Icon != c.want {
			t.Errorf("a card of %+v shows %q, want %s", c.v, b.Icon, c.want)
		}
	}
	old := b
	b.Icon = "star"
	b.FollowIcon(old, cfg.Money)
	old, b.Value = b, &lab.Value{Agg: "count"}
	b.FollowIcon(old, cfg.Money)
	if b.Icon != "star" {
		t.Errorf("a picked icon changed with the value to %q", b.Icon)
	}

	for icon, ok := range map[string]bool{"star": true, "none": true, "": true, "rocket": false, "Star": false} {
		b.Icon = icon
		if err := b.Check(d); (err == nil) != ok {
			t.Errorf("Check of icon %q = %v", icon, err)
		}
	}
}

func TestEveryTemplateCardNamesAnIconFromTheSet(t *testing.T) {
	for _, tpl := range lab.TemplatesFor(true) {
		d := readSample(t, tpl)
		for _, b := range tpl.Open(tpl.Guess(d), d).Blocks {
			if b.IsCard() && !slices.Contains(lab.Icons, b.Icon) {
				t.Errorf("%s card %q names icon %q", tpl.Key, b.Label, b.Icon)
			}
		}
	}
}

func TestABarStandsHorizontalUnlessSetVertical(t *testing.T) {
	d := &lab.Dataset{Columns: []lab.Column{{Name: "Staf", Type: lab.Text}}}
	for dir, want := range map[string]string{"": "", "horizontal": "", "vertical": "vertical"} {
		b := lab.WorkBlock{Kind: "bar", Value: &lab.Value{Agg: "count"}, Group: "Staf", Top: 8, Dir: dir}
		b.Fill(d)
		if err := b.Check(d); err != nil || b.Dir != want {
			t.Errorf("direction %q is stored as %q (%v), want %q", dir, b.Dir, err, want)
		}
	}
	b := lab.WorkBlock{Kind: "bar", Value: &lab.Value{Agg: "count"}, Group: "Staf", Top: 8, Dir: "diagonal"}
	b.Fill(d)
	if err := b.Check(d); err == nil {
		t.Error("a diagonal bar is accepted")
	}
	b = lab.WorkBlock{Kind: "donut", Value: &lab.Value{Agg: "count"}, Group: "Staf", Dir: "vertical"}
	if b.Fill(d); b.Dir != "" {
		t.Errorf("a donut keeps direction %q", b.Dir)
	}
}

func TestAHeatmapCrossesTwoDifferentTextColumns(t *testing.T) {
	d := &lab.Dataset{Columns: []lab.Column{{Name: "Jabatan", Type: lab.Text}, {Name: "Gaji", Type: lab.Number}, {Name: "Status", Type: lab.Text}}}
	var cfg lab.Config
	at, err := cfg.Add("heatmap", d, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := json.Marshal(cfg.Blocks[at]); string(got) != `{"kind":"heatmap","label":"Rows by Jabatan × Status","value":{"agg":"count"},"split":"Status","group":"Jabatan","top":8,"wide":true}` {
		t.Fatalf("a new heatmap = %s", got)
	}
	for _, c := range []struct {
		group, split string
		want         string
	}{
		{"Jabatan", "Jabatan", "lab.err.block-heat-cols"},
		{"Jabatan", "", "lab.err.block-heat-cols"},
		{"Jabatan", "Gaji", "lab.err.block-type"},
		{"Jabatan", "Cawangan", "lab.err.block-column"},
		{"Gaji", "Status", "lab.err.block-type"},
	} {
		b := lab.WorkBlock{Kind: "heatmap", Value: &lab.Value{Agg: "count"}, Group: c.group, Split: c.split, Top: 8}
		var p lab.Problem
		if err := b.Check(d); !errors.As(err, &p) || p.Key != c.want {
			t.Errorf("heatmap %s × %s: %v, want %s", c.group, c.split, err, c.want)
		}
	}
	// Its columns pause it when gone or retyped, and follow a rename.
	retyped := &lab.Dataset{Columns: []lab.Column{{Name: "Jabatan", Type: lab.Text}, {Name: "Status", Type: lab.Number}}}
	if got := cfg.Blocks[at].Missing(retyped); !reflect.DeepEqual(got, []string{"Status"}) {
		t.Errorf("missing = %v", got)
	}
	cfg.Rename(map[string]string{"Status": "Keadaan", "Jabatan": "Unit"})
	if b := cfg.Blocks[at]; b.Group != "Unit" || b.Split != "Keadaan" {
		t.Errorf("renamed heatmap = %+v", b)
	}
	// With one Text column, the Dataset cannot give it a column across.
	one := &lab.Dataset{Columns: []lab.Column{{Name: "Jabatan", Type: lab.Text}}}
	var p lab.Problem
	if _, err := cfg.Add("heatmap", one, "en"); !errors.As(err, &p) || p.Key != "lab.err.block-heat-cols" {
		t.Errorf("a heatmap on one Text column: %v", err)
	}
}
