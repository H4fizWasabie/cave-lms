package lab

// Config is a Work Dashboard's own design, stored on the Dashboard: its blocks and filter bar, naming the
// Dataset's columns rather than Template blanks. It is configuration Cave applies, never code (ADR 0013).
type Config struct {
	Blocks    []WorkBlock `json:"blocks"`
	FilterBar []string    `json:"filterBar"`       // the Text and Date columns offered as filters
	Money     []string    `json:"money,omitempty"` // Number columns shown as RM
}

// WorkBlock is one block of a Work Dashboard, in the shape charts.js draws (spec #237).
type WorkBlock struct {
	Kind     string   `json:"kind"` // card, line, bar, donut, table
	Label    string   `json:"label"`
	LabelSet bool     `json:"labelSet,omitempty"` // the Learner typed the label, so it no longer follows the choices
	Value    *Value   `json:"value,omitempty"`    // card, line, bar, donut
	X        string   `json:"x,omitempty"`        // line: the Date column
	By       string   `json:"by,omitempty"`       // line: day or month
	Split    string   `json:"split,omitempty"`    // line: one line per value of this column
	Group    string   `json:"group,omitempty"`    // bar, donut
	Cols     []string `json:"cols,omitempty"`     // table: columns shown, in order
	Sort     string   `json:"sort,omitempty"`     // table: newest or largest first by this column
	Limit    int      `json:"limit,omitempty"`    // table: 10, 25 or 50 rows
	Filter   *Filter  `json:"filter,omitempty"`   // only rows where a column holds one of some values
	Wide     bool     `json:"wide,omitempty"`
	// Locked marks a Template block the editor cannot express; it keeps its Template calculation below,
	// naming columns as Block names blanks.
	Locked bool     `json:"locked,omitempty"`
	Where  []string `json:"where,omitempty"`
	Minus  []string `json:"minus,omitempty"`
	Low    []string `json:"low,omitempty"`
}

// Value is what a block counts or adds up: count rows; sum, avg or distinct of Col; or calc, A Op B row by
// row, then summed or averaged.
type Value struct {
	Agg  string `json:"agg"`
	Col  string `json:"col,omitempty"`
	A    string `json:"a,omitempty"`
	Op   string `json:"op,omitempty"` // + - x /
	B    string `json:"b,omitempty"`
	Then string `json:"then,omitempty"`
}

// Filter keeps only the rows where Col holds one of Vals.
type Filter struct {
	Col  string   `json:"col"`
	Vals []string `json:"vals"`
}

// Open copies the Template's blocks and filter bar for a Work Dashboard, each blank swapped for the column
// chosen for it; blocks that need a skipped blank are left out, as in a Practice Dashboard. General builds
// its blocks from the Dataset.
func (t *Template) Open(c Choices, d *Dataset) *Config {
	if t.Build != nil {
		t, c = t.Build(d)
	}
	cols := func(keys []string) []string {
		var out []string
		for _, k := range keys {
			if c[k] != "" {
				out = append(out, c[k])
			}
		}
		return out
	}
	cfg := &Config{FilterBar: cols(t.Filters), Blocks: []WorkBlock{}}
	for _, b := range t.Blanks {
		if b.Money && c[b.Key] != "" {
			cfg.Money = append(cfg.Money, c[b.Key])
		}
	}
	for _, b := range t.included(c) {
		w := WorkBlock{Kind: b.Kind, Label: b.Label, X: c[b.X], Split: c[b.Series], Group: c[b.Group], Cols: cols(b.Cols),
			Sort: c[b.Sort], Limit: b.Limit, Wide: b.Wide}
		if b.Kind != "table" {
			switch {
			case b.Times != "":
				w.Value = &Value{Agg: "calc", A: c[b.Of], Op: "x", B: c[b.Times], Then: "sum"}
			case b.Of != "":
				w.Value = &Value{Agg: "sum", Col: c[b.Of]}
			default:
				w.Value = &Value{Agg: "count"}
			}
		}
		if b.Kind == "line" {
			w.By = "month"
		}
		if b.Kind == "table" && w.Limit == 0 {
			w.Limit = 50
		}
		switch {
		case b.Minus != nil || b.Low != nil:
			w.Locked, w.Where, w.Minus, w.Low = true, columnPair(c, b.Where), columnPair(c, b.Minus), cols(b.Low)
		case b.Where != nil && c[b.Where[0]] != "":
			w.Filter = &Filter{Col: c[b.Where[0]], Vals: []string{b.Where[1]}}
		}
		cfg.Blocks = append(cfg.Blocks, w)
	}
	return cfg
}

// columnPair is [blank, value] as [column, value], or nil when the blank is skipped.
func columnPair(c Choices, p []string) []string {
	if p == nil || c[p[0]] == "" {
		return nil
	}
	return []string{c[p[0]], p[1]}
}

// included is the Template's blocks drawn with choices c: a block needing a skipped blank is left out, as is
// one whose Unless blank is chosen.
func (t *Template) included(c Choices) []Block {
	var blocks []Block
	for _, b := range t.Blocks {
		if b.Unless != "" && c[b.Unless] != "" {
			continue
		}
		needs := append([]string{b.Of, b.X, b.Group}, b.Needs...)
		ok := true
		for _, k := range needs {
			ok = ok && (k == "" || c[k] != "")
		}
		if ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}
