package lab

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestOpenFillsEachBlankWithItsColumnAndSetsWhatABlockKindNeeds(t *testing.T) {
	c := choices{"d": "Tarikh", "m": "Jumlah", "q": "Kuantiti", "p": "Harga", "g": "Cawangan", "s": "Jenis", "k": "Kod"}
	design := &design{
		Filters: []string{"d", "g", "none"},
		Blocks: []Block{
			{Kind: "card", Label: "Times", Of: "q", Times: "p", Agg: "avg"},
			{Kind: "card", Label: "Distinct", Of: "k", Agg: "distinct"},
			{Kind: "card", Label: "Average", Of: "m", Agg: "avg"},
			{Kind: "card", Label: "Sum", Of: "m", Agg: "sum"},
			{Kind: "card", Label: "Sum without an Agg", Of: "m"},
			{Kind: "card", Label: "Count", Agg: "count", Icon: "people"},
			{Kind: "line", Label: "Line", X: "d", Of: "m", Series: "s"},
			{Kind: "line", Label: "Line by day", X: "d", By: "day", Wide: true},
			{Kind: "bar", Label: "Bar", Group: "g", Of: "m"},
			{Kind: "donut", Label: "Donut", Group: "g", Of: "m"},
			{Kind: "table", Label: "Table", Cols: []string{"d", "none", "m"}, Sort: "d"},
			{Kind: "table", Label: "Table of 10", Limit: 10},
			{Kind: "card", Label: "Left out", Of: "none"},
		},
	}
	got := design.open(c)
	if want := []string{"Tarikh", "Cawangan"}; !reflect.DeepEqual(got.FilterBar, want) {
		t.Errorf("filter bar = %v, want %v", got.FilterBar, want)
	}
	want := []WorkBlock{
		{Kind: "card", Label: "Times", Value: &Value{Agg: "calc", A: "Kuantiti", Op: "x", B: "Harga", Then: "sum"}},
		{Kind: "card", Label: "Distinct", Value: &Value{Agg: "distinct", Col: "Kod"}},
		{Kind: "card", Label: "Average", Value: &Value{Agg: "avg", Col: "Jumlah"}},
		{Kind: "card", Label: "Sum", Value: &Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "card", Label: "Sum without an Agg", Value: &Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "card", Label: "Count", Icon: "people", Value: &Value{Agg: "count"}},
		{Kind: "line", Label: "Line", X: "Tarikh", By: "month", Split: "Jenis", Value: &Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "line", Label: "Line by day", X: "Tarikh", By: "day", Wide: true, Value: &Value{Agg: "count"}},
		{Kind: "bar", Label: "Bar", Group: "Cawangan", Top: 8, Value: &Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "donut", Label: "Donut", Group: "Cawangan", Value: &Value{Agg: "sum", Col: "Jumlah"}},
		{Kind: "table", Label: "Table", Cols: []string{"Tarikh", "Jumlah"}, Sort: "Tarikh", Limit: 50, Wide: true},
		{Kind: "table", Label: "Table of 10", Limit: 10, Wide: true},
	}
	asJSON := func(b []WorkBlock) []string {
		var out []string
		for _, w := range b {
			j, _ := json.Marshal(w)
			out = append(out, string(j))
		}
		return out
	}
	if g, w := asJSON(got.Blocks), asJSON(want); !slices.Equal(g, w) {
		t.Errorf("blocks =\n%s\nwant\n%s", strings.Join(g, "\n"), strings.Join(w, "\n"))
	}
}
