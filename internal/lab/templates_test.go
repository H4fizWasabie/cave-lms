package lab_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

func template(t *testing.T, key string) *lab.Template {
	t.Helper()
	tpl := lab.TemplateByKey(key)
	if tpl == nil {
		t.Fatalf("no %s Template", key)
	}
	return tpl
}

// labels are cfg's blocks' labels, in order.
func labels(cfg *lab.Config) []string {
	var l []string
	for _, b := range cfg.Blocks {
		l = append(l, b.Label)
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

func TestEveryTemplateCleansItsSampleAndGuessesEveryBlank(t *testing.T) {
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
		res := sample(t, tpl)
		if d := res.Dataset; !reflect.DeepEqual(d.Columns, want.columns) || len(d.Rows) != want.rows {
			t.Errorf("%s Sample: columns %v, %d rows; want %v, %d rows", key, d.Columns, len(d.Rows), want.columns, want.rows)
		}
		if got := res.Guess; len(got) != len(want.guess) || len(got) > 0 && !reflect.DeepEqual(got, want.guess) {
			t.Errorf("%s guesses = %v, want %v", key, got, want.guess)
		}
	}
}

func TestInventoryBlocks(t *testing.T) {
	inv := template(t, "inventory")
	res := sample(t, inv)
	d := res.Dataset
	cfg := inv.Open(res.Guess, d)
	all := []string{"Stock value", "Total items", "Low stock", "Stock by category", "Running low", "All items"}
	if got := labels(cfg); !reflect.DeepEqual(got, all) {
		t.Fatalf("blocks = %v, want %v", got, all)
	}
	if !reflect.DeepEqual(cfg.Money, []string{"Harga Seunit"}) || !reflect.DeepEqual(cfg.FilterBar, []string{"Kategori"}) {
		t.Errorf("money columns %v, filter bar %v", cfg.Money, cfg.FilterBar)
	}
	value, low, list := cfg.Blocks[0], cfg.Blocks[2], cfg.Blocks[4]
	if v := value.Value; v == nil || v.Agg != "calc" || v.A != "Kuantiti" || v.Op != "x" || v.B != "Harga Seunit" {
		t.Errorf("Stock value block = %+v, want quantity times price", value)
	}
	for _, b := range []lab.WorkBlock{low, list} {
		if !reflect.DeepEqual(b.Low, []string{"Kuantiti", "Paras Pesanan Semula"}) {
			t.Errorf("%v block does not pick rows at or below the reorder level", b.Label)
		}
	}

	for blank, want := range map[string][]string{
		"price":    {"Total items", "Low stock", "Stock by category", "Running low", "All items"},
		"reorder":  {"Stock value", "Total items", "Stock by category", "All items"},
		"category": {"Stock value", "Total items", "Low stock", "Running low", "All items"},
	} {
		if got := labels(inv.Open(without(res.Guess, blank), d)); !reflect.DeepEqual(got, want) {
			t.Errorf("without %s: blocks = %v, want %v", blank, got, want)
		}
	}
}

func TestHRBlocks(t *testing.T) {
	hr := template(t, "hr")
	res := sample(t, hr)
	d := res.Dataset
	cfg := hr.Open(res.Guess, d)
	all := []string{"Headcount", "Active", "Headcount by department", "Status split", "Joins per month", "Staff"}
	if got := labels(cfg); !reflect.DeepEqual(got, all) {
		t.Fatalf("blocks = %v, want %v", got, all)
	}
	if active := cfg.Blocks[1]; active.Value == nil || active.Value.Agg != "count" || !reflect.DeepEqual(active.Filter, &lab.Filter{Col: "Status", Vals: []string{"Aktif"}}) {
		t.Errorf("Active block = %+v, want a count where Status is Aktif", active)
	}
	if want := []string{"Jabatan", "Status", "Jantina", "Tarikh Mula"}; !reflect.DeepEqual(cfg.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", cfg.FilterBar, want)
	}

	for blank, want := range map[string][]string{
		"status": {"Headcount", "Headcount by department", "Joins per month", "Staff"},
		"joined": {"Headcount", "Active", "Headcount by department", "Status split", "Staff"},
		"gender": all,
	} {
		if got := labels(hr.Open(without(res.Guess, blank), d)); !reflect.DeepEqual(got, want) {
			t.Errorf("without %s: blocks = %v, want %v", blank, got, want)
		}
	}
}

func TestGeneralBuildsBlocksFromTheSampleByColumnRoles(t *testing.T) {
	gen := template(t, "general")
	if len(gen.Blanks) != 0 {
		t.Fatalf("General has blanks %v", gen.Blanks)
	}
	d := sample(t, gen).Dataset
	cfg := gen.Open(nil, d)
	want := []string{"Jualan", "Unit", "Rows", "Jualan by month", "Jualan by Produk", "Jualan by Cawangan", "Latest rows"}
	if got := labels(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(cfg.FilterBar, []string{"Tarikh", "Produk", "Cawangan"}) {
		t.Errorf("filter bar %v, want Tarikh, Produk and Cawangan", cfg.FilterBar)
	}
}

func TestEachTemplateShowsTheQuestionItsDashboardAnswers(t *testing.T) {
	for key, want := range map[string]string{
		"finance":   "Am I spending more than I earn?",
		"inventory": "What do I need to reorder?",
		"hr":        "Is the team growing, and where?",
	} {
		tpl := template(t, key)
		res := sample(t, tpl)
		index, _, err := lab.Generate(tpl, "Mine", "", tpl.Open(res.Guess, res.Dataset), res.Dataset, "data.json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(index), `<p class="dq">`+want+`</p>`) {
			t.Errorf("%s page has no Question %q", key, want)
		}
	}
	gen := template(t, "general")
	d := sample(t, gen).Dataset
	index, _, err := lab.Generate(gen, "Mine", "", gen.Open(nil, d), d, "data.json")
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
		{"general", "", "Jualan"},
	} {
		tpl := template(t, tc.key)
		res := sample(t, tpl)
		cfg := tpl.Open(without(res.Guess, tc.skip), res.Dataset)
		if got := labels(cfg)[0]; got != tc.want || cfg.Blocks[0].Kind != "card" {
			t.Errorf("%s without %q: first block = %v, want the %s card", tc.key, tc.skip, cfg.Blocks[0], tc.want)
		}
	}
}
