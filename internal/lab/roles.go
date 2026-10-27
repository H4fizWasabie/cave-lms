package lab

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// Role is what a column is for when Cave builds blocks from it (ADR 0014). Identifiers, constants and free text
// are never charted; they stay in the table.
type Role string

const (
	Constant   Role = "constant"   // one value on every row
	Identifier Role = "identifier" // different on every row, an ID or code, too many values to group by, or dates kept as numbers
	FreeText   Role = "free text"  // notes and long text
	DateRole   Role = "date"
	Dimension  Role = "dimension" // a handful of values to group by, or years
	Measure    Role = "measure"   // a number to add up, or average
)

// profile is one column as Cave reads it to build blocks: its role, how many values it holds, and how it ranks.
type profile struct {
	Column
	Role   Role
	Avg    bool    // a measure averaged, not added up: a price, rate, balance, level, age or score
	Values int     // distinct values in its filled cells
	note   string  // the text catalog key (lab.note.*) saying why it is left out of charts or averaged
	rank   int     // a measure's or date's place by name; lower first
	spread float64 // a dimension's unevenness: how far its groups' sizes differ, relative to their mean
	from   string  // a date's first and last day, "2006-01-02"
	to     string
}

// Word lists for roles, lower case, Malay and English, beside salesBlanks' and stockBlanks' Loose names.
var (
	idWords   = []string{"id", "kod", "code", "sku", "barcode", "invoice", "invois", "resit", "receipt", "ic", "nric", "ref"}
	idNames   = []string{"no", "no.", "bil", "bil.", "#", "number", "nombor"}
	freeWords = []string{"remark", "remarks", "catatan", "nota", "note", "notes", "komen", "comment", "comments", "ulasan",
		"keterangan", "description", "butiran", "details", "alamat", "address"}
	avgWords = []string{"price", "harga", "cost", "kos", "rate", "kadar", "percent", "percentage", "peratus", "balance", "baki",
		"stock", "stok", "age", "umur", "score", "skor", "markah", "average", "avg", "purata"}
	// sumWords keep a column added up though it names a price or stock: a total cost, stock in, a stock value.
	sumWords = []string{"total", "jumlah", "amount", "amaun", "value", "nilai", "in", "out", "masuk", "keluar", "sold", "jual", "received", "issued", "purchase", "belian"}
	// priceWords name a price, which a list of sales adds up: each row's price is what that sale took.
	priceWords = []string{"price", "harga", "cost", "kos"}
	yearWords  = []string{"year", "tahun", "yr"}
	dateWords  = []string{"tarikh", "date", "created"}
	// dateNames rank dates, best first: the names the Finance Template looked for.
	dateNames = []string{"tarikh", "date", "created", "hari", "day"}
)

// mainNames rank measures, best first: Sales' Amount names, then the closing and actual counts Inventory looks for,
// in English and Malay.
var mainNames = append(slices.Clone(salesBlanks[0].Loose), "closing", "actual", "sebenar", "akhir")

