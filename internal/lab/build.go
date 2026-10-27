package lab

import (
	"cmp"
	"slices"
	"strings"
)

// Build is a built Dashboard's blocks and filter bar, by the General rules (ADR 0014, ADR 0019): Cave builds them
// from the Dataset's columns and their values, and the Learner fills nothing in.
func Build(d *Dataset) *Config {
	t, c := buildGeneral(d)
	return t.open(c)
}

// design is what the General rules make of a Dataset before it becomes a Config: blocks naming blanks, each blank a
// key the columns fill. A block is left out when a blank it needs is empty.
type design struct {
	Blocks  []Block
	Filters []string // blank keys: a Date blank filters by month range, a Text blank by value
}

// Blank is a hole that one column fills, guessed by its names.
type Blank struct {
	Key   string // what blocks refer to it by
	Type  Type
	Guess []string // column names Cave tries first, lower case, Malay and English
	// Loose, when set, are the names Cave's guessing looks for in column names instead of Guess,
	// lower case, best first.
	Loose []string
	// Not, with Loose, are words that keep a column from being guessed by part of its name, lower case.
	Not []string
}

// Block is one part of a design. Fields name blanks by key; the chart script (charts.js) draws it.
type Block struct {
	Kind   string // card, line, bar, donut, table
	Label  string
	Icon   string   // card: one of the icons charts.js draws
	Agg    string   // card: count or sum; distinct counts Of's values, avg averages them
	Of     string   // the Number blank summed; counts rows when empty
	X      string   // line: the Date blank, by month
	By     string   // line: day, else month
	Group  string   // bar and donut: the Text blank grouped by
	Series string   // line: one line per value of this blank, when chosen
	Times  string   // card: sum Of times this Number blank, row by row
	Cols   []string // table: blanks shown, in order; skipped ones are left out
	Sort   string   // table: newest first by this blank
	Limit  int      // table: rows shown
	Wide   bool     // spans the full width, as does a half-width block left without a half-width neighbour
}

// choices maps each filled blank's key to the column that fills it. An empty blank is absent.
type choices map[string]string

// open copies the design's blocks and filter bar for a Dashboard, each blank swapped for the column chosen for it;
// blocks that need an empty blank are left out.
func (t *design) open(c choices) *Config {
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
	for _, b := range t.included(c) {
		w := WorkBlock{Kind: b.Kind, Label: b.Label, Icon: b.Icon, X: c[b.X], Split: c[b.Series], Group: c[b.Group], Cols: cols(b.Cols),
			Sort: c[b.Sort], Limit: b.Limit, Wide: b.Wide || b.Kind == "table"} // tables span the row
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
		cfg.Blocks = append(cfg.Blocks, w)
	}
	return cfg
}

// included is the design's blocks drawn with choices c: a block needing an empty blank is left out.
func (t *design) included(c choices) []Block {
	var blocks []Block
	for _, b := range t.Blocks {
		ok := true
		for _, k := range []string{b.Of, b.X, b.Group} {
			ok = ok && (k == "" || c[k] != "")
		}
		if ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// guess is Cave's guess for each blank: an unused column of the blank's type whose name holds one of the blank's
// Loose names (else Guess names), trying the names in order and, for each, a column of exactly that name first;
// then, after Loose, a column named exactly as one of its Guess names. A column whose name holds one of the blank's
// Not words is never guessed by part of its name. A blank no column name matches is left empty: a guess by type
// alone is too often wrong on real files.
func guess(blanks []Blank, d *Dataset) choices {
	c, used := choices{}, map[string]bool{}
	for _, b := range blanks {
		names, whole := b.Loose, b.Guess
		if len(names) == 0 {
			names, whole = b.Guess, nil
		}
		pick := func(match func(name string) bool) bool {
			for _, col := range d.Columns {
				if name := columnKey(col.Name); !used[col.Name] && col.Type == b.Type && match(name) {
					c[b.Key], used[col.Name] = col.Name, true
					return true
				}
			}
			return false
		}
		part := func(g string) func(string) bool {
			return func(name string) bool {
				return strings.Contains(name, g) && !slices.ContainsFunc(b.Not, func(w string) bool { return strings.Contains(name, w) })
			}
		}
		is := func(g string) func(string) bool { return func(name string) bool { return name == g } }
		found := false
		for _, g := range names {
			if found = pick(is(g)) || pick(part(g)); found {
				break
			}
		}
		for _, g := range whole {
			if found = found || pick(is(g)); found {
				break
			}
		}
	}
	return c
}

// salesBlanks and stockBlanks are the Number blanks of the Sales and Inventory Templates, gone with the others
// (ADR 0019), by whose names General still finds a list's amount, quantity and price. Each guess keeps the blanks
// before it in their Template's order, as a column one fills is not guessed for the next.
var (
	salesBlanks = []Blank{
		{Key: "amount", Type: Number, Guess: []string{"jumlah", "amaun", "amount", "total", "jualan", "sales", "hasil", "revenue", "harga", "price"},
			Loose: []string{"jumlah", "amaun", "amount", "total", "jualan", "sales", "hasil", "revenue", "nilai", "value", "harga", "price"}},
		{Key: "quantity", Type: Number, Guess: []string{"kuantiti", "quantity", "qty", "unit"},
			Loose: []string{"kuantiti", "quantity", "qty", "bil. unit", "bilangan unit", "units sold"}},
	}
	stockBlanks = []Blank{
		// Stock on hand: a closing or actual count beats purchases, opening stock and movements.
		{Key: "quantity", Type: Number, Guess: []string{"kuantiti", "quantity", "qty", "stok", "baki"},
			Loose: []string{"closing", "actual", "baki akhir", "on hand", "in stock", "kuantiti", "quantity", "qty", "stok", "stock", "baki", "balance"},
			Not:   []string{"opening", "awal", "purchase", "belian", "movement", "pergerakan", "alert", "amaran"}},
		{Key: "reorder", Type: Number, Guess: []string{"paras pesanan semula", "paras minimum", "reorder level", "reorder", "minimum"},
			Loose: []string{"paras pesanan semula", "paras minimum", "reorder", "minimum", "par level", "safety stock", "stock alert", "amaran stok"}},
		{Key: "price", Type: Number, Guess: []string{"harga seunit", "harga", "unit price", "price"},
			Loose: []string{"harga seunit", "unit price", "harga", "price", "cost", "kos"}},
	}
)
