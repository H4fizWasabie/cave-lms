package lab

import (
	"slices"
)

// Config is a built Dashboard's blocks and filter bar, stored on the Dashboard, naming the Dataset's columns. It is configuration Cave applies, never code (ADR 0013).
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
	// Locked marks a block from a Template (gone, ADR 0019) that keeps its Template calculation below, naming
	// columns; a Dashboard built on one still draws it.
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
