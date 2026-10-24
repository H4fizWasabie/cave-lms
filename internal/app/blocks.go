package app

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxFilterValues is how many of a column's values a block's filter offers to tick.
// ponytail: a column with more values (receipt numbers) shows only the first ones; a search box if Learners need them.
const maxFilterValues = 100

// option is one choice of a select, a set of radio buttons or of checkboxes on the Blocks tab.
type option struct {
	Value, Text string
	On          bool
	Gone        bool // a ticked filter value the Dataset no longer has
}

// choice is one form field of a block: its name, its label and its options.
type choice struct {
	Name, Label string
	Options     []option
	Off         bool // greyed out: a Locked block's calculation
}

// blockRow is one block in the Blocks tab's list, with its form when it is open.
type blockRow struct {
	lab.WorkBlock
	I           int
	Summary     string
	First, Last bool // first or last of its sort (cards, or charts and tables), so it cannot move that way
	Open        bool
	Fields      []choice // the open block's selects, in order, after Kind and Label
	Radios      []choice // the open block's button rows: by, top, rows, then, width
	ColTicks    *choice  // a table's columns, ticked
	FilterCol   choice   // the filter's column
	FilterVals  *choice  // the filter's values, ticked; nil until a column is picked
	More        int      // the filter offers only this many of the column's values; 0 when it offers all
	Needs       []need   // the columns the Dataset lacks, so the block is paused
	Gone        []string // filter values the Dataset lacks, so the block shows no rows
}

// need is a column a paused block names that the Dataset lacks, with the columns that may stand in for it.
type need struct {
	Name    string
	Options []string
}

// blocksView is the Blocks tab: the list, the add buttons and the filter bar.
type blocksView struct {
	Rows      []blockRow
	Count     string
	Full      bool // 20 blocks: no more of any kind
	CardsFull bool // 8 cards
	Kinds     []option
	FilterBar []option
	Saved     bool
}

