package lab_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func template(t *testing.T, key string) *lab.Template {
	t.Helper()
	tpl := lab.TemplateByKey(key)
	if tpl == nil {
		t.Fatalf("no %s Template", key)
	}
	return tpl
}

// dashboardConfig is the config Generate writes into index.html for charts.js.
type dashboardConfig struct {
	Columns map[string]struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Money bool   `json:"money"`
	} `json:"columns"`
	Blocks  []map[string]any `json:"blocks"`
	Filters []string         `json:"filters"`
}

func generate(t *testing.T, tpl *lab.Template, c lab.Choices, d *lab.Dataset) dashboardConfig {
	t.Helper()
	index, _, err := lab.Generate(tpl, "Mine", c, d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(index), "const dashboard = ")
	config, _, ok2 := strings.Cut(after, ";\n")
	if !ok || !ok2 {
		t.Fatalf("index.html has no dashboard config:\n%s", index)
	}
	var cfg dashboardConfig
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		t.Fatalf("config: %v\n%s", err, config)
	}
	return cfg
}

func labels(cfg dashboardConfig) []string {
	var l []string
	for _, b := range cfg.Blocks {
		l = append(l, b["label"].(string))
	}
	return l
}

func without(c lab.Choices, key string) lab.Choices {
	out := lab.Choices{}
	for k, v := range c {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func TestEveryTemplateReadsItsSampleAndGuessesEveryBlank(t *testing.T) {
	for key, want := range map[string]struct {
		columns []lab.Column
		rows    int
		guess   lab.Choices
	}{
		"inventory": {
			[]lab.Column{{"Kod Item", lab.Text}, {"Nama Item", lab.Text}, {"Kategori", lab.Text}, {"Kuantiti", lab.Number}, {"Paras Pesanan Semula", lab.Number}, {"Harga Seunit", lab.Number}},
			22, lab.Choices{"item": "Nama Item", "quantity": "Kuantiti", "reorder": "Paras Pesanan Semula", "category": "Kategori", "price": "Harga Seunit"},
		},
		"hr": {
			[]lab.Column{{"Nama", lab.Text}, {"Jabatan", lab.Text}, {"Jawatan", lab.Text}, {"Tarikh Mula", lab.Date}, {"Status", lab.Text}, {"Jantina", lab.Text}},
			30, lab.Choices{"name": "Nama", "department": "Jabatan", "joined": "Tarikh Mula", "status": "Status", "gender": "Jantina"},
		},
		"general": {
			[]lab.Column{{"Tarikh", lab.Date}, {"Cawangan", lab.Text}, {"Produk", lab.Text}, {"Unit", lab.Number}, {"Jualan", lab.Number}},
			81, lab.Choices{},
		},
	} {
		tpl := template(t, key)
		d := readSample(t, tpl)
		if !reflect.DeepEqual(d.Columns, want.columns) || len(d.Rows) != want.rows {
			t.Errorf("%s Sample: columns %v, %d rows; want %v, %d rows", key, d.Columns, len(d.Rows), want.columns, want.rows)
		}
		if got := tpl.Guess(d); !reflect.DeepEqual(got, want.guess) {
			t.Errorf("%s guesses = %v, want %v", key, got, want.guess)
		}
	}
}

func TestTheLabOffersFourTemplates(t *testing.T) {
	var keys []string
	for _, tpl := range lab.Templates {
		keys = append(keys, tpl.Key)
	}
	if want := []string{"finance", "inventory", "hr", "general"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("Templates = %v, want %v", keys, want)
	}
}

func TestInventoryBlocks(t *testing.T) {
	inv := template(t, "inventory")
	d := readSample(t, inv)
	cfg := generate(t, inv, inv.Guess(d), d)
	all := []string{"Stock value", "Total items", "Low stock", "Stock by category", "Running low", "All items"}
	if got := labels(cfg); !reflect.DeepEqual(got, all) {
		t.Fatalf("blocks = %v, want %v", got, all)
	}
	if !cfg.Columns["price"].Money || !reflect.DeepEqual(cfg.Filters, []string{"category"}) {
		t.Errorf("price column %+v, filters %v", cfg.Columns["price"], cfg.Filters)
	}
	value, low, list := cfg.Blocks[0], cfg.Blocks[2], cfg.Blocks[4]
	if value["of"] != "quantity" || value["times"] != "price" {
		t.Errorf("Stock value block = %v, want quantity times price", value)
	}
	for _, b := range []map[string]any{low, list} {
		if !reflect.DeepEqual(b["low"], []any{"quantity", "reorder"}) {
			t.Errorf("%v block does not pick rows at or below the reorder level", b["label"])
		}
	}

	for blank, want := range map[string][]string{
		"price":    {"Total items", "Low stock", "Stock by category", "Running low", "All items"},
		"reorder":  {"Stock value", "Total items", "Stock by category", "All items"},
		"category": {"Stock value", "Total items", "Low stock", "Running low", "All items"},
	} {
		if got := labels(generate(t, inv, without(inv.Guess(d), blank), d)); !reflect.DeepEqual(got, want) {
			t.Errorf("without %s: blocks = %v, want %v", blank, got, want)
		}
	}
}

func TestHRBlocks(t *testing.T) {
	hr := template(t, "hr")
	d := readSample(t, hr)
	cfg := generate(t, hr, hr.Guess(d), d)
	all := []string{"Headcount", "Active", "Headcount by department", "Status split", "Joins per month", "Staff"}
	if got := labels(cfg); !reflect.DeepEqual(got, all) {
		t.Fatalf("blocks = %v, want %v", got, all)
	}
	if active := cfg.Blocks[1]; active["agg"] != "count" || !reflect.DeepEqual(active["where"], []any{"status", "Aktif"}) {
		t.Errorf("Active block = %v, want a count where status is Aktif", active)
	}
	if want := []string{"department", "status", "gender", "joined"}; !reflect.DeepEqual(cfg.Filters, want) {
		t.Errorf("filters = %v, want %v", cfg.Filters, want)
	}

	for blank, want := range map[string][]string{
		"status": {"Headcount", "Headcount by department", "Joins per month", "Staff"},
		"joined": {"Headcount", "Active", "Headcount by department", "Status split", "Staff"},
		"gender": all,
	} {
		if got := labels(generate(t, hr, without(hr.Guess(d), blank), d)); !reflect.DeepEqual(got, want) {
			t.Errorf("without %s: blocks = %v, want %v", blank, got, want)
		}
	}
}

func TestGeneralBuildsBlocksFromColumnTypes(t *testing.T) {
	gen := template(t, "general")
	if len(gen.Blanks) != 0 {
		t.Fatalf("General has blanks %v", gen.Blanks)
	}
	d := readSample(t, gen)
	cfg := generate(t, gen, nil, d)
	want := []string{"Unit", "Jualan", "Unit by Cawangan", "Unit by month", "Latest rows"}
	if got := labels(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	kinds := []string{}
	for _, b := range cfg.Blocks {
		kinds = append(kinds, b["kind"].(string))
	}
	if want := []string{"card", "card", "bar", "line", "table"}; !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
	if table := cfg.Blocks[4]["cols"].([]any); len(table) != 5 {
		t.Errorf("table shows %v, want all 5 columns", table)
	}
	var filtered []string
	for _, k := range cfg.Filters {
		filtered = append(filtered, cfg.Columns[k].Name)
	}
	if !reflect.DeepEqual(filtered, []string{"Cawangan", "Tarikh"}) {
		t.Errorf("filters on %v, want Cawangan and Tarikh", filtered)
	}
}

func TestGeneralWithoutDatesOrNumbers(t *testing.T) {
	gen := template(t, "general")
	file := testutil.XLSX(t, []string{"Nama", "Bandar"}, []any{"Ali", "Ipoh"}, []any{"Abu", "Kuantan"})
	d, err := lab.Read(bytes.NewReader(file), gen)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := labels(generate(t, gen, nil, d)), []string{"Rows by Nama", "Rows"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}

	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file = testutil.XLSX(t, []string{"Bila", "Berapa"}, []any{day, 3.0})
	if d, err = lab.Read(bytes.NewReader(file), gen); err != nil {
		t.Fatal(err)
	}
	if got, want := labels(generate(t, gen, nil, d)), []string{"Berapa", "Berapa by month", "Latest rows"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
}

func TestEachTemplateShowsTheQuestionItsDashboardAnswers(t *testing.T) {
	for key, want := range map[string]string{
		"finance":   "Am I spending more than I earn?",
		"inventory": "What do I need to reorder?",
		"hr":        "Is the team growing, and where?",
	} {
		tpl := template(t, key)
		d := readSample(t, tpl)
		index, _, err := lab.Generate(tpl, "Mine", tpl.Guess(d), d, "data.json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(index), `<p class="dq">`+want+`</p>`) {
			t.Errorf("%s page has no Question %q", key, want)
		}
	}
	gen := template(t, "general")
	d := readSample(t, gen)
	index, _, err := lab.Generate(gen, "Mine", nil, d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), `class="dq"`) {
		t.Errorf("General page shows a Question")
	}
}

// The first card is drawn largest, so it holds the number the Question turns on.
func TestTheFirstCardIsTheHeadline(t *testing.T) {
	for _, tc := range []struct {
		key, skip, want string
	}{
		{"finance", "", "Balance"},
		{"finance", "type", "Total"},
		{"inventory", "", "Stock value"},
		{"inventory", "price", "Total items"},
		{"hr", "", "Headcount"},
		{"general", "", "Unit"},
	} {
		tpl := template(t, tc.key)
		d := readSample(t, tpl)
		cfg := generate(t, tpl, without(tpl.Guess(d), tc.skip), d)
		if got := labels(cfg)[0]; got != tc.want || cfg.Blocks[0]["kind"] != "card" {
			t.Errorf("%s without %q: first block = %v, want the %s card", tc.key, tc.skip, cfg.Blocks[0], tc.want)
		}
	}
}