// words are a column name's lower-case words: "No. Invoice" is [no invoice].
func words(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func anyWord(ws, list []string) bool {
	return slices.ContainsFunc(ws, func(w string) bool { return slices.Contains(list, w) })
}

// rankOf is the place in names of the first one name holds, or len(names).
func rankOf(name string, names []string) int {
	key := columnKey(name)
	if i := slices.IndexFunc(names, func(n string) bool { return strings.Contains(key, n) }); i >= 0 {
		return i
	}
	return len(names)
}

// isYear reports whether s is a whole year from 1900 to 2100, as a column named like a year holds.
func isYear(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1900 && n <= 2100
}

// yearNamed reports whether a column's name says it holds years: tahun, year.
func yearNamed(name string) bool { return anyWord(words(name), yearWords) }

// dateNamed reports whether a column's name says it holds dates: tarikh, date, created.
func dateNamed(name string) bool { return anyWord(words(name), dateWords) }

// idName is whether the column is named like an ID: an ID word, or No not followed by "of".
func idName(name string) bool {
	ws, key := words(name), columnKey(name)
	if anyWord(ws, idWords) || slices.Contains(idNames, key) {
		return true
	}
	i := slices.Index(ws, "no")
	return i >= 0 && (i+1 == len(ws) || ws[i+1] != "of")
}

// profiles gives every column of d its role. ponytail: names and counts only, no look at values' shapes such as
// phone numbers; add rules when real files need them.
func profiles(d *Dataset) []profile {
	out := make([]profile, len(d.Columns))
	for ci, col := range d.Columns {
		p := profile{Column: col}
		counts := map[any]int{}
		filled, runes, years := 0, 0, true
		for _, r := range d.Rows {
			v := r[ci]
			if v == nil {
				continue
			}
			filled++
			counts[v]++
			switch x := v.(type) {
			case string:
				runes += utf8.RuneCountInString(x)
				if p.from == "" || x < p.from {
					p.from = x
				}
				p.to = max(p.to, x)
				years = years && isYear(x)
			case float64:
				years = years && isYear(strconv.FormatFloat(x, 'f', -1, 64))
			}
		}
		p.Values = len(counts)
		ws := words(col.Name)
		role := func(r Role, note string) { p.Role, p.note = r, note }
		switch {
		case p.Values <= 1:
			role(Constant, "lab.note.constant")
		case col.Type == Date:
			role(DateRole, "")
			p.rank = rankOf(col.Name, dateNames)
		case col.Type != Date && yearNamed(col.Name) && years: // Clean-up types them as Text, so they group
			role(Dimension, "lab.note.year")
		case col.Type == Number && dateNamed(col.Name): // a day of the month, or Excel's days since 1900
			role(Identifier, "lab.note.serial-date")
		case col.Type == Number && idName(col.Name):
			role(Identifier, "lab.note.id")
		case col.Type == Number:
			role(Measure, "")
			p.rank = rankOf(col.Name, mainNames)
			if p.Avg = anyWord(ws, avgWords) && !anyWord(ws, sumWords) || strings.Contains(col.Name, "%"); p.Avg {
				p.note = "lab.note.average"
			}
		case anyWord(ws, freeWords) || runes > 40*filled:
			role(FreeText, "lab.note.free-text")
		case idName(col.Name) && !(anyWord(ws, []string{"kod", "code"}) && p.Values <= 15): // a handful of codes is a category
			role(Identifier, "lab.note.id")
		case p.Values == filled && filled >= 10: // ponytail: fewer rows can't tell an ID from a category
			role(Identifier, "lab.note.unique")
		case p.Values > 50:
			role(Identifier, "lab.note.many")
		default:
			role(Dimension, "")
		}
		if p.Role == Dimension {
			mean, sq := float64(filled)/float64(p.Values), 0.0
			for _, n := range counts {
				sq += (float64(n) - mean) * (float64(n) - mean)
			}
			p.spread = math.Sqrt(sq/float64(p.Values)) / mean
		}
		out[ci] = p
	}
	addUpSalePrices(d, out)
	return out
}

// addUpSalePrices stops averaging the prices on a list of sales: rows with a date and a grouping (an item, a payment
// method, a staff name) whose values come again, where each row's price is what that sale took. A price beside a
// quantity stays averaged, being per unit; the price times quantity card adds it up. ponytail: a dated list with
// any repeating grouping counts as sales; a price list that repeats a category on every update would be summed too,
// so look at whether the items themselves repeat if one turns up.
func addUpSalePrices(d *Dataset, ps []profile) {
	dated := slices.ContainsFunc(ps, func(p profile) bool { return p.Role == DateRole })
	grouped := slices.ContainsFunc(ps, func(p profile) bool { return p.Role == Dimension && p.Name != sheetColumn })
	if q, _ := priceAndQuantity(d, ps); !dated || !grouped || q != "" {
		return
	}
	for i, p := range ps {
		if p.Avg && averagedForPriceOnly(words(p.Name)) {
			ps[i].Avg, ps[i].note = false, ""
		}
	}
}

// averagedForPriceOnly reports whether a column named by ws is averaged only for naming a price, not a rate,
// balance, level, age or score as well.
func averagedForPriceOnly(ws []string) bool {
	return anyWord(ws, priceWords) && !slices.ContainsFunc(avgWords, func(w string) bool {
		return !slices.Contains(priceWords, w) && slices.Contains(ws, w)
	})
}

// Note is why a column is left out of charts or averaged, as a text catalog key and the column's name, so the
// preview can word it in the Learner's language.
type Note struct {
	Key string `json:"key"`
	Col string `json:"col"`
}

// In words n in lang.
func (n Note) In(lang string) string { return text.T(lang, n.Key, n.Col) }

// notes are d's columns that Cave leaves out of charts or averages, each with its reason.
func notes(d *Dataset) []Note {
	var out []Note
	for _, p := range profiles(d) {
		if p.note != "" {
			out = append(out, Note{Key: p.note, Col: p.Name})
		}
	}
	return out
}

// ranked are the profiles of role r and type t, best first: measures added up before averaged ones, then by name;
// dates by name; dimensions with 3 to 15 values first (the Sheet column of combined sheets before all), then the
// most uneven. Ties keep column order.
func ranked(ps []profile, r Role, t Type) []profile {
	var out []profile
	for _, p := range ps {
		if p.Role == r && p.Type == t {
			out = append(out, p)
		}
	}
	band := func(p profile) int {
		switch {
		case p.Name == sheetColumn:
			return 0
		case p.Values >= 3 && p.Values <= 15:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b profile) int {
		switch r {
		case Measure:
			return cmp.Or(boolCmp(a.Avg, b.Avg), a.rank-b.rank)
		case Dimension:
			return cmp.Or(band(a)-band(b), cmp.Compare(b.spread, a.spread))
		}
		return a.rank - b.rank
	})
	return out
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// General's budget (ADR 0014): at most 4 cards and 2 bars; a donut only for a dimension of up to 6 values, a line
// split only by one of up to 5; a table of up to 6 columns.
const (
	generalCards       = 4
	generalBars        = 2
	generalDonutValues = 6
	generalLineSeries  = 5
	generalTableCols   = 6
)

// mainMeasure is the measure blocks add up: the best-ranked of measures when it is added up, not averaged; empty
// when there is none, and blocks count rows.
func mainMeasure(measures []profile) string {
	if len(measures) > 0 && !measures[0].Avg {
		return measures[0].Name
	}
	return ""
}

// tableCols are the columns of General's table: the date, an identifier, then the top dimensions and measures; a
// short table fills up with the rest, in column order.
func tableCols(d *Dataset, ps []profile) []string {
	measures, dims, dates := ranked(ps, Measure, Number), ranked(ps, Dimension, Text), ranked(ps, DateRole, Date)
	var cols []string
	add := func(name string) {
		if len(cols) < generalTableCols && !slices.Contains(cols, name) {
			cols = append(cols, name)
		}
	}
	if len(dates) > 0 {
		add(dates[0].Name)
	}
	if i := slices.IndexFunc(ps, func(p profile) bool { return p.Role == Identifier }); i >= 0 {
		add(ps[i].Name)
	}
	for _, p := range slices.Concat(dims[:min(generalBars, len(dims))], measures[:min(2, len(measures))]) {
		add(p.Name)
	}
	if len(cols) < 4 {
		for _, col := range d.Columns {
			add(col.Name)
		}
	}
	return cols
}

// buildGeneral makes one blank per column, filled by that column, and fills General's budget of blocks with the
// best-ranked columns by role (ADR 0014): cards, a line, bars, a donut and a table.
func buildGeneral(d *Dataset) (*design, choices) {
	t, c := &design{}, choices{}
	key := map[string]string{}
	for i, col := range d.Columns {
		k := fmt.Sprintf("c%d", i+1)
		c[k], key[col.Name] = col.Name, k
	}
	ps := profiles(d)
	measures, dims, dates := ranked(ps, Measure, Number), ranked(ps, Dimension, Text), ranked(ps, DateRole, Date)

	of, what := "", "Rows"
	if m := mainMeasure(measures); m != "" {
		of, what = key[m], m
	}
	var cards []Block
	for _, m := range measures {
		b := Block{Kind: "card", Label: m.Name, Icon: "chart-up", Agg: "sum", Of: key[m.Name]}
		if m.Avg {
			b.Label, b.Agg = "Average "+m.Name, "avg"
		}
		cards = append(cards, b)
	}
	if q, p := priceAndQuantity(d, ps); q != "" {
		calc := Block{Kind: "card", Label: q + " × " + p, Icon: "chart-up", Agg: "sum", Of: key[q], Times: key[p]}
		at := 1 // after the main measure; first when nothing is added up
		if of == "" {
			at = 0
		}
		cards = slices.Insert(cards, min(at, len(cards)), calc)
	}
	cards = append(cards, Block{Kind: "card", Label: "Rows", Icon: "people", Agg: "count"})
	t.Blocks = cards[:min(generalCards, len(cards))]

	if len(dates) > 0 {
		dt := dates[0]
		if by, ok := lineBy(dt); ok {
			line := Block{Kind: "line", Label: what + " by " + by, X: key[dt.Name], By: by, Of: of, Wide: true}
			if s := slices.IndexFunc(dims, func(p profile) bool { return p.Values <= generalLineSeries }); s >= 0 {
				line.Series = key[dims[s].Name]
			}
			t.Blocks = append(t.Blocks, line)
		}
		t.Filters = append(t.Filters, key[dt.Name])
	}
	var bars []string
	for _, g := range dims[:min(generalBars, len(dims))] {
		t.Blocks = append(t.Blocks, Block{Kind: "bar", Label: what + " by " + g.Name, Group: key[g.Name], Of: of})
		bars = append(bars, g.Name)
		t.Filters = append(t.Filters, key[g.Name])
	}
	if i := slices.IndexFunc(dims, func(p profile) bool { return p.Values <= generalDonutValues && !slices.Contains(bars, p.Name) }); i >= 0 {
		t.Blocks = append(t.Blocks, Block{Kind: "donut", Label: what + " by " + dims[i].Name, Group: key[dims[i].Name], Of: of})
	}

	table := Block{Kind: "table", Label: "Rows", Limit: 10}
	if len(dates) > 0 {
		table.Label, table.Sort = "Latest rows", key[dates[0].Name]
	}
	for _, col := range tableCols(d, ps) {
		table.Cols = append(table.Cols, key[col])
	}
	t.Blocks = append(t.Blocks, table)
	return t, c
}

// priceAndQuantity are d's quantity and price measures, by Sales' and Inventory's names, when it has both: a
// price averaged, a quantity another column. Empty when either is missing.
func priceAndQuantity(d *Dataset, ps []profile) (quantity, price string) {
	is := func(name string, avg bool) bool {
		return slices.ContainsFunc(ps, func(p profile) bool { return p.Name == name && p.Role == Measure && (!avg || p.Avg) })
	}
	inv := guess(stockBlanks, d)
	price, quantity = inv["price"], cmp.Or(guess(salesBlanks, d)["quantity"], inv["quantity"])
	if !is(price, true) || !is(quantity, false) || price == quantity {
		return "", ""
	}
	return quantity, price
}

// lineBy is how the line over date column p goes: by day over 62 days or less, else by month; false when the data
// covers only one day or month.
func lineBy(p profile) (string, bool) {
	from, err1 := time.Parse(time.DateOnly, p.from)
	to, err2 := time.Parse(time.DateOnly, p.to)
	if err1 != nil || err2 != nil {
		return "", false
	}
	if to.Sub(from) <= 62*24*time.Hour {
		return "day", p.Values > 1
	}
	return "month", p.from[:7] != p.to[:7]
}
