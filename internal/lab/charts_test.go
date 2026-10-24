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
	cfg := &lab.Config{
		Money:     []string{"Jumlah"},
		FilterBar: []string{"Staf", "Tarikh", "Cawangan"},
		Blocks: append(cards,
			lab.WorkBlock{Kind: "card", Label: "Untung", Value: &lab.Value{Agg: "calc", A: "Jumlah", Op: "-", B: "Kos", Then: "sum"}},
			lab.WorkBlock{Kind: "card", Label: "Purata", Value: &lab.Value{Agg: "avg", Col: "Jumlah"}},
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
	index, data, err := lab.GenerateWork(lab.TemplateByKey("general"), "Klinik", cfg, d, "data.json")
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
		"Ikut hari", "6 Jan", "Ikut staf", "(blank)", "Dr Farah sahaja</h2><p class=\"dnone\">No rows match</p>", "Terkini"} {
		if !strings.Contains(got.Cards+got.Charts, want) {
			t.Errorf("drawn Dashboard lacks %q:\n%s\n%s", want, got.Cards, got.Charts)
		}
	}
	if strings.Contains(got.Charts, "Ikut cawangan") || strings.Contains(got.Charts, "Ikut jumlah") {
		t.Errorf("a block needing Cawangan, absent from the Dataset, or grouping by the Number Jumlah, is drawn:\n%s", got.Charts)
	}
	if got.CardCols != "repeat(4,minmax(0,1fr))" {
		t.Errorf("8 cards sit in %q, want two rows of 4", got.CardCols)
	}
}
