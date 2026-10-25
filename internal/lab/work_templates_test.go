package lab_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func blockLabels(cfg *lab.Config) []string {
	var l []string
	for _, b := range cfg.Blocks {
		l = append(l, b.Label)
	}
	return l
}

func TestSalesAndRosterCleanTheirSamplesAndGuessEveryBlank(t *testing.T) {
	for key, want := range map[string]struct {
		columns []lab.Column
		rows    int
		guess   lab.Choices
	}{
		"sales": {
			[]lab.Column{{"Tarikh", lab.Date}, {"Produk", lab.Text}, {"Kategori", lab.Text}, {"Staf", lab.Text}, {"Kaedah bayaran", lab.Text}, {"Kuantiti", lab.Number}, {"Jumlah", lab.Number}},
			90, lab.Choices{"date": "Tarikh", "amount": "Jumlah", "item": "Produk", "category": "Kategori", "staff": "Staf", "payment": "Kaedah bayaran", "quantity": "Kuantiti"},
		},
		"roster": {
			[]lab.Column{{"Tarikh", lab.Date}, {"Staf", lab.Text}, {"Jabatan", lab.Text}, {"Kod", lab.Text}},
			84, lab.Choices{"staff": "Staf", "date": "Tarikh", "code": "Kod", "department": "Jabatan"},
		},
	} {
		tpl := template(t, key)
		res := clean(t, tpl.Sample, tpl)
		if !reflect.DeepEqual(res.Dataset.Columns, want.columns) || len(res.Dataset.Rows) != want.rows {
			t.Errorf("%s Sample: columns %v, %d rows; want %v, %d rows", key, res.Dataset.Columns, len(res.Dataset.Rows), want.columns, want.rows)
		}
		if len(res.Changes) != 0 {
			t.Errorf("%s Sample needed Clean-up: %v", key, changesIn(res, "en"))
		}
		if !reflect.DeepEqual(res.Guess, want.guess) {
			t.Errorf("%s guesses = %v, want %v", key, res.Guess, want.guess)
		}
	}
}