// newBlocksView lays out the Work Dashboard's blocks for the Blocks tab in lang, block open (if any) with its form.
func newBlocksView(d *dashboard, lang string, open int, saved bool) *blocksView {
	cfg, ds := d.Config, d.Dataset
	v := &blocksView{Saved: saved, Full: len(cfg.Blocks) >= lab.MaxBlocks, CardsFull: cfg.Cards() >= lab.MaxCards,
		Count: text.T(lang, "lab.b.count", len(cfg.Blocks), lab.MaxBlocks, cfg.Cards(), lab.MaxCards)}
	for _, k := range lab.Kinds {
		v.Kinds = append(v.Kinds, option{Value: k, Text: text.T(lang, "lab.b.kind."+k)})
	}
	for _, c := range ds.Columns {
		if c.Type == lab.Text || c.Type == lab.Date {
			v.FilterBar = append(v.FilterBar, option{Value: c.Name, Text: c.Name, On: slices.Contains(cfg.FilterBar, c.Name)})
		}
	}
	uses := cfg.Uses()
	for i, b := range cfg.Blocks {
		card := b.IsCard()
		row := blockRow{WorkBlock: b, I: i, Summary: b.Summary(lang), Open: i == open, Gone: b.Gone(ds),
			First: i == 0 || cfg.Blocks[i-1].IsCard() != card, Last: i == len(cfg.Blocks)-1 || cfg.Blocks[i+1].IsCard() != card}
		for _, name := range b.Missing(ds) {
			u := uses[slices.IndexFunc(uses, func(u lab.Use) bool { return u.Name == name })]
			row.Needs = append(row.Needs, need{Name: name, Options: u.Options(ds)})
		}
		if row.Open {
			row.form(ds, lang)
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

// columns are the Dataset's column names of type t, any type when t is empty, as options with chosen on; a chosen
// column the Dataset lacks stays first, so the form shows what the block names.
func columns(ds *lab.Dataset, t lab.Type, chosen string, none string) []option {
	var out []option
	if none != "" {
		out = append(out, option{Value: "", Text: none, On: chosen == ""})
	}
	found := false
	for _, c := range ds.Columns {
		if t == "" || c.Type == t {
			out = append(out, option{Value: c.Name, Text: c.Name, On: c.Name == chosen})
			found = found || c.Name == chosen
		}
	}
	if chosen != "" && !found {
		out = append([]option{{Value: chosen, Text: chosen, On: true}}, out...)
	}
	return out
}

// buttons is a row of radio buttons, values paired with their catalog keys.
func buttons(lang, name, label string, chosen string, pairs ...string) choice {
	c := choice{Name: name, Label: text.T(lang, label)}
	for i := 0; i < len(pairs); i += 2 {
		c.Options = append(c.Options, option{Value: pairs[i], Text: text.T(lang, pairs[i+1]), On: pairs[i] == chosen})
	}
	return c
}

// form fills in the open block's fields for its kind.
func (row *blockRow) form(ds *lab.Dataset, lang string) {
	b := row.WorkBlock
	field := func(name, label string, opts []option) {
		row.Fields = append(row.Fields, choice{Name: name, Label: text.T(lang, label), Options: opts})
	}
	switch b.Kind {
	case "line":
		field("x", "lab.b.date", columns(ds, lab.Date, b.X, ""))
	case "bar", "donut":
		field("group", "lab.b.group", columns(ds, lab.Text, b.Group, ""))
	}
	if v := b.Value; v != nil {
		var aggs []option
		for _, a := range lab.Aggs {
			aggs = append(aggs, option{Value: a, Text: text.T(lang, "lab.b.agg."+a), On: v.Agg == a})
		}
		field("agg", "lab.b.value", aggs)
		row.Fields[len(row.Fields)-1].Off = b.Locked
		switch v.Agg {
		case "sum", "avg":
			field("col", "lab.b.column", columns(ds, lab.Number, v.Col, ""))
		case "distinct":
			field("col", "lab.b.column", columns(ds, "", v.Col, ""))
		case "calc":
			field("a", "lab.b.calc-a", columns(ds, lab.Number, v.A, ""))
			var ops []option
			for _, o := range lab.Ops {
				ops = append(ops, option{Value: o[0], Text: o[1], On: v.Op == o[0]})
			}
			field("op", "lab.b.calc-op", ops)
			field("b", "lab.b.calc-b", columns(ds, lab.Number, v.B, ""))
			row.Radios = append(row.Radios, buttons(lang, "then", "lab.b.then", v.Then, "sum", "lab.b.then.sum", "avg", "lab.b.then.avg"))
		}
	}
	switch b.Kind {
	case "line":
		field("split", "lab.b.split", columns(ds, lab.Text, b.Split, text.T(lang, "lab.b.none")))
		row.Radios = append(row.Radios, buttons(lang, "by", "lab.b.by-word", b.By, "day", "lab.b.by-day", "month", "lab.b.by-month"))
	case "bar":
		field("split", "lab.b.split", columns(ds, lab.Text, b.Split, text.T(lang, "lab.b.none")))
		top := choice{Name: "top", Label: text.T(lang, "lab.b.top")}
		for _, n := range lab.Tops {
			o := option{Value: strconv.Itoa(n), Text: text.T(lang, "lab.b.top-n", n), On: b.Top == n}
			if n == 0 {
				o.Text = text.T(lang, "lab.b.top-all")
			}
			top.Options = append(top.Options, o)
		}
		row.Radios = append(row.Radios, top)
	case "table":
		cols := choice{Name: "cols", Label: text.T(lang, "lab.b.cols")}
		for _, c := range ds.Columns {
			cols.Options = append(cols.Options, option{Value: c.Name, Text: c.Name, On: slices.Contains(b.Cols, c.Name)})
		}
		row.ColTicks = &cols
		field("sort", "lab.b.sort", columns(ds, "", b.Sort, text.T(lang, "lab.b.none")))
		limit := choice{Name: "limit", Label: text.T(lang, "lab.b.rows")}
		for _, n := range lab.Limits {
			limit.Options = append(limit.Options, option{Value: strconv.Itoa(n), Text: strconv.Itoa(n), On: b.Limit == n})
		}
		row.Radios = append(row.Radios, limit)
	}
	if len(b.Where) == 2 { // a Locked block's own columns
		row.Fields = append(row.Fields, choice{Name: "where", Label: text.T(lang, "lab.b.lock-where", b.Where[1]), Options: columns(ds, lab.Text, b.Where[0], "")})
	}
	if len(b.Minus) == 2 {
		row.Fields = append(row.Fields, choice{Name: "minus", Label: text.T(lang, "lab.b.lock-minus", b.Minus[1]), Options: columns(ds, lab.Text, b.Minus[0], "")})
	}
	if len(b.Low) == 2 {
		field("low-a", "lab.b.lock-low-a", columns(ds, lab.Number, b.Low[0], ""))
		field("low-b", "lab.b.lock-low-b", columns(ds, lab.Number, b.Low[1], ""))
	}
	var f lab.Filter
	if b.Filter != nil {
		f = *b.Filter
	}
	row.FilterCol = choice{Name: "fcol", Label: text.T(lang, "lab.b.filter"), Options: columns(ds, lab.Text, f.Col, text.T(lang, "lab.b.filter-all"))}
	if f.Col != "" {
		vals := choice{Name: "fval", Label: text.T(lang, "lab.b.filter-is", f.Col)}
		seen := map[string]bool{}
		var all []string
		if i := slices.IndexFunc(ds.Columns, func(c lab.Column) bool { return c.Name == f.Col }); i >= 0 {
			for _, r := range ds.Rows {
				s, ok := r[i].(string)
				if r[i] == nil {
					s, ok = "(blank)", true // as the Dashboard groups empty cells
				}
				if ok && !seen[s] {
					seen[s] = true
					all = append(all, s)
				}
			}
		}
		sort.Strings(all)
		if len(all) > maxFilterValues {
			all, row.More = all[:maxFilterValues], maxFilterValues
		}
		for _, s := range f.Vals { // a ticked value stays offered even when the Dataset no longer has it
			if !slices.Contains(all, s) {
				all = append(all, s)
			}
		}
		for _, s := range all {
			o := option{Value: s, Text: s, On: slices.Contains(f.Vals, s), Gone: slices.Contains(row.Gone, s)}
			if s == "(blank)" {
				o.Text = text.T(lang, "lab.b.blank")
			}
			vals.Options = append(vals.Options, o)
		}
		row.FilterVals = &vals
	}
	if !b.IsCard() {
		wide := "half"
		if b.Wide {
			wide = "full"
		}
		row.Radios = append(row.Radios, buttons(lang, "width", "lab.b.width", wide, "full", "lab.b.full", "half", "lab.b.half"))
	}
}

// blockFromForm is block old as the open block's form f changes it. A new kind starts from that kind's defaults,
// keeping the label, value, filter and width; otherwise each option is as posted. The label follows the choices
// until the Learner types one: a typed label stays, an emptied one follows again.
func blockFromForm(f url.Values, old lab.WorkBlock, ds *lab.Dataset, lang string) lab.WorkBlock {
	b := old
	if old.Value != nil {
		v := *old.Value
		b.Value = &v
	}
	kind, agg := f.Get("kind"), f.Get("agg")
	if old.Locked { // kind and value are greyed out, so not posted: they stay, and KeepsLock refuses a change
		kind = cmp.Or(kind, old.Kind)
		if old.Value != nil {
			agg = cmp.Or(agg, old.Value.Agg)
		}
		repoint := func(p []string, name string) []string {
			if len(p) != 2 {
				return p
			}
			return []string{cmp.Or(f.Get(name), p[0]), p[1]}
		}
		b.Where, b.Minus = repoint(old.Where, "where"), repoint(old.Minus, "minus")
		if len(old.Low) == 2 {
			b.Low = []string{cmp.Or(f.Get("low-a"), old.Low[0]), cmp.Or(f.Get("low-b"), old.Low[1])}
		}
	}
	if kind != old.Kind {
		b.Kind = kind
		switch kind {
		case "bar":
			b.Top = 8
		case "line", "table":
			b.Wide = true
		}
	} else {
		b.Value = nil
		if agg != "" {
			b.Value = &lab.Value{Agg: agg, Col: f.Get("col"), A: f.Get("a"), Op: f.Get("op"), B: f.Get("b"), Then: f.Get("then")}
		}
		b.X, b.By, b.Split, b.Group, b.Sort = f.Get("x"), f.Get("by"), f.Get("split"), f.Get("group"), f.Get("sort")
		b.Top, _ = strconv.Atoi(f.Get("top"))
		b.Limit, _ = strconv.Atoi(f.Get("limit"))
		b.Cols = append([]string{}, f["cols"]...) // none ticked stays empty, so Check asks for one
		b.Wide = f.Get("width") == "full"
	}
	b.Filter = nil
	if col := f.Get("fcol"); col != "" {
		vals := []string{}
		if old.Filter != nil && old.Filter.Col == col { // ticks belong to the column they were shown for
			vals = append(vals, f["fval"]...)
		}
		b.Filter = &lab.Filter{Col: col, Vals: vals}
	}
	b.Fill(ds, old.Missing(ds)...)
	switch typed := strings.TrimSpace(f.Get("label")); {
	case typed == "":
		b.Label, b.LabelSet = b.AutoLabel(lang), false
	case typed != old.Label:
		b.Label, b.LabelSet = typed, true
	case !old.LabelSet && b.AutoLabel(lang) != old.AutoLabel(lang):
		b.Label = b.AutoLabel(lang)
	}
	return b
}

// editBlocks applies one edit from the Blocks tab and saves it, live at once on the preview and the Deployment:
// do is add (a block of kind), save (block's form; close shuts it), up, down, remove (block), filter-bar or reset.
// An edit Cave refuses shows why and changes nothing.
func (a *App) editBlocks(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	if !d.Work || d.Config == nil || d.Dataset == nil {
		a.notFound(w, r)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	cfg := *d.Config // edited apart, so a refused edit leaves d.Config as saved
	cfg.Blocks = slices.Clone(cfg.Blocks)
	choices := d.Choices // follow Use instead, so Reset to Template fills the blanks from the new column
	i, err := strconv.Atoi(r.FormValue("block"))
	if err != nil || i < 0 || i >= len(cfg.Blocks) {
		i = -1
	}
	open, at, do := -1, -1, r.FormValue("do") // the block whose form opens, and the one the page scrolls to
	if do != "add" && do != "filter-bar" && do != "reset" && i < 0 {
		a.notFound(w, r)
		return
	}
	switch do {
	case "add":
		open, err = cfg.Add(r.FormValue("kind"), d.Dataset, lang)
	case "save":
		b := blockFromForm(r.Form, cfg.Blocks[i], d.Dataset, lang)
		if err = b.KeepsLock(cfg.Blocks[i]); err == nil {
			err = b.Check(d.Dataset, cfg.Blocks[i].Missing(d.Dataset)...)
		}
		if err == nil {
			cfg.Blocks[i] = b
			open = cfg.Arrange(i)
			err = cfg.Check()
		}
		if r.FormValue("close") != "" {
			open, at = -1, open
		}
	case "up":
		at = cfg.Move(i, -1)
	case "down":
		at = cfg.Move(i, 1)
	case "remove":
		cfg.Blocks = slices.Delete(cfg.Blocks, i, i+1)
	case "use": // a paused block's column swapped for another in every block
		from, to := r.FormValue("from"), r.FormValue("to")
		uses := cfg.Uses()
		k := slices.IndexFunc(uses, func(u lab.Use) bool { return u.Name == from })
		if k < 0 || !(lab.Use{Name: to, Type: uses[k].Type}).Fits(d.Dataset) {
			err = lab.Problem{Key: "lab.err.block-column", Args: []any{to}}
			break
		}
		cfg.Rename(map[string]string{from: to})
		choices = choices.Rename(map[string]string{from: to})
		at = i
	case "reset": // the Template's blocks and filter bar from the current columns; General rebuilds from the Dataset
		cfg, err = *d.Template.Open(d.Choices, d.Dataset), nil
	case "filter-bar":
		bar := append([]string{}, r.Form["bar"]...)
		if err = lab.CheckFilterBar(bar, d.Dataset); err == nil {
			cfg.FilterBar = bar
		}
	default:
		a.notFound(w, r)
		return
	}
	var problem lab.Problem
	if errors.As(err, &problem) {
		p := &dashboardPage{Dashboard: d, Tab: "blocks", Access: acc, Open: i} // the saved blocks, the edited one open
		p.Error = problem.In(lang)
		a.renderDashboard(w, r, http.StatusUnprocessableEntity, p)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	config, err := json.Marshal(cfg)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	chosen, err := json.Marshal(choices)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE dashboards SET config = $2, choices = $3, updated_at = now() WHERE id = $1`, d.ID, config, chosen); err != nil {
		a.serverError(w, r, err)
		return
	}
	to := dashboardURL(d) + "/blocks?saved"
	if open >= 0 {
		to += fmt.Sprintf("&open=%d#b-%d", open, open)
	} else if at >= 0 {
		to += fmt.Sprintf("#b-%d", at)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}
