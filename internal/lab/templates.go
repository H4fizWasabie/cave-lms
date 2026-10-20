package lab

import (
	_ "embed"
	"strings"
)

// Template is a Dashboard design: blanks the Learner fills with columns, and blocks drawn from them.
// A block is left out when a blank it needs is skipped. To add a Template, define it below with its
// Sample embedded from samples/ and list it in Templates.
type Template struct {
	Key     string // in URLs and stored on each Dashboard; never changes
	Name    string
	Blurb   string
	Blanks  []Blank
	Blocks  []Block
	Filters []string // blank keys: a Date blank filters by month range, a Text blank by value
	Sample  []byte   // the .xlsx Learners download
}

// Blank is a hole in a Template that one column fills.
type Blank struct {
	Key      string // what blocks refer to it by
	Label    string
	Type     Type
	Required bool
	Money    bool     // a Number shown as RM
	Help     string   // what goes in it, for the Learner
	Guess    []string // column names Cave tries first, lower case, Malay and English
}

// Block is one part of a Dashboard. Fields name blanks by key; the chart script (charts.js) draws it.
type Block struct {
	Kind   string   `json:"kind"` // card, line, bar, donut, table
	Label  string   `json:"label"`
	Agg    string   `json:"agg,omitempty"`    // card: count or sum
	Of     string   `json:"of,omitempty"`     // the Number blank summed; counts rows when empty
	X      string   `json:"x,omitempty"`      // line: the Date blank, by month
	Group  string   `json:"group,omitempty"`  // bar and donut: the Text blank grouped by
	Series string   `json:"series,omitempty"` // line: one line per value of this blank, when chosen
	Where  []string `json:"where,omitempty"`  // [blank, value]: only rows where the blank holds value, when chosen
	Minus  []string `json:"minus,omitempty"`  // card: subtract the sum over rows where [blank, value]
	Cols   []string `json:"cols,omitempty"`   // table: blanks shown, in order; skipped ones are left out
	Sort   string   `json:"sort,omitempty"`   // table: newest first by this blank
	Limit  int      `json:"limit,omitempty"`  // table: rows shown
	Wide   bool     `json:"wide,omitempty"`   // spans the full width
	Needs  []string `json:"-"`                // blanks the block needs beyond Of, X and Group
	Unless string   `json:"-"`                // left out when this blank is chosen
}

//go:embed samples/finance.xlsx
var financeSample []byte

var Finance = &Template{
	Key:   "finance",
	Name:  "Finance",
	Blurb: "Money in and out: totals, balance, spending by month and category.",
	Blanks: []Blank{
		{Key: "date", Label: "Date", Type: Date, Required: true, Help: "The day of each transaction", Guess: []string{"tarikh", "date"}},
		{Key: "amount", Label: "Amount", Type: Number, Required: true, Money: true, Help: "How much, in RM", Guess: []string{"jumlah", "amaun", "amount", "total"}},
		{Key: "category", Label: "Category", Type: Text, Help: "What the money was for", Guess: []string{"kategori", "category"}},
		{Key: "type", Label: "Type", Type: Text, Help: "Masuk (money in) or Keluar (money out)", Guess: []string{"jenis", "type", "masuk/keluar"}},
		{Key: "description", Label: "Description", Type: Text, Help: "A short note on each transaction", Guess: []string{"perkara", "butiran", "keterangan", "description", "item"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Money in", Agg: "sum", Of: "amount", Where: []string{"type", "Masuk"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Money out", Agg: "sum", Of: "amount", Where: []string{"type", "Keluar"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Balance", Agg: "sum", Of: "amount", Where: []string{"type", "Masuk"}, Minus: []string{"type", "Keluar"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Total", Agg: "sum", Of: "amount", Unless: "type"},
		{Kind: "card", Label: "Transactions", Agg: "count"},
		{Kind: "line", Label: "By month", X: "date", Of: "amount", Series: "type", Wide: true},
		{Kind: "bar", Label: "Spending by category", Group: "category", Of: "amount", Where: []string{"type", "Keluar"}},
		{Kind: "donut", Label: "Money in by source", Group: "category", Of: "amount", Where: []string{"type", "Masuk"}, Needs: []string{"type"}},
		{Kind: "table", Label: "Latest transactions", Cols: []string{"date", "description", "category", "type", "amount"}, Sort: "date", Limit: 10},
	},
	Filters: []string{"category", "type", "date"},
	Sample:  financeSample,
}

// Templates are every Template, in the order the Lab offers them.
var Templates = []*Template{Finance}

// TemplateByKey finds a Template by its Key, or nil.
func TemplateByKey(key string) *Template {
	for _, t := range Templates {
		if t.Key == key {
			return t
		}
	}
	return nil
}

// Choices maps each filled blank's key to the column that fills it. A skipped optional blank is absent.
type Choices map[string]string

// Guess is Cave's pre-filled choice for each blank: an unused column of the blank's type whose name
// matches one of its guesses, else, for a required blank, the first unused column of its type.
func (t *Template) Guess(d *Dataset) Choices {
	c, used := Choices{}, map[string]bool{}
	pick := func(b Blank, match func(Column) bool) bool {
		for _, col := range d.Columns {
			if !used[col.Name] && col.Type == b.Type && match(col) {
				c[b.Key], used[col.Name] = col.Name, true
				return true
			}
		}
		return false
	}
	for _, b := range t.Blanks {
		named := false
		for _, g := range b.Guess {
			if named = pick(b, func(col Column) bool { return strings.EqualFold(col.Name, g) }); named {
				break
			}
		}
		if !named && b.Required {
			pick(b, func(Column) bool { return true })
		}
	}
	return c
}
