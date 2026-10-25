package lab

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// Config is a Work Dashboard's own design, stored on the Dashboard: its blocks and filter bar, naming the
// Dataset's columns rather than Template blanks. It is configuration Cave applies, never code (ADR 0013).
type Config struct {
	Blocks    []WorkBlock `json:"blocks"`
	FilterBar []string    `json:"filterBar"`       // the Text and Date columns offered as filters
	Money     []string    `json:"money,omitempty"` // Number columns shown as RM
}

// WorkBlock is one block of a Work Dashboard, in the shape charts.js draws (spec #237).
type WorkBlock struct {
	Kind     string   `json:"kind"` // card, line, bar, heatmap, donut, table
	Label    string   `json:"label"`
	LabelSet bool     `json:"labelSet,omitempty"` // the Learner typed the label, so it no longer follows the choices
	Value    *Value   `json:"value,omitempty"`    // card, line, bar, donut
	X        string   `json:"x,omitempty"`        // line: the Date column
	By       string   `json:"by,omitempty"`       // line: day or month
	Split    string   `json:"split,omitempty"`    // line: one line per value of this column; heatmap: its columns across
	Group    string   `json:"group,omitempty"`    // bar, donut; heatmap: its rows down
	Top      int      `json:"top,omitempty"`      // bar, heatmap: the largest 5, 8 or 15 groups; 0 shows all
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
		w := WorkBlock{Kind: b.Kind, Label: b.Label, X: c[b.X], Split: c[b.Series], Group: c[b.Group], Cols: cols(b.Cols),
			Sort: c[b.Sort], Limit: b.Limit, Wide: b.Wide || b.Kind == "table"} // a Template's tables span the row
		if b.Kind != "table" {
			switch {
			case b.Times != "":
				w.Value = &Value{Agg: "calc", A: c[b.Of], Op: "x", B: c[b.Times], Then: "sum"}
			case b.Agg == "distinct":
				w.Value = &Value{Agg: "distinct", Col: c[b.Of]}
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

// MaxBlocks is the most blocks a Work Dashboard holds, and MaxCards the most of them that are cards (two rows).
const (
	MaxBlocks = 20
	MaxCards  = 8
)

// The options the editor offers, in its order.
var (
	Kinds  = []string{"card", "line", "bar", "heatmap", "donut", "table"}
	Aggs   = []string{"count", "sum", "avg", "distinct", "calc"}
	Ops    = [][2]string{{"+", "+"}, {"-", "−"}, {"x", "×"}, {"/", "÷"}} // a calculation's operators, as posted and as written
	Tops   = []int{5, 8, 15, 0}                                          // a bar's largest groups; 0 shows all
	Limits = []int{10, 25, 50}                                           // a table's rows
)

// opSign is operator op as people write it; empty for one Cave does not offer.
func opSign(op string) string {
	for _, o := range Ops {
		if o[0] == op {
			return o[1]
		}
	}
	return ""
}

// IsCard is whether the block is a card: cards sit in their own rows above the charts and tables.
func (b *WorkBlock) IsCard() bool { return b.Kind == "card" }

// Cards counts the Config's cards.
func (c *Config) Cards() int {
	n := 0
	for _, b := range c.Blocks {
		if b.IsCard() {
			n++
		}
	}
	return n
}

// Check refuses a Config over the limits: 20 blocks, 8 of them cards.
func (c *Config) Check() error {
	switch {
	case len(c.Blocks) > MaxBlocks:
		return Problem{"lab.err.blocks-full", []any{MaxBlocks}}
	case c.Cards() > MaxCards:
		return Problem{"lab.err.cards-full", []any{MaxCards}}
	}
	return nil
}

// Arrange puts the cards first, keeping the order within cards and within the rest: the Dashboard draws cards
// in their own rows on top, so the list shows them the same way. It returns where block i went.
func (c *Config) Arrange(i int) int {
	order := make([]int, len(c.Blocks))
	for k := range order {
		order[k] = k
	}
	rest := func(k int) int {
		if c.Blocks[k].IsCard() {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(order, func(a, b int) int { return rest(a) - rest(b) })
	blocks, at := make([]WorkBlock, len(order)), -1
	for k, from := range order {
		blocks[k] = c.Blocks[from]
		if from == i {
			at = k
		}
	}
	c.Blocks = blocks
	return at
}

// Add adds a new block of kind with its options at their defaults for d and returns where it went: a card after
// the last card, anything else at the end.
func (c *Config) Add(kind string, d *Dataset, lang string) (int, error) {
	b := WorkBlock{Kind: kind}
	if kind == "line" || kind == "table" || kind == "heatmap" {
		b.Wide = true
	}
	if kind == "bar" || kind == "heatmap" {
		b.Top = 8
	}
	b.Fill(d)
	b.Label = b.AutoLabel(lang)
	if err := b.Check(d); err != nil {
		return 0, err
	}
	c.Blocks = append(c.Blocks, b)
	at := c.Arrange(len(c.Blocks) - 1)
	return at, c.Check()
}

// Move moves block i one place up (step -1) or down (step 1) among its own sort, cards among cards and charts
// and tables among themselves; at the end of its sort it stays. It returns where the block is now.
func (c *Config) Move(i, step int) int {
	j := i + step
	if i < 0 || j < 0 || i >= len(c.Blocks) || j >= len(c.Blocks) || c.Blocks[i].IsCard() != c.Blocks[j].IsCard() {
		return i
	}
	c.Blocks[i], c.Blocks[j] = c.Blocks[j], c.Blocks[i]
	return j
}

// CheckFilterBar refuses a filter bar naming anything but the Dataset's Text and Date columns, or more than one
// Date column: the Dashboard draws one From and To.
func CheckFilterBar(names []string, d *Dataset) error {
	dates := 0
	for _, n := range names {
		switch d.typeOf(n) {
		case Date:
			if dates++; dates > 1 {
				return Problem{"lab.err.filter-bar-dates", nil}
			}
		case Text:
		default:
			return Problem{"lab.err.filter-bar", []any{n}}
		}
	}
	return nil
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

// first is the name of the Dataset's first column of type t (any type when t is empty) other than skip;
// empty if there is none.
func (d *Dataset) first(t Type, skip ...string) string {
	for _, c := range d.Columns {
		if (t == "" || c.Type == t) && !slices.Contains(skip, c.Name) {
			return c.Name
		}
	}
	return ""
}

// Fill gives each option the block's kind needs and the editor left empty, or set to a column of the wrong
// type, its default for d; options the kind does not use are cleared. A column the Dataset lacks, or one in
// paused, is left as it is, so the block can say what it needs.
func (b *WorkBlock) Fill(d *Dataset, paused ...string) {
	fit := func(col string, t Type) string {
		if have := d.typeOf(col); col == "" || (have != "" && t != "" && have != t && !slices.Contains(paused, col)) {
			return d.first(t)
		}
		return col
	}
	if b.Kind == "table" {
		b.Value, b.X, b.By, b.Split, b.Group, b.Top = nil, "", "", "", "", 0
		if len(b.Cols) == 0 {
			for _, c := range d.Columns[:min(3, len(d.Columns))] {
				b.Cols = append(b.Cols, c.Name)
			}
		}
		if b.Sort == "" {
			b.Sort = d.first(Date)
		}
		if !slices.Contains(Limits, b.Limit) {
			b.Limit = 10
		}
		return
	}
	b.Cols, b.Sort, b.Limit = nil, "", 0
	if b.Value == nil {
		b.Value = &Value{Agg: "count"}
	}
	v := b.Value
	switch v.Agg {
	case "sum", "avg":
		v.Col, v.A, v.Op, v.B, v.Then = fit(v.Col, Number), "", "", "", ""
	case "distinct":
		v.Col, v.A, v.Op, v.B, v.Then = fit(v.Col, ""), "", "", "", ""
	case "calc":
		v.Col, v.A = "", fit(v.A, Number)
		if have := d.typeOf(v.B); v.B == "" || (have != "" && have != Number && !slices.Contains(paused, v.B)) {
			if v.B = d.first(Number, v.A); v.B == "" {
				v.B = v.A
			}
		}
		if v.Op == "" {
			v.Op = "-"
		}
		if v.Then == "" {
			v.Then = "sum"
		}
	case "count":
		*v = Value{Agg: "count"}
	}
	if b.Kind != "line" {
		b.X, b.By = "", ""
	}
	if b.Kind != "bar" && b.Kind != "donut" && b.Kind != "heatmap" {
		b.Group = ""
	}
	if b.Kind != "line" && b.Kind != "bar" && b.Kind != "heatmap" {
		b.Split = ""
	}
	if b.Kind != "bar" && b.Kind != "heatmap" {
		b.Top = 0
	}
	switch b.Kind {
	case "card":
		b.Wide = false
	case "line":
		b.X = fit(b.X, Date)
		if b.By == "" {
			b.By = "month"
		}
	case "bar", "donut", "heatmap":
		b.Group = fit(b.Group, Text)
	}
	if have := d.typeOf(b.Split); have != "" && have != Text && !slices.Contains(paused, b.Split) {
		b.Split = ""
	}
	if b.Kind == "heatmap" && b.Split == "" { // a heatmap needs its columns across: another Text column
		b.Split = d.first(Text, b.Group)
	}
}

// Check refuses a block the editor cannot save on d: an option Cave does not offer, a label over 80
// characters, or a column the Dataset lacks or of the wrong type, a Locked block's own columns included. A paused
// block may keep naming the columns it was paused for.
func (b *WorkBlock) Check(d *Dataset, paused ...string) error {
	bad := Problem{"lab.err.block-option", nil}
	col := func(name string, t Type) error {
		have := d.typeOf(name)
		switch {
		case slices.Contains(paused, name):
			return nil
		case name == "" && d.first(t) == "":
			return Problem{"lab.err.block-none", []any{typeWord(t)}}
		case name == "":
			return bad
		case have == "":
			return Problem{"lab.err.block-column", []any{name}}
		case t != "" && have != t:
			return Problem{"lab.err.block-type", []any{typeWord(t), name, typeWord(have)}}
		}
		return nil
	}
	maybe := func(name string, t Type) error {
		if name == "" {
			return nil
		}
		return col(name, t)
	}
	if utf8.RuneCountInString(b.Label) > 80 {
		return Problem{"lab.err.block-label", nil}
	}
	var errs []error
	switch b.Kind {
	case "table":
		if len(b.Cols) == 0 {
			return Problem{"lab.err.block-cols", nil}
		}
		for _, c := range b.Cols {
			errs = append(errs, col(c, ""))
		}
		errs = append(errs, maybe(b.Sort, ""))
		if !slices.Contains(Limits, b.Limit) {
			return bad
		}
	case "card", "line", "bar", "heatmap", "donut":
		v := b.Value
		if v == nil {
			return bad
		}
		switch v.Agg {
		case "count":
		case "sum", "avg":
			errs = append(errs, col(v.Col, Number))
		case "distinct":
			errs = append(errs, col(v.Col, ""))
		case "calc":
			if opSign(v.Op) == "" || (v.Then != "sum" && v.Then != "avg") {
				return bad
			}
			errs = append(errs, col(v.A, Number), col(v.B, Number))
		default:
			return bad
		}
	default:
		return bad
	}
	switch b.Kind {
	case "line":
		if b.By != "day" && b.By != "month" {
			return bad
		}
		errs = append(errs, col(b.X, Date), maybe(b.Split, Text))
	case "bar":
		if !slices.Contains(Tops, b.Top) {
			return bad
		}
		errs = append(errs, col(b.Group, Text), maybe(b.Split, Text))
	case "heatmap":
		if !slices.Contains(Tops, b.Top) {
			return bad
		}
		if b.Split == "" || b.Split == b.Group {
			return Problem{"lab.err.block-heat-cols", nil}
		}
		errs = append(errs, col(b.Group, Text), col(b.Split, Text))
	case "donut":
		errs = append(errs, col(b.Group, Text))
	}
	if b.Filter != nil {
		errs = append(errs, col(b.Filter.Col, Text))
	}
	for _, p := range [][]string{b.Where, b.Minus} {
		if len(p) == 2 {
			errs = append(errs, col(p[0], Text))
		}
	}
	for _, c := range b.Low {
		errs = append(errs, col(c, Number))
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// KeepsLock refuses a change to a Locked block's kind or calculation, or a filter on it: of a block old locks,
// only the label, place, width and columns change. Its Template calculation filters rows itself, so a filter
// would reach only some of them.
func (b *WorkBlock) KeepsLock(old WorkBlock) error {
	calc := func(v *Value) Value {
		if v == nil {
			return Value{}
		}
		return Value{Agg: v.Agg, Op: v.Op, Then: v.Then}
	}
	if old.Locked && (b.Kind != old.Kind || calc(b.Value) != calc(old.Value) || b.Filter != nil) {
		return Problem{"lab.err.block-locked", nil}
	}
	return nil
}

// what is the block's value in a few words: Rows, Jumlah, Average Jumlah, Jumlah − Kos.
func (b *WorkBlock) what(lang string) string {
	v := b.Value
	switch {
	case v == nil || v.Agg == "count":
		return text.T(lang, "lab.b.lbl.count")
	case v.Agg == "calc":
		return v.A + " " + opSign(v.Op) + " " + v.B
	case v.Agg == "sum":
		return v.Col
	}
	return text.T(lang, "lab.b.lbl."+v.Agg, v.Col)
}

// AutoLabel is the label the block's choices give it, such as "Jumlah by Staf", until the Learner types one.
func (b *WorkBlock) AutoLabel(lang string) string {
	switch b.Kind {
	case "table":
		return text.T(lang, "lab.b.kind.table")
	case "line":
		return text.T(lang, "lab.b.by", b.what(lang), text.T(lang, "lab.b.by-"+b.By))
	case "bar", "donut":
		return text.T(lang, "lab.b.by", b.what(lang), b.Group)
	case "heatmap":
		return text.T(lang, "lab.b.by", b.what(lang), b.Group+" × "+b.Split)
	}
	return b.what(lang)
}

// Summary is the block's one line in the list: its value, what it groups by, and its filter.
func (b *WorkBlock) Summary(lang string) string {
	var parts []string
	switch v := b.Value; {
	case b.Kind == "table":
		parts = append(parts, strings.Join(b.Cols, ", "))
	case v == nil || v.Agg == "count":
		parts = append(parts, text.T(lang, "lab.b.agg.count"))
	case v.Agg == "calc":
		parts = append(parts, b.what(lang))
	default:
		parts = append(parts, text.T(lang, "lab.b.agg."+v.Agg)+" "+v.Col)
	}
	switch {
	case b.Kind == "line":
		parts = append(parts, text.T(lang, "lab.b.by", b.X, text.T(lang, "lab.b.by-"+b.By)))
	case b.Kind == "heatmap":
		parts = append(parts, b.Group+" × "+b.Split)
	case b.Group != "":
		parts = append(parts, b.Group)
	}
	if f := b.Filter; f != nil && len(f.Vals) > 0 {
		parts = append(parts, f.Col+" = "+strings.Join(f.Vals, ", "))
	}
	return strings.Join(parts, " · ")
}

// Use is one column a Work Dashboard's blocks name, with the type they need of it; empty when any type does.
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
				out = append(out, Use{n, t})
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

// Missing are the columns the block names that d lacks, or has of another type than the block needs. While any
// is missing the block is paused: left off the Dashboard and its Deployment, and back by itself once a Dataset
// has the column again.
func (b *WorkBlock) Missing(d *Dataset) []string {
	var out []string
	for _, u := range b.uses() {
		if !u.Fits(d) && !slices.Contains(out, u.Name) {
			out = append(out, u.Name)
		}
	}
	return out
}

// Gone are the block's filter values no cell of d holds, so the block shows no rows until they return.
func (b *WorkBlock) Gone(d *Dataset) []string {
	f := b.Filter
	if f == nil {
		return nil
	}
	i := slices.IndexFunc(d.Columns, func(c Column) bool { return c.Name == f.Col })
	if i < 0 {
		return nil
	}
	have := map[string]bool{}
	for _, r := range d.Rows {
		if s, ok := r[i].(string); ok {
			have[s] = true
		} else if r[i] == nil {
			have["(blank)"] = true // as the Dashboard groups empty cells
		}
	}
	var out []string
	for _, v := range f.Vals {
		if !have[v] {
			out = append(out, v)
		}
	}
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
