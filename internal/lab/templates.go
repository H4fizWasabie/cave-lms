package lab

import (
	_ "embed"
	"fmt"
	"strings"
)

// Template is a Dashboard design: blanks the Learner fills with columns, and blocks drawn from them.
// A block is left out when a blank it needs is skipped; the first card left is drawn largest. To add
// a Template, define it below with its Sample embedded from samples/ and list it in workTemplates, and in
// Templates too if Practice Dashboards offer it.
type Template struct {
	Key      string // in URLs and stored on each Dashboard; never changes
	Name     string
	Blurb    string
	Question string // the Dashboard question, shown under its name; General has none
	Blanks   []Blank
	Blocks   []Block
	Filters  []string // blank keys: a Date blank filters by month range, a Text blank by value
	Sample   []byte   // the .xlsx Learners download
	// Build, when set, makes the blanks, blocks and choices from the Dataset itself (General has no blanks).
	Build func(*Dataset) (*Template, Choices)
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
	Agg    string   `json:"agg,omitempty"`    // card: count or sum; distinct counts Of's values (Work Templates only)
	Of     string   `json:"of,omitempty"`     // the Number blank summed; counts rows when empty
	X      string   `json:"x,omitempty"`      // line: the Date blank, by month
	By     string   `json:"by,omitempty"`     // line: day, else month (Work Templates only)
	Group  string   `json:"group,omitempty"`  // bar and donut: the Text blank grouped by
	Series string   `json:"series,omitempty"` // line: one line per value of this blank, when chosen
	Where  []string `json:"where,omitempty"`  // [blank, value]: only rows where the blank holds value, when chosen
	Minus  []string `json:"minus,omitempty"`  // card: subtract the sum over rows where [blank, value]
	Times  string   `json:"times,omitempty"`  // card: sum Of times this Number blank, row by row
	Low    []string `json:"low,omitempty"`    // [a, b]: only rows where blank a is at or below blank b
	Cols   []string `json:"cols,omitempty"`   // table: blanks shown, in order; skipped ones are left out
	Sort   string   `json:"sort,omitempty"`   // table: newest first by this blank
	Limit  int      `json:"limit,omitempty"`  // table: rows shown
	Wide   bool     `json:"wide,omitempty"`   // spans the full width, as does a half-width block left without a half-width neighbour
	Needs  []string `json:"-"`                // blanks the block needs beyond Of, X and Group
	Unless string   `json:"-"`                // left out when this blank is chosen
}

//go:embed samples/finance.xlsx
var financeSample []byte

