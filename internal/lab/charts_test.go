package lab_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// node runs charts_test.js with args, skipping the test where node is not installed (CI has it).
func node(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	out, err := exec.Command("node", append([]string{"charts_test.js"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return string(out)
}

func TestChartsComputesEveryBlockOption(t *testing.T) {
	if out := node(t); strings.TrimSpace(out) != "ok" {
		t.Fatal(out)
	}
}

func TestAWorkDashboardWithHandSetBlocksDraws(t *testing.T) {
	d := &lab.Dataset{
		Columns: []lab.Column{{Name: "Tarikh", Type: lab.Date}, {Name: "Staf", Type: lab.Text}, {Name: "Jumlah", Type: lab.Number}, {Name: "Kos", Type: lab.Number}},
		Rows: [][]any{
			{"2026-01-05", "Ali", 100.0, 40.0},
			{"2026-01-06", "Siti", 50.0, nil},
			{"2026-02-03", nil, 30.0, 20.0},
		},
	}
	cards := []lab.WorkBlock{}
	for _, l := range []string{"C1", "C2", "C3", "C4", "C5", "C6"} {
		cards = append(cards, lab.WorkBlock{Kind: "card", Label: l, Value: &lab.Value{Agg: "count"}})
	}
	cards[0].Icon = "star"
	cfg := &lab.Config{
		Money:     []string{"Jumlah"},
		FilterBar: []string{"Staf", "Tarikh", "Cawangan"},
		Blocks: append(cards,
			lab.WorkBlock{Kind: "card", Label: "Untung", Value: &lab.Value{Agg: "calc", A: "Jumlah", Op: "-", B: "Kos", Then: "sum"}},
			lab.WorkBlock{Kind: "card", Label: "Purata", Icon: "none", Value: &lab.Value{Agg: "avg", Col: "Jumlah"}},
			lab.WorkBlock{Kind: "line", Label: "Ikut hari", Value: &lab.Value{Agg: "sum", Col: "Jumlah"}, X: "Tarikh", By: "day", Split: "Staf"},
			lab.WorkBlock{Kind: "bar", Label: "Ikut staf", Value: &lab.Value{Agg: "count"}, Group: "Staf", Top: 5},
			lab.WorkBlock{Kind: "donut", Label: "Dr Farah sahaja", Value: &lab.Value{Agg: "count"}, Group: "Staf", Filter: &lab.Filter{Col: "Staf", Vals: []string{"Dr Farah"}}},
			lab.WorkBlock{Kind: "bar", Label: "Ikut cawangan", Value: &lab.Value{Agg: "count"}, Group: "Cawangan"},
			lab.WorkBlock{Kind: "bar", Label: "Ikut jumlah", Value: &lab.Value{Agg: "count"}, Group: "Jumlah"},
			lab.WorkBlock{Kind: "table", Label: "Terkini", Cols: []string{"Tarikh", "Staf", "Jumlah"}, Sort: "Jumlah", Limit: 10, Wide: true},
			lab.WorkBlock{Kind: "table", Label: "Jadual A", Cols: []string{"Staf"}, Limit: 10},
			lab.WorkBlock{Kind: "table", Label: "Jadual B", Cols: []string{"Kos"}, Limit: 10},
		),
	}
	index, data, err := lab.GenerateWork(lab.TemplateByKey("general"), "Klinik", "", cfg, d, "data.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, b := range map[string][]byte{"index.html": index, "data.json": data} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got struct{ Cards, CardCols, Charts string }
	out := node(t, filepath.Join(dir, "index.html"), filepath.Join(dir, "data.json"))
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v in %s", err, out)
	}
	for _, want := range []string{`<article class="dpanel"><h2>Jadual A`, `<article class="dpanel"><h2>Jadual B`, "<span>C6</span>", "<span>Untung</span><strong>RM70.00</strong>", "<span>Purata</span><strong>RM60.00</strong>",
		"Ikut hari", "6 Jan", "Ikut staf", "(blank)", "Dr Farah sahaja</h2><p class=\"dnone\">No rows match</p>", "Terkini</h2><button type=\"button\" class=\"dcsv\" data-csv=\"11\">Download CSV</button>"} {
		if !strings.Contains(got.Cards+got.Charts, want) {
			t.Errorf("drawn Dashboard lacks %q:\n%s\n%s", want, got.Cards, got.Charts)
		}
	}
	// A card draws the icon it names, or with none named the one its value suggests; Purata names none.
	for _, want := range []string{`data-icon="star"`, `data-icon="people"`, `data-icon="chart-up"`} {
		if !strings.Contains(got.Cards, want) {
			t.Errorf("cards lack %s:\n%s", want, got.Cards)
		}
	}
	if !strings.Contains(got.Cards, `<div class="dcard"><span>Purata</span>`) {
		t.Errorf("Purata, naming no icon, draws one:\n%s", got.Cards)
	}
	if strings.Contains(got.Charts, "Ikut cawangan") || strings.Contains(got.Charts, "Ikut jumlah") {
		t.Errorf("a block needing Cawangan, absent from the Dataset, or grouping by the Number Jumlah, is drawn:\n%s", got.Charts)
	}
	if got.CardCols != "repeat(4,minmax(0,1fr))" {
		t.Errorf("8 cards sit in %q, want two rows of 4", got.CardCols)
	}
}

func TestADashboardDrawsInItsTheme(t *testing.T) {
	d := &lab.Dataset{
		Columns: []lab.Column{{Name: "Jabatan", Type: lab.Text}},
		Rows:    [][]any{{"IT"}, {"IT"}, {"Jualan"}},
	}
	cfg := &lab.Config{Blocks: []lab.WorkBlock{{Kind: "bar", Label: "Ikut jabatan", Value: &lab.Value{Agg: "count"}, Group: "Jabatan"}}}
	for _, c := range []struct{ theme, bg, top, rest string }{
		{"midnight", "--bg:#081726", "#D4AA55", "#3D7CC9"},
		{"minimal", "--bg:#FFFFFF", "#3569A8", "#3569A8"},
		// No Theme, or one Cave does not know, draws in Corporate Executive.
		{"", "--bg:#F6F8FB", "#C89B3C", "#0E2A47"},
		{"neon", "--bg:#F6F8FB", "#C89B3C", "#0E2A47"},
	} {
		index, data, err := lab.GenerateWork(lab.TemplateByKey("general"), "Staf", c.theme, cfg, d, "data.json")
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		for name, b := range map[string][]byte{"index.html": index, "data.json": data} {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var got struct{ Style, Charts string }
		out := node(t, filepath.Join(dir, "index.html"), filepath.Join(dir, "data.json"))
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%v in %s", err, out)
		}
		if !strings.Contains(got.Style, c.bg) {
			t.Errorf("Theme %q: style lacks %s", c.theme, c.bg)
		}
		// The largest group, IT, is in the Theme's highlight colour; the rest in its bar colour.
		if !strings.Contains(got.Charts, "background:"+c.top+`"></span></span><span class="dbar-v">2`) ||
			!strings.Contains(got.Charts, "background:"+c.rest+`"></span></span><span class="dbar-v">1`) {
			t.Errorf("Theme %q: bars not in %s and %s:\n%s", c.theme, c.top, c.rest, got.Charts)
		}
	}
}

func TestChartsDrawsEveryThemeCaveOffersAndNoOther(t *testing.T) {
	var keys []string
	for _, th := range lab.Themes {
		keys = append(keys, th.Key)
	}
	if out := strings.TrimSpace(node(t, "themes")); out != strings.Join(keys, " ") {
		t.Fatalf("charts.js draws Themes %q, Cave offers %q", out, keys)
	}
	if lab.Themes[0].Key != "corporate" {
		t.Fatalf("the first Theme, every Dashboard's until its Learner picks another, is %q", lab.Themes[0].Key)
	}
}

func TestChartsDrawsEveryIconACardMayName(t *testing.T) {
	if out := strings.TrimSpace(node(t, "icons")); out != strings.Join(lab.Icons, " ") {
		t.Fatalf("charts.js draws icons %q, a card may name %q", out, lab.Icons)
	}
}

func TestChartScriptCarriesInterSoADashboardLoadsNoFontFromAnotherSite(t *testing.T) {
	s := string(lab.ChartScript)
	for _, w := range []string{"400", "600"} {
		if !strings.Contains(s, "@font-face{font-family:Inter;font-weight:"+w+";font-display:swap;src:url(data:font/woff2;base64,d09GMg") {
			t.Errorf("charts.js lacks Inter at weight %s as a data URL", w)
		}
	}
	// The script as served, with the fonts written in, still parses.
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := filepath.Join(t.TempDir(), "charts.js")
	if err := os.WriteFile(f, lab.ChartScript, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("node", "--check", f).CombinedOutput(); err != nil {
		t.Fatalf("charts.js as served does not parse: %v\n%.300s", err, out)
	}
}