func TestOpeningSalesGivesItsBlocksAndFilterBar(t *testing.T) {
	tpl := template(t, "sales")
	res := clean(t, tpl.Sample, tpl)
	cfg := tpl.Open(res.Guess, res.Dataset)

	want := []string{"Revenue", "Sales", "Units", "Revenue by month", "Revenue by item", "Revenue by staff", "Revenue by payment method", "Latest sales"}
	if got := blockLabels(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	for label, want := range map[string]string{
		"Revenue":                   `{"kind":"card","label":"Revenue","icon":"money","value":{"agg":"sum","col":"Jumlah"}}`,
		"Sales":                     `{"kind":"card","label":"Sales","icon":"cart","value":{"agg":"count"}}`,
		"Units":                     `{"kind":"card","label":"Units","icon":"box","value":{"agg":"sum","col":"Kuantiti"}}`,
		"Revenue by month":          `{"kind":"line","label":"Revenue by month","value":{"agg":"sum","col":"Jumlah"},"x":"Tarikh","by":"month","split":"Kategori","wide":true}`,
		"Revenue by item":           `{"kind":"bar","label":"Revenue by item","value":{"agg":"sum","col":"Jumlah"},"group":"Produk","top":8}`,
		"Revenue by staff":          `{"kind":"bar","label":"Revenue by staff","value":{"agg":"sum","col":"Jumlah"},"group":"Staf","top":8}`,
		"Revenue by payment method": `{"kind":"donut","label":"Revenue by payment method","value":{"agg":"sum","col":"Jumlah"},"group":"Kaedah bayaran"}`,
		"Latest sales":              `{"kind":"table","label":"Latest sales","cols":["Tarikh","Produk","Kategori","Staf","Kaedah bayaran","Kuantiti","Jumlah"],"sort":"Tarikh","limit":10,"wide":true}`,
	} {
		if got := blockJSON(t, cfg, label); got != want {
			t.Errorf("%s =\n%s\nwant\n%s", label, got, want)
		}
	}
	if want := []string{"Kategori", "Staf", "Kaedah bayaran", "Tarikh"}; !reflect.DeepEqual(cfg.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", cfg.FilterBar, want)
	}
	if want := []string{"Jumlah"}; !reflect.DeepEqual(cfg.Money, want) {
		t.Errorf("money = %v, want %v", cfg.Money, want)
	}

	// Only Date and Amount are required: with nothing else, the blocks needing the rest are left out.
	bare := tpl.Open(lab.Choices{"date": "Tarikh", "amount": "Jumlah"}, res.Dataset)
	if got, want := blockLabels(bare), []string{"Revenue", "Sales", "Revenue by month", "Latest sales"}; !reflect.DeepEqual(got, want) {
		t.Errorf("with Date and Amount only: blocks = %v, want %v", got, want)
	}
}

func TestOpeningRosterGivesItsBlocksAndFilterBar(t *testing.T) {
	tpl := template(t, "roster")
	res := clean(t, tpl.Sample, tpl)
	cfg := tpl.Open(res.Guess, res.Dataset)

	want := []string{"Staff", "Shifts", "Shifts by staff", "Shifts by code", "Shifts per day", "Roster"}
	if got := blockLabels(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	for label, want := range map[string]string{
		"Staff":           `{"kind":"card","label":"Staff","icon":"people","value":{"agg":"distinct","col":"Staf"}}`,
		"Shifts":          `{"kind":"card","label":"Shifts","icon":"clock","value":{"agg":"count"}}`,
		"Shifts by staff": `{"kind":"bar","label":"Shifts by staff","value":{"agg":"count"},"group":"Staf","top":8}`,
		"Shifts by code":  `{"kind":"bar","label":"Shifts by code","value":{"agg":"count"},"group":"Kod","top":8}`,
		"Shifts per day":  `{"kind":"line","label":"Shifts per day","value":{"agg":"count"},"x":"Tarikh","by":"day","wide":true}`,
		"Roster":          `{"kind":"table","label":"Roster","cols":["Tarikh","Staf","Jabatan","Kod"],"sort":"Tarikh","limit":50,"wide":true}`,
	} {
		if got := blockJSON(t, cfg, label); got != want {
			t.Errorf("%s =\n%s\nwant\n%s", label, got, want)
		}
	}
	if want := []string{"Jabatan", "Kod", "Staf", "Tarikh"}; !reflect.DeepEqual(cfg.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", cfg.FilterBar, want)
	}
}

func TestWorkDashboardsAlsoOfferSalesAndRoster(t *testing.T) {
	keys := func(ts []*lab.Template) []string {
		var k []string
		for _, t := range ts {
			k = append(k, t.Key)
		}
		return k
	}
	if got, want := keys(lab.TemplatesFor(true)), []string{"finance", "inventory", "hr", "sales", "roster", "general"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Work Templates = %v, want %v", got, want)
	}
	if got, want := keys(lab.TemplatesFor(false)), []string{"finance", "inventory", "hr", "general"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Practice Templates = %v, want %v", got, want)
	}
	for _, key := range []string{"sales", "roster"} {
		if tpl := template(t, key); tpl.Question == "" {
			t.Errorf("%s has no Dashboard question", key)
		}
	}
}

// A clinic's sales export names its columns its own way; Sales still finds the date and the amount.
func TestAClinicSalesSheetOpensAsSales(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Created Date & Time", "Invoice No", "Service", "Doctor", "Payment Mode", "Qty", "Price"},
		[]any{at, "INV-0001", "Konsultasi", "Dr Aina", "Tunai", 1, 40},
		[]any{at.Add(50 * time.Minute), "INV-0002", "Ujian darah", "Dr Hakim", "Kad", 2, 85.5},
	)
	tpl := template(t, "sales")
	res := clean(t, file, tpl)
	if res.Guess["date"] != "Created Date & Time" || res.Guess["amount"] != "Price" {
		t.Fatalf("guesses = %v, want Created Date & Time and Price", res.Guess)
	}
	if err := tpl.Check(res.Guess, res.Dataset); err != nil {
		t.Fatalf("the guesses do not open Sales: %v", err)
	}
	if got := blockJSON(t, tpl.Open(res.Guess, res.Dataset), "Revenue"); got != `{"kind":"card","label":"Revenue","icon":"money","value":{"agg":"sum","col":"Price"}}` {
		t.Errorf("Revenue = %s", got)
	}
}

// Sales and Roster guess columns named in words beyond their exact names.
func TestSalesAndRosterGuessLooselyNamedColumns(t *testing.T) {
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for key, c := range map[string]struct {
		header []string
		row    []any
		want   lab.Choices
	}{
		"sales": {
			[]string{"Tarikh Resit", "Nama Produk", "Served By", "Cara Bayar", "Bil. Unit", "Nilai (RM)"},
			[]any{at, "Kopi", "Aina", "Tunai", 2, 9.5},
			lab.Choices{"date": "Tarikh Resit", "item": "Nama Produk", "staff": "Served By", "payment": "Cara Bayar", "quantity": "Bil. Unit", "amount": "Nilai (RM)"},
		},
		"roster": {
			[]string{"Staff Name", "Shift Date", "Shift Code", "Ward"},
			[]any{"Aina", at, "P", "Wad 3"},
			lab.Choices{"staff": "Staff Name", "date": "Shift Date", "code": "Shift Code", "department": "Ward"},
		},
	} {
		res := clean(t, testutil.XLSX(t, c.header, c.row), template(t, key))
		if !reflect.DeepEqual(res.Guess, c.want) {
			t.Errorf("%s guesses = %v, want %v", key, res.Guess, c.want)
		}
	}
}

// A loose name that is part of another column's name, like unit in Unit Price, is never guessed by part; a column
// named exactly so still is.
func TestSalesAndRosterNeverGuessBySharedWords(t *testing.T) {
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		key    string
		header []string
		row    []any
		want   lab.Choices
	}{
		{"sales", []string{"Bil", "Tarikh", "Barang", "Unit Price", "Jumlah"}, []any{1, at, "Kopi", 4.5, 9}, lab.Choices{"date": "Tarikh", "item": "Barang", "amount": "Jumlah"}},
		{"roster", []string{"Nama", "Tarikh", "Kod", "Community"}, []any{"Aina", at, "P", "Taman Melati"}, lab.Choices{"staff": "Nama", "date": "Tarikh", "code": "Kod"}},
		{"roster", []string{"Nama", "Tarikh", "Kod", "Unit"}, []any{"Aina", at, "P", "ICU"}, lab.Choices{"staff": "Nama", "date": "Tarikh", "code": "Kod", "department": "Unit"}},
	} {
		res := clean(t, testutil.XLSX(t, c.header, c.row), template(t, c.key))
		if !reflect.DeepEqual(res.Guess, c.want) {
			t.Errorf("%s %v guesses = %v, want %v", c.key, c.header, res.Guess, c.want)
		}
	}
}

func TestSalesAndRosterDrawFromTheirSamples(t *testing.T) {
	for key, want := range map[string][]string{
		"sales":  {`<span>Sales</span><strong>90</strong>`, "Revenue by month", "Revenue by payment method", "Latest sales"},
		"roster": {`<span>Staff</span><strong>6</strong>`, `<span>Shifts</span><strong>84</strong>`, "Shifts per day", "Shifts by code"},
	} {
		tpl := template(t, key)
		res := clean(t, tpl.Sample, tpl)
		index, data, err := lab.GenerateWork(tpl, tpl.Name, "", tpl.Open(res.Guess, res.Dataset), res.Dataset, "data.json")
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		for name, b := range map[string][]byte{"index.html": index, "data.json": data} {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		out := node(t, filepath.Join(dir, "index.html"), filepath.Join(dir, "data.json"))
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("%s Dashboard lacks %q:\n%s", key, w, out)
			}
		}
	}
}
