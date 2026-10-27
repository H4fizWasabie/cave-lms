package lab

import (
	"cmp"
	"slices"
)

// Config is a Dashboard's own design, stored on the Dashboard: its blocks and filter bar, naming the
// Dataset's columns rather than Template blanks. It is configuration Cave applies, never code (ADR 0013).
type Config struct {
	Blocks    []WorkBlock `json:"blocks"`
	FilterBar []string    `json:"filterBar"`       // the Text and Date columns offered as filters
	Money     []string    `json:"money,omitempty"` // Number columns shown as RM
}

// WorkBlock is one block of a Dashboard, in the shape charts.js draws (spec #237).
type WorkBlock struct {
	Kind     string `json:"kind"` // card, line, bar, heatmap, donut, table
	Label    string `json:"label"`
	LabelSet bool   `json:"labelSet,omitempty"` // the Learner typed the label, so it no longer follows the choices
	Icon     string `json:"icon,omitempty"`     // card: one of Icons, or none; empty draws the one its value suggests
	Value    *Value `json:"value,omitempty"`    // card, line, bar, donut
	X        string `json:"x,omitempty"`        // line: the Date column
	By       string `json:"by,omitempty"`       // line: day or month
	Split    string `json:"split,omitempty"`    // line: one line per value of this column; heatmap: its columns across
	Group    string `json:"group,omitempty"`    // bar, donut; heatmap: its rows down
	Top      int    `json:"top,omitempty"`      // bar, heatmap: the largest 5, 8 or 15 groups; 0 shows all
	// Slices is a donut's: the largest 5, 6 or 8 groups, the rest folded into Other, or 0 for all; none chosen is 6.
	Slices *int     `json:"slices,omitempty"`
	Dir    string   `json:"dir,omitempty"`    // bar: vertical, or empty for horizontal
	Cols   []string `json:"cols,omitempty"`   // table: columns shown, in order
	Sort   string   `json:"sort,omitempty"`   // table: newest or largest first by this column
	Limit  int      `json:"limit,omitempty"`  // table: 10, 25 or 50 rows
	Filter *Filter  `json:"filter,omitempty"` // only rows where a column holds one of some values
	Wide   bool     `json:"wide,omitempty"`
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

// Open copies the Template's blocks and filter bar for a Dashboard, each blank swapped for the column
// chosen for it; blocks that need a skipped blank are left out. General builds
// its blocks from the Dataset. A choice naming a column d lacks, or holding another type, counts as skipped, so
// Reset to Template fills the blanks from the current columns by name.
func (t *Template) Open(c Choices, d *Dataset) *Config {
	if t.Build != nil {
		t, c = t.Build(d)
	}
	kept := Choices{}
	for _, b := range t.Blanks {
		if col := c[b.Key]; col != "" && d.typeOf(col) == b.Type {
			kept[b.Key] = col
		}
	}
	c = kept
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
		w := WorkBlock{Kind: b.Kind, Label: b.Label, Icon: b.Icon, X: c[b.X], Split: c[b.Series], Group: c[b.Group], Cols: cols(b.Cols),
			Sort: c[b.Sort], Limit: b.Limit, Wide: b.Wide || b.Kind == "table"} // a Template's tables span the row
		if b.Kind != "table" {
			switch {
			case b.Times != "":
				w.Value = &Value{Agg: "calc", A: c[b.Of], Op: "x", B: c[b.Times], Then: "sum"}
			case b.Agg == "distinct" || b.Agg == "avg":
				w.Value = &Value{Agg: b.Agg, Col: c[b.Of]}
			case b.Of != "":
				w.Value = &Value{Agg: "sum", Col: c[b.Of]}
			default:
				w.Value = &Value{Agg: "count"}
			}
		}
		if b.Kind == "line" {
			w.By = cmp.Or(b.By, "month")
		}
		if b.Kind == "bar" {
			w.Top = 8
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

// typeOf is the type of the Dataset's column called name; empty if it has none.
func (d *Dataset) typeOf(name string) Type {
	for _, c := range d.Columns {
		if c.Name == name {
			return c.Type
		}
	}
	return ""
}

// Use is one column a Dashboard's blocks name, with the type they need of it; empty when any type does.
type Use struct {
	Name string
	Type Type
}

// Fits is whether d has the column, of the type the blocks need.
func (u Use) Fits(d *Dataset) bool {
	have := d.typeOf(u.Name)
	return have != "" && (u.Type == "" || u.Type == have)
}

// Options are d's columns that may stand in for the column: those of the type the blocks need.
func (u Use) Options(d *Dataset) []string {
	var out []string
	for _, c := range d.Columns {
		if u.Type == "" || c.Type == u.Type {
			out = append(out, c.Name)
		}
	}
	return out
}

// uses are the columns the block names, with the type each needs.
func (b *WorkBlock) uses() []Use {
	var out []Use
	add := func(t Type, names ...string) {
		for _, n := range names {
			if n != "" {
				out = append(out, Use{Name: n, Type: t})
			}
		}
	}
	if v := b.Value; v != nil {
		if v.Agg == "distinct" {
			add("", v.Col)
		} else {
			add(Number, v.Col, v.A, v.B)
		}
	}
	add(Date, b.X)
	add(Text, b.Group, b.Split)
	add("", b.Cols...)
	add("", b.Sort)
	if b.Filter != nil {
		add(Text, b.Filter.Col)
	}
	if len(b.Where) > 0 {
		add(Text, b.Where[0])
	}
	if len(b.Minus) > 0 {
		add(Text, b.Minus[0])
	}
	add(Number, b.Low...)
	return out
}

// Uses are the columns the Config's blocks name, each once, needing the type any block needs of it.
func (c *Config) Uses() []Use {
	var out []Use
	for _, b := range c.Blocks {
		for _, u := range b.uses() {
			i := slices.IndexFunc(out, func(o Use) bool { return o.Name == u.Name })
			switch {
			case i < 0:
				out = append(out, u)
			case out[i].Type == "":
				out[i].Type = u.Type
			}
		}
	}
	return out
}

// Rename is c with column m[from] in place of column from, as Config.Rename, so Reset to Template fills the
// blanks from the columns the blocks were moved to.
func (c Choices) Rename(m map[string]string) Choices {
	out := Choices{}
	for k, v := range c {
		out[k] = cmp.Or(m[v], v)
	}
	return out
}

// Rename puts column m[from] in place of column from, for every from in m, in every block, the filter bar and
// the money columns. The pairs swap at once, so a rename never feeds another.
func (c *Config) Rename(m map[string]string) {
	one := func(s *string) {
		if to, ok := m[*s]; ok && *s != "" {
			*s = to
		}
	}
	all := func(s []string) []string {
		s = slices.Clone(s)
		for i := range s {
			one(&s[i])
		}
		return s
	}
	pair := func(p []string) []string { // [column, value]
		if len(p) == 0 {
			return p
		}
		p = slices.Clone(p)
		one(&p[0])
		return p
	}
	c.Blocks = slices.Clone(c.Blocks)
	for i := range c.Blocks {
		b := &c.Blocks[i]
		if b.Value != nil {
			v := *b.Value
			one(&v.Col)
			one(&v.A)
			one(&v.B)
			b.Value = &v
		}
		one(&b.X)
		one(&b.Split)
		one(&b.Group)
		one(&b.Sort)
		if b.Filter != nil {
			b.Filter = &Filter{Col: b.Filter.Col, Vals: b.Filter.Vals}
			one(&b.Filter.Col)
		}
		b.Cols, b.Low = all(b.Cols), all(b.Low)
		b.Where, b.Minus = pair(b.Where), pair(b.Minus)
	}
	c.FilterBar, c.Money = all(c.FilterBar), all(c.Money)
}
