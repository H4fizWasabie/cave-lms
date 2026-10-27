package lab_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

var pageConfig = regexp.MustCompile(`(?s)const dashboard = (\{.*?\n    \});`)

func TestGenerateWritesTheBlocksAndFilterBarIntoThePage(t *testing.T) {
	d := clean(t, testdata(t, "finance.xlsx")).Dataset
	cfg := lab.Build(d)
	cfg.Money = []string{"Jumlah"} // as a Dashboard built on Finance, before the Templates went, keeps
	index, data, err := generate("Kedai <Ali>", "", cfg, d, "data/x")
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
	if !strings.Contains(string(index), "Kedai &lt;Ali&gt;") {
		t.Fatalf("page does not name the Dashboard:\n%s", index)
	}
	if !strings.HasPrefix(string(data), "[\n  {\"Tarikh\": ") {
		t.Fatalf("data.json = %.80s", data)
	}
}