var Finance = &Template{
	Key:      "finance",
	Name:     "Finance",
	Blurb:    "Money in and out: totals, balance, spending by month and category.",
	Question: "Am I spending more than I earn?",
	Blanks: []Blank{
		{Key: "date", Label: "Date", Type: Date, Required: true, Help: "The day of each transaction", Guess: []string{"tarikh", "date"}},
		{Key: "amount", Label: "Amount", Type: Number, Required: true, Money: true, Help: "How much, in RM", Guess: []string{"jumlah", "amaun", "amount", "total"}},
		{Key: "category", Label: "Category", Type: Text, Help: "What the money was for", Guess: []string{"kategori", "category"}},
		{Key: "type", Label: "Type", Type: Text, Help: "Masuk (money in) or Keluar (money out)", Guess: []string{"jenis", "type", "masuk/keluar"}},
		{Key: "description", Label: "Description", Type: Text, Help: "A short note on each transaction", Guess: []string{"perkara", "butiran", "keterangan", "description", "item"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Balance", Agg: "sum", Of: "amount", Where: []string{"type", "Masuk"}, Minus: []string{"type", "Keluar"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Total", Agg: "sum", Of: "amount", Unless: "type"},
		{Kind: "card", Label: "Money in", Agg: "sum", Of: "amount", Where: []string{"type", "Masuk"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Money out", Agg: "sum", Of: "amount", Where: []string{"type", "Keluar"}, Needs: []string{"type"}},
		{Kind: "card", Label: "Transactions", Agg: "count"},
		{Kind: "line", Label: "By month", X: "date", Of: "amount", Series: "type", Wide: true},
		{Kind: "bar", Label: "Spending by category", Group: "category", Of: "amount", Where: []string{"type", "Keluar"}},
		{Kind: "donut", Label: "Money in by source", Group: "category", Of: "amount", Where: []string{"type", "Masuk"}, Needs: []string{"type"}},
		{Kind: "table", Label: "Latest transactions", Cols: []string{"date", "description", "category", "type", "amount"}, Sort: "date", Limit: 10},
	},
	Filters: []string{"category", "type", "date"},
	Sample:  financeSample,
}

//go:embed samples/inventory.xlsx
var inventorySample []byte

var Inventory = &Template{
	Key:      "inventory",
	Name:     "Inventory",
	Blurb:    "Stock on hand: items, value, and what is running low.",
	Question: "What do I need to reorder?",
	Blanks: []Blank{
		{Key: "item", Label: "Item", Type: Text, Required: true, Help: "The name of each item", Guess: []string{"nama item", "nama barang", "item", "barang", "nama"}},
		{Key: "quantity", Label: "Quantity", Type: Number, Required: true, Help: "How many are in stock", Guess: []string{"kuantiti", "quantity", "qty", "stok", "baki"}},
		{Key: "reorder", Label: "Reorder level", Type: Number, Help: "Order more when stock falls to this", Guess: []string{"paras pesanan semula", "paras minimum", "reorder level", "reorder", "minimum"}},
		{Key: "category", Label: "Category", Type: Text, Help: "The kind of item", Guess: []string{"kategori", "category"}},
		{Key: "price", Label: "Unit price", Type: Number, Money: true, Help: "What one costs, in RM", Guess: []string{"harga seunit", "harga", "unit price", "price"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Stock value", Agg: "sum", Of: "quantity", Times: "price", Needs: []string{"price"}},
		{Kind: "card", Label: "Total items", Agg: "sum", Of: "quantity"},
		{Kind: "card", Label: "Low stock", Agg: "count", Low: []string{"quantity", "reorder"}, Needs: []string{"reorder"}},
		{Kind: "bar", Label: "Stock by category", Group: "category", Of: "quantity"},
		{Kind: "table", Label: "Running low", Cols: []string{"item", "category", "quantity", "reorder"}, Low: []string{"quantity", "reorder"}, Needs: []string{"reorder"}},
		{Kind: "table", Label: "All items", Cols: []string{"item", "category", "quantity", "reorder", "price"}},
	},
	Filters: []string{"category"},
	Sample:  inventorySample,
}

//go:embed samples/hr.xlsx
var hrSample []byte

var HR = &Template{
	Key:      "hr",
	Name:     "HR",
	Blurb:    "Staff: headcount, departments, who joined when.",
	Question: "Is the team growing, and where?",
	Blanks: []Blank{
		{Key: "name", Label: "Name", Type: Text, Required: true, Help: "Each staff member's name", Guess: []string{"nama", "nama pekerja", "name"}},
		{Key: "department", Label: "Department", Type: Text, Required: true, Help: "Where they work", Guess: []string{"jabatan", "bahagian", "department"}},
		{Key: "joined", Label: "Join date", Type: Date, Help: "The day they started", Guess: []string{"tarikh mula", "tarikh masuk", "tarikh lantikan", "join date", "start date", "tarikh"}},
		{Key: "status", Label: "Status", Type: Text, Help: "Aktif for staff still working", Guess: []string{"status"}},
		{Key: "gender", Label: "Gender", Type: Text, Help: "Lelaki or Perempuan", Guess: []string{"jantina", "gender"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Headcount", Agg: "count"},
		{Kind: "card", Label: "Active", Agg: "count", Where: []string{"status", "Aktif"}, Needs: []string{"status"}},
		{Kind: "bar", Label: "Headcount by department", Group: "department"},
		{Kind: "donut", Label: "Status split", Group: "status"},
		{Kind: "line", Label: "Joins per month", X: "joined", Wide: true},
		{Kind: "table", Label: "Staff", Cols: []string{"name", "department", "joined", "status", "gender"}, Sort: "joined"},
	},
	Filters: []string{"department", "status", "gender", "joined"},
	Sample:  hrSample,
}

//go:embed samples/general.xlsx
var generalSample []byte

var General = &Template{
	Key:    "general",
	Name:   "General",
	Blurb:  "Any table. No blanks to fill: Cave builds the charts from your column types.",
	Build:  buildGeneral,
	Sample: generalSample,
}

// buildGeneral makes one blank per column, filled by that column: a card per Number column, a bar
// chart for the first Text column, a line chart by month for the first Date column, and a table.
func buildGeneral(d *Dataset) (*Template, Choices) {
	t, c := &Template{}, Choices{}
	first := map[Type]string{}
	var cols []string
	for i, col := range d.Columns {
		key := fmt.Sprintf("c%d", i+1)
		t.Blanks = append(t.Blanks, Blank{Key: key, Label: col.Name, Type: col.Type})
		c[key] = col.Name
		cols = append(cols, key)
		if first[col.Type] == "" {
			first[col.Type] = key
		}
		if col.Type == Number {
			t.Blocks = append(t.Blocks, Block{Kind: "card", Label: col.Name, Agg: "sum", Of: key})
		}
	}
	measure := "Rows"
	if k := first[Number]; k != "" {
		measure = c[k]
	}
	if k := first[Text]; k != "" {
		t.Blocks = append(t.Blocks, Block{Kind: "bar", Label: measure + " by " + c[k], Group: k, Of: first[Number]})
		t.Filters = append(t.Filters, k)
	}
	table := Block{Kind: "table", Label: "Rows", Cols: cols, Limit: 10}
	if k := first[Date]; k != "" {
		t.Blocks = append(t.Blocks, Block{Kind: "line", Label: measure + " by month", X: k, Of: first[Number], Wide: true})
		t.Filters = append(t.Filters, k)
		table.Label, table.Sort = "Latest rows", k
	}
	t.Blocks = append(t.Blocks, table)
	return t, c
}

//go:embed samples/sales.xlsx
var salesSample []byte

var Sales = &Template{
	Key:      "sales",
	Name:     "Sales",
	Blurb:    "Takings: revenue by month, top items, staff and payment methods.",
	Question: "What sells, and when?",
	Blanks: []Blank{
		{Key: "date", Label: "Date", Type: Date, Required: true, Help: "The day of each sale", Guess: []string{"tarikh", "date", "tarikh jualan", "sale date"}},
		{Key: "amount", Label: "Amount", Type: Number, Required: true, Money: true, Help: "What the sale took, in RM", Guess: []string{"jumlah", "amaun", "amount", "total", "jualan", "sales", "hasil", "revenue", "harga", "price"}},
		{Key: "item", Label: "Item", Type: Text, Help: "What was sold", Guess: []string{"produk", "product", "item", "barang", "nama item", "nama barang", "perkhidmatan", "service"}},
		{Key: "category", Label: "Category", Type: Text, Help: "The kind of item", Guess: []string{"kategori", "category"}},
		{Key: "staff", Label: "Staff", Type: Text, Help: "Who made the sale", Guess: []string{"staf", "staff", "pekerja", "juruwang", "cashier", "doktor", "doctor"}},
		{Key: "payment", Label: "Payment method", Type: Text, Help: "Tunai, Kad, DuitNow QR and so on", Guess: []string{"kaedah bayaran", "bayaran", "payment method", "payment mode", "payment"}},
		{Key: "quantity", Label: "Quantity", Type: Number, Help: "How many were sold", Guess: []string{"kuantiti", "quantity", "qty", "unit"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Revenue", Agg: "sum", Of: "amount"},
		{Kind: "card", Label: "Sales", Agg: "count"},
		{Kind: "card", Label: "Units", Agg: "sum", Of: "quantity"},
		{Kind: "line", Label: "Revenue by month", X: "date", Of: "amount", Series: "category", Wide: true},
		{Kind: "bar", Label: "Revenue by item", Group: "item", Of: "amount"},
		{Kind: "bar", Label: "Revenue by staff", Group: "staff", Of: "amount"},
		{Kind: "donut", Label: "Revenue by payment method", Group: "payment", Of: "amount"},
		{Kind: "table", Label: "Latest sales", Cols: []string{"date", "item", "category", "staff", "payment", "quantity", "amount"}, Sort: "date", Limit: 10},
	},
	Filters: []string{"category", "staff", "payment", "date"},
	Sample:  salesSample,
}

//go:embed samples/roster.xlsx
var rosterSample []byte

var Roster = &Template{
	Key:      "roster",
	Name:     "Roster",
	Blurb:    "Shifts: who works, which shift codes, and how many each day.",
	Question: "Who works when?",
	Blanks: []Blank{
		{Key: "staff", Label: "Staff", Type: Text, Required: true, Help: "Who works the shift", Guess: []string{"staf", "staff", "nama", "name", "pekerja", "nama staf"}},
		{Key: "date", Label: "Date", Type: Date, Required: true, Help: "The day of the shift", Guess: []string{"tarikh", "date", "hari", "day"}},
		{Key: "code", Label: "Code", Type: Text, Required: true, Help: "The shift code, such as P, M or C", Guess: []string{"kod", "code", "syif", "shift", "kod syif", "shift code"}},
		{Key: "department", Label: "Department", Type: Text, Help: "Where they work", Guess: []string{"jabatan", "bahagian", "department", "unit"}},
	},
	Blocks: []Block{
		{Kind: "card", Label: "Staff", Agg: "distinct", Of: "staff"},
		{Kind: "card", Label: "Shifts", Agg: "count"},
		{Kind: "bar", Label: "Shifts by staff", Group: "staff"},
		{Kind: "bar", Label: "Shifts by code", Group: "code"},
		{Kind: "line", Label: "Shifts per day", X: "date", By: "day", Wide: true},
		{Kind: "table", Label: "Roster", Cols: []string{"date", "staff", "department", "code"}, Sort: "date"},
	},
	Filters: []string{"department", "code", "staff", "date"},
	Sample:  rosterSample,
}

// Templates are the Templates a Practice Dashboard offers, in the order the Lab offers them.
var Templates = []*Template{Finance, Inventory, HR, General}

// workTemplates are the Templates a Work Dashboard offers: Practice's four with Sales and Roster.
var workTemplates = []*Template{Finance, Inventory, HR, Sales, Roster, General}

// TemplatesFor is the Templates a Work (work true) or Practice Dashboard offers.
func TemplatesFor(work bool) []*Template {
	if work {
		return workTemplates
	}
	return Templates
}

// TemplateByKey finds a Template by its Key, Work-only ones included, or nil.
func TemplateByKey(key string) *Template {
	for _, t := range workTemplates {
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

// blankType is the type of the first blank one of whose guesses is name.
func (t *Template) blankType(name string) (Type, bool) {
	for _, b := range t.Blanks {
		if b.Named(name) {
			return b.Type, true
		}
	}
	return "", false
}

// Named reports whether col is one of the names Cave guesses for the blank.
func (b Blank) Named(col string) bool {
	for _, g := range b.Guess {
		if strings.EqualFold(col, g) {
			return true
		}
	}
	return false
}

// SampleName is the blank's column name in the Sample: its first guess, capitalised.
func (b Blank) SampleName() string {
	if len(b.Guess) == 0 {
		return b.Label
	}
	return strings.ToUpper(b.Guess[0][:1]) + b.Guess[0][1:]
}
