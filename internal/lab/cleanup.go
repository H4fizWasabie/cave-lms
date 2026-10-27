package lab

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/xuri/excelize/v2"
)

// Settings are the Learner's choices for one Dataset upload. The zero value lets Cave choose everything.
// The Dashboard remembers the JSON fields for its next upload; the others hold for one upload only.
type Settings struct {
	Sheet        string   `json:"sheet,omitempty"`        // the sheet to read; empty, or not a readable sheet in the file, lets Cave pick
	Alone        bool     `json:"alone,omitempty"`        // read only Sheet; else every sheet with the same columns joins it
	TwoRowHeader bool     `json:"twoRowHeader,omitempty"` // the header is two rows, joined into one name per column
	Ticked       []string `json:"-"`                      // of the sheets that can join, just these; nil is all of them
	HeaderRow    int      `json:"-"`                      // the header's row number in Excel, on every sheet; 0 lets Cave find each
	Month        int      `json:"-"`                      // the month, 1 to 12, of every days-across grid; 0 lets Cave find it
	Year         int      `json:"-"`                      // the grids' year, with Month; 0 is this year
	Recipe       *Recipe  `json:"recipe,omitempty"`       // the first Part's way of reading, once the Dataset has Parts; nil reads as for a first Part
	FileName     string   `json:"-"`                      // the uploaded file's name, where its Part date may be named; empty for none
}

// Recipe is how a Dataset's first Part was read (ADR 0022), and how every later Part is: only its Sheet, found by
// name ignoring case and never combined with others, and its HeaderRow, unless the Learner gives one.
type Recipe struct {
	Sheet     string   `json:"sheet"`
	HeaderRow int      `json:"headerRow"`
	Columns   []Column `json:"-"` // the Dataset's columns, set by the caller: when no sheet has the name, the sheets with them can be picked
}

// Sheet is one sheet of an uploaded workbook.
type Sheet struct {
	Name       string `json:"name"`
	Rows       int    `json:"rows"`                 // data rows below its header; 0 if it cannot be read
	Readable   bool   `json:"readable,omitempty"`   // Clean-up can read it as a table, so it can be picked
	CanCombine bool   `json:"canCombine,omitempty"` // it has the picked sheet's columns, so it can be ticked to join it
	ReasonKey  string `json:"reason,omitempty"`     // the text catalog key (lab.clean.sheet-*) saying why it cannot join the picked sheet
	Fits       bool   `json:"fits,omitempty"`       // under a Recipe: it has the Dataset's columns, so it can be picked
}

// MaxCleanRows is the most rows Clean-up keeps and MaxColumns the most columns, both counted on the Dataset as
// made: combined, with empty columns dropped and the Sheet or Date column added.
const (
	MaxCleanRows = 50000
	MaxColumns   = 100
)

// MaxMerges is the most merged ranges a workbook may have. A real export has a few hundred; each range is work for
// every header and filled-down cell it spans.
const MaxMerges = 10000

// Change is one thing Clean-up changed, as a text catalog key and the numbers it fills in, then the column it
// was in, if one, so the preview can word it in the Learner's language.
type Change struct {
	Key    string `json:"key"`
	Args   []int  `json:"args,omitempty"`
	Column string `json:"column,omitempty"`
}

// In words c in lang.
func (c Change) In(lang string) string {
	var args []any
	for _, a := range c.Args {
		args = append(args, a)
	}
	if c.Column != "" {
		args = append(args, c.Column)
	}
	return text.T(lang, c.Key, args...)
}

// CleanUp is what Clean-up made of an upload: the Dataset and every change it made to get there. Its JSON leaves
// out the Dataset, which is kept on its own.
type CleanUp struct {
	Sheets      []Sheet  `json:"sheets"`           // every sheet in the workbook, in order
	Used        []string `json:"used"`             // the sheets that went into the Dataset, in workbook order
	PickedSheet string   `json:"picked,omitempty"` // the sheet Clean-up read first, the one the others joined
	HeaderRow   int      `json:"headerRow"`        // the picked sheet's header row number in Excel
	Month       int      `json:"month,omitempty"`  // the month, 1 to 12, the days-across grids were dated in; 0 when no grid was used
	Year        int      `json:"year,omitempty"`   // the grids' year, with Month
	Dataset     *Dataset `json:"-"`
	Changes     []Change `json:"changes"`
	Notes       []Note   `json:"notes,omitempty"`    // the columns General leaves out of charts or averages, and why
	PartDate    string   `json:"partDate,omitempty"` // the Part's date as 2006-01-02, found in the Excel; empty when it names none
	DateFrom    string   `json:"dateFrom,omitempty"` // where PartDate was found: DateFromCell, DateFromSheet or DateFromFile
	DateCell    string   `json:"dateCell,omitempty"` // the cell it was found in, such as H2, when DateFrom is DateFromCell
}

// Where a Part's date was found, in the order Clean-up tries them. A Part with none takes the upload day, which the
// app records as "upload", and a date the Learner changed in the preview as "chosen".
const (
	DateFromCell  = "cell"  // a date in the title rows above the column names
	DateFromSheet = "sheet" // a date as the sheet's name
	DateFromFile  = "file"  // a date in the file's name
)

// Clean is Clean-up, the Lab's way of reading an upload. Its rules run as steps, in order: read every
// sheet on its own (readTable: header row, merged cells, cut-off, totals at the bottom, empty columns), pick one
// sheet (s.Sheet, else the first flat list with no number for a column name, else the first flat list, else the
// first readable sheet), combine the sheets with the same columns into one Dataset, type each column from its
// cells (- left empty, more date forms), and note the columns General leaves out of charts. Every step that changes
// something lists it in Changes. A file it cannot read gives one Problem.
func Clean(r io.Reader, s Settings) (*CleanUp, error) {
	if s.Recipe != nil && s.HeaderRow == 0 {
		s.HeaderRow = s.Recipe.HeaderRow
	}
	f, err := openWorkbook(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names := f.GetSheetList()
	tables, problems := make([]*table, len(names)), make([]error, len(names))
	res := &CleanUp{}
	merges := 0
	for i, name := range names {
		l := readLayout(f, name)
		if merges += len(l.merges); merges > MaxMerges {
			return nil, Problem{"lab.err.too-many-merges", []any{thousands(MaxMerges)}}
		}
		tables[i], problems[i] = readTable(f, name, l, s)
	}
	year, month := gridMonth(tables, time.Now())
	if s.Month >= 1 && s.Month <= 12 {
		year, month = cmp.Or(s.Year, time.Now().Year()), time.Month(s.Month)
	}
	for i, tb := range tables {
		if tb != nil && tb.days > 0 {
			if tb.unpivot(year, month); len(tb.raw) == 0 {
				tables[i], problems[i] = nil, Problem{"lab.err.grid-empty", []any{tb.header + 1}}
			}
		}
	}
	for i, name := range names {
		sh := Sheet{Name: name, ReasonKey: "lab.clean.sheet-unreadable"}
		if tb := tables[i]; tb != nil {
			sh = Sheet{Name: name, Rows: len(tb.raw), Readable: true}
		}
		res.Sheets = append(res.Sheets, sh)
	}

	pick := slices.Index(names, s.Sheet)
	if rc := s.Recipe; rc != nil && (pick < 0 || tables[pick] == nil) {
		// The Recipe's sheet by name ignoring case, else the first with the Dataset's columns.
		pick = slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, rc.Sheet) })
		if pick < 0 || tables[pick] == nil {
			pick = slices.IndexFunc(tables, func(tb *table) bool { return tb != nil && sameColumns(tb.cols, rc.Columns) })
		}
	}
	if pick < 0 || tables[pick] == nil {
		// A header with a number for a column name is a summary's label and value pair, not a list's header.
		// ponytail: a list with years for column names ranks down too; the Learner can pick the sheet instead.
		pick = slices.IndexFunc(tables, func(tb *table) bool {
			return tb != nil && tb.flat && !slices.ContainsFunc(tb.cols, func(c Column) bool { _, ok := number(c.Name); return ok })
		})
		if pick < 0 {
			pick = slices.IndexFunc(tables, func(tb *table) bool { return tb != nil && tb.flat })
		}
		if pick < 0 {
			pick = slices.IndexFunc(tables, func(tb *table) bool { return tb != nil })
		}
	}
	// onSheet names the sheet a Problem is on, when the workbook has several.
	onSheet := func(p error, sheet string) error {
		if len(names) > 1 {
			return Problem{"lab.err.on-sheet", []any{sheet, p}}
		}
		return p
	}
	if pick < 0 {
		i := likeliest(f, names, s.Sheet)
		return nil, onSheet(problems[i], names[i])
	}
	base := tables[pick]
	res.PickedSheet = base.name
	var used []*table
	for i, tb := range tables {
		switch sh := &res.Sheets[i]; {
		case tb == nil:
			continue
		case s.Recipe != nil: // a Part is read from one sheet, never combined with others
			sh.Fits = sameColumns(tb.cols, s.Recipe.Columns)
			if i != pick {
				continue
			}
		case i == pick:
		case !sameColumns(tb.cols, base.cols):
			sh.ReasonKey = "lab.clean.sheet-differs"
			continue
		default:
			sh.CanCombine = true
			if s.Alone || s.Ticked != nil && !slices.Contains(s.Ticked, tb.name) {
				continue
			}
		}
		used = append(used, tb)
		res.Used = append(res.Used, tb.name)
	}

	d := &Dataset{Columns: slices.Clone(base.cols)}
	var raw [][]cell
	for _, tb := range used {
		at := make([]int, len(d.Columns)) // each Dataset column's index in tb
		for ci, c := range d.Columns {
			at[ci] = slices.IndexFunc(tb.cols, func(o Column) bool { return columnKey(o.Name) == columnKey(c.Name) })
		}
		for ri, vals := range tb.raw {
			row := make([]cell, len(at))
			for ci, j := range at {
				row[ci] = vals[j]
			}
			raw, d.RowNumbers = append(raw, row), append(d.RowNumbers, tb.rowNo[ri])
		}
	}
	switch {
	case len(raw) > MaxCleanRows && len(used) > 1:
		return nil, Problem{"lab.err.too-many-rows-combined", []any{len(used), thousands(len(raw)), thousands(MaxCleanRows)}}
	case len(raw) > MaxCleanRows:
		return nil, onSheet(Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxCleanRows), thousands(d.RowNumbers[MaxCleanRows])}}, base.name)
	}
	res.HeaderRow = base.header + 1
	res.PartDate, res.DateFrom, res.DateCell = partDate(base, s.FileName)
	for _, tb := range used {
		res.addChanges(tb.changes)
	}
	res.Dataset = typed(d, raw, d.RowNumbers, res)
	if slices.ContainsFunc(used, func(tb *table) bool { return tb.days > 0 }) {
		res.Month, res.Year = int(month), year
		res.Changes = append(res.Changes, Change{Key: "lab.clean.unpivoted", Args: []int{len(raw), res.Month, year}})
	}
	if len(used) > 1 {
		res.Changes = append(res.Changes, addSheetColumn(d, used))
	}
	switch n := len(d.Columns); {
	case n > MaxColumns && len(used) > 1:
		return nil, Problem{"lab.err.too-many-columns-combined", []any{n, d.Columns[0].Name, MaxColumns}}
	case n > MaxColumns:
		return nil, onSheet(Problem{"lab.err.too-many-columns", []any{n, MaxColumns}}, base.name)
	}
	res.Notes = notes(d)
	return res, nil
}

// table is one sheet as Clean-up reads it, before its columns are typed.
type table struct {
	name    string
	header  int // the header row's index
	cols    []Column
	raw     [][]cell // each data row, as wide as cols
	rowNo   []int    // each data row's number in Excel
	flat    bool     // a flat list, not a grouped notes table with a label on a group's first row only
	days    int      // a days-across grid's day columns, after its staff columns in raw until unpivot; 0 if not a grid
	title   string   // a grid's title rows and sheet name, where its month may be named
	date    string   // the first date in the title rows above the header, as 2006-01-02; empty if none
	dateAt  string   // the cell it is in, such as H2
	changes []Change // what reading the sheet changed

}

// readTable reads one sheet as a table, or gives the Problem that stops it. Its rules run in order: find the
// header row below any title rows (or take the Learner's, of one or two rows), fill merged data cells down,
// stop at the first fully blank row and column, then drop totals and labels at the bottom and columns with no data.
func readTable(f *excelize.File, name string, l layout, s Settings) (*table, error) {
	cells, err := sheetRows(f, name, excelize.Options{RawCellValue: true})
	if p, ok := err.(Problem); ok {
		return nil, p
	} else if err != nil {
		return nil, Problem{"lab.err.sheet-unreadable", []any{err.Error()}}
	}
	h := s.HeaderRow - 1
	if h < 0 {
		if h = headerRow(cells); s.TwoRowHeader && groupsAbove(l.merges, cells, h) {
			h--
		}
	}
	first := h + 1 // the first data row
	if s.TwoRowHeader {
		first++
	}
	switch {
	case h < 0:
		return nil, Problem{"lab.err.no-header", nil}
	case first >= len(cells):
		return nil, Problem{"lab.err.no-rows", []any{h + 1}}
	}
	res := &CleanUp{} // collects this sheet's changes
	date, dateAt := titleDate(cells[:h], l)
	if s.HeaderRow > 0 {
		res.change("lab.clean.header-chosen", h+1)
	}
	switch {
	case h == 1:
		res.change("lab.clean.title-row", h+1)
	case h > 1:
		res.change("lab.clean.title-rows", h, h+1)
	}
	// A days-across grid has its own reading, so the cleaning rules below do not run on it.
	if first, days := gridDays(cells[h]); days > 0 {
		tb, err := readGrid(name, cells, h, first, days)
		if tb != nil {
			tb.changes, tb.date, tb.dateAt = res.Changes, date, dateAt
		}
		return tb, err
	}

	if cells, err = fillDown(l.merges, cells, first, res); err != nil {
		return nil, err
	}
	if cells, err = cutOff(cells, h, first, res); err != nil {
		return nil, err
	}
	header := cells[h]
	if s.TwoRowHeader {
		header = joinHeader(l.merges, cells, h, res)
	}
	raw, rowNo := rawRows(l, cells, first-1, len(header))
	if len(raw) == 0 {
		return nil, Problem{"lab.err.no-rows", []any{h + 1}}
	}
	raw, rowNo = dropBottom(raw, rowNo, res)
	header, raw = dropEmpty(header, raw, res)
	cols, err := columnsOf(header)
	if p, ok := err.(Problem); ok {
		switch p.Key {
		case "lab.err.unnamed-column":
			return nil, Problem{"lab.err.header-unnamed", append(p.Args, h+1)}
		case "lab.err.same-name":
			return nil, Problem{"lab.err.header-same-name", append(p.Args, h+1)}
		}
		return nil, err
	}
	// ponytail: a blank first cell on any row reads as a grouped table; the Learner can pick the sheet instead.
	flat := !slices.ContainsFunc(raw, func(vals []cell) bool { return vals[0].v == "" })
	return &table{name: name, header: h, cols: cols, raw: raw, rowNo: rowNo, flat: flat, changes: res.Changes, date: date, dateAt: dateAt}, nil
}

// groupsAbove reports whether header row h, found by headerRow, is the bottom row of a two-row header whose
// top row holds only a few group names: data starts right below h, and the row above has two names or more, or
// a name merged across columns other than from the first (a title merged across the sheet starts at A).
func groupsAbove(merges [][4]int, cells [][]string, h int) bool {
	if h < 1 || h+1 >= len(cells) || !slices.ContainsFunc(cells[h+1], func(v string) bool { _, ok := number(strings.TrimSpace(v)); return ok }) {
		return false
	}
	names := 0
	for _, v := range cells[h-1] {
		if strings.TrimSpace(v) != "" {
			names++
		}
	}
	return names >= 2 || slices.ContainsFunc(merges, func(m [4]int) bool { return m[0] == h-1 && m[1] > 0 && m[3] > m[1] })
}

// likeliest is the index of the sheet whose Problem to give when no sheet reads: the one the Learner picked,
// else the one with the most filled rows, likely the table rather than a cover or notes sheet.
func likeliest(f *excelize.File, names []string, picked string) int {
	if i := slices.Index(names, picked); i >= 0 {
		return i
	}
	best, most := 0, -1
	for i, name := range names {
		// Raw, as readTable reads: through a number format of many '@', a cell's text repeats once per '@'.
		cells, _ := sheetRows(f, name, excelize.Options{RawCellValue: true})
		n := 0
		for _, row := range cells {
			if strings.TrimSpace(strings.Join(row, "")) != "" {
				n++
			}
		}
		if n > most {
			best, most = i, n
		}
	}
	return best
}

// columnKey is a column name as sheets are matched on: ignoring case and extra spaces.
func columnKey(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), " ")) }

// sameColumns reports whether a and b have the same column names, in any order.
func sameColumns(a, b []Column) bool {
	keys := func(cols []Column) []string {
		var out []string
		for _, c := range cols {
			out = append(out, columnKey(c.Name))
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(keys(a), keys(b))
}

// columnChanges lists the columns of was that now lacks and those of now that was lacks, matched as sheets are.
// Ignoring case and spaces is safe for Add, which keeps was's names, so a page reading data.json finds the same keys;
// the Replace warning in the app matches names exactly instead, as a Replace serves now's names as they are.
func columnChanges(was, now []Column) (gone, added []string) {
	in := func(cols []Column, c Column) bool {
		return slices.ContainsFunc(cols, func(o Column) bool { return columnKey(o.Name) == columnKey(c.Name) })
	}
	for _, c := range was {
		if !in(now, c) {
			gone = append(gone, c.Name)
		}
	}
	for _, c := range now {
		if !in(was, c) {
			added = append(added, c.Name)
		}
	}
	return gone, added
}

// AddPart is more's rows put in cols, the columns of a Dataset that already holds had rows, and the changes it made to
// more's cells. more needs the Dataset's columns by name, in any order, and the two at most limit rows, or any number
// when limit is 0 (a Dataset behind the Learner's own page; each upload keeps to MaxCleanRows in Clean); else a
// Problem names the columns missing and extra, or the row counts. The Dataset's column types stay: a cell of more read
// as another type is read again as its column's, as typed reads one, and one that does not fit is left empty and
// listed as a change, once per column.
func AddPart(cols []Column, had int, more *Dataset, limit int) (*Dataset, []Change, error) {
	if gone, added := columnChanges(cols, more.Columns); gone != nil || added != nil {
		return nil, nil, Problem{"lab.err.add-columns", []any{columnList(gone), columnList(added)}}
	}
	if n := had + len(more.Rows); limit > 0 && n > limit {
		return nil, nil, Problem{"lab.err.add-too-many-rows", []any{thousands(len(more.Rows)), thousands(had), thousands(n), thousands(limit)}}
	}
	at := make([]int, len(cols)) // each column's index in more
	for i, c := range cols {
		at[i] = slices.IndexFunc(more.Columns, func(o Column) bool { return columnKey(o.Name) == columnKey(c.Name) })
	}
	out := &Dataset{Columns: cols}
	misfits := make([]int, len(cols))
	for _, vals := range more.Rows {
		row := make([]any, len(at))
		for i, j := range at {
			row[i] = vals[j]
			if typ := cols[i].Type; row[i] != nil && more.Columns[j].Type != typ {
				var ok bool
				if row[i], ok = retyped(row[i], typ); !ok {
					misfits[i]++
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	var changes []Change
	for i, n := range misfits {
		if n > 0 {
			changes = append(changes, Change{Key: "lab.clean.not-" + string(cols[i].Type) + "s", Args: []int{n}, Column: cols[i].Name})
		}
	}
	return out, changes, nil
}

// retyped is v, a Dataset value of another type, read as typ as typed reads a cell; nil and false when it does not
// fit.
func retyped(v any, typ Type) (any, bool) {
	s, ok := v.(string)
	if !ok {
		s = strconv.FormatFloat(v.(float64), 'f', -1, 64)
	}
	if s == "-" && typ == Number {
		return nil, true
	}
	if day, ok := otherDate(s); ok && typ == Date {
		return day, true
	}
	return parse(cell{v: s}, typ)
}

// columnList is names as a Problem lists them, or the word for none.
func columnList(names []string) any {
	if len(names) == 0 {
		return Problem{"lab.add.none", nil}
	}
	return strings.Join(names, ", ")
}

// sheetColumn names the column Clean-up adds to a Dataset combined from several sheets, holding each row's sheet.
const sheetColumn = "Sheet"

// addSheetColumn puts each row's sheet in a new first column of combined d: a Date column when every sheet's
// name reads as a date, else a Sheet column of names. It gives the change to list.
func addSheetColumn(d *Dataset, used []*table) Change {
	col, change := Column{"Date", Date}, Change{Key: "lab.clean.combined-dates", Args: []int{len(used)}}
	values := make([]any, len(used))
	for i, tb := range used {
		day, ok := sheetDate(tb.name)
		if !ok {
			col, change = Column{sheetColumn, Text}, Change{Key: "lab.clean.combined", Args: []int{len(used)}}
			break
		}
		values[i] = day
	}
	if col.Type == Text {
		for i, tb := range used {
			values[i] = tb.name
		}
	}
	for slices.ContainsFunc(d.Columns, func(c Column) bool { return columnKey(c.Name) == columnKey(col.Name) }) {
		col.Name += " (sheet)"
	}
	d.Columns = append([]Column{col}, d.Columns...)
	ri := 0
	for i, tb := range used {
		for range tb.raw {
			d.Rows[ri] = append([]any{values[i]}, d.Rows[ri]...)
			ri++
		}
	}
	return change
}

var dmyShort = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{2})$`)

// sheetDate reads a sheet name as a date: day/month/year, the year in two digits or four, like 01.09.26.
func sheetDate(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if m := dmyShort.FindStringSubmatch(name); m != nil {
		name = m[1] + "/" + m[2] + "/20" + m[3]
	}
	return textDate(name)
}

// counted are the changes whose one number counts cells or columns, so the sheets' counts add up.
var counted = []string{"lab.clean.merged-down", "lab.clean.empty-columns"}

// bottom are the changes that drop a sheet's totals or notes at the bottom. Several sheets' are listed as one
// lab.clean.bottom-sheets with their count, as every month's export drops its own total row, on rows that differ.
var bottom = []string{"lab.clean.bottom-row", "lab.clean.bottom-rows", "lab.clean.bottom-sheets"}

// addChanges lists one used sheet's changes: a counted change adds to the same change of a sheet before it, a
// bottom change joins the sheets' before it into one with their count, and any other is listed once however
// many sheets made it.
func (res *CleanUp) addChanges(changes []Change) {
	for _, c := range changes {
		if slices.Contains(bottom, c.Key) {
			if i := slices.IndexFunc(res.Changes, func(o Change) bool { return slices.Contains(bottom, o.Key) }); i >= 0 {
				sheets := 2
				if res.Changes[i].Key == "lab.clean.bottom-sheets" {
					sheets = res.Changes[i].Args[0] + 1
				}
				res.Changes[i] = Change{Key: "lab.clean.bottom-sheets", Args: []int{sheets}}
				continue
			}
		}
		i := slices.IndexFunc(res.Changes, func(o Change) bool {
			return o.Key == c.Key && (slices.Contains(counted, c.Key) || slices.Equal(o.Args, c.Args))
		})
		switch {
		case i < 0:
			res.Changes = append(res.Changes, Change{Key: c.Key, Args: slices.Clone(c.Args), Column: c.Column})
		case slices.Contains(counted, c.Key):
			res.Changes[i].Args[0] += c.Args[0]
		}
	}
}

// change lists one thing Clean-up changed.
func (res *CleanUp) change(key string, args ...int) {
	res.Changes = append(res.Changes, Change{Key: key, Args: args})
}

// layout is what Clean-up reads of a sheet besides its values, read straight from the sheet's XML: excelize's own
// readers of merges and styles first pad every row out to its last cell, so a few-KB sheet with a cell far out on
// each row would take gigabytes, and excelize joins overlapping merges pairwise, in time the square of their count.
type layout struct {
	merges [][4]int // each merged range as 0-based [top row, left column, bottom row, right column]
	// A cell's style is its own, else its row's, else its column's, as excelize gives it. Each is kept as whether
	// its number format shows a date.
	cells []int        // the cells with a style of their own, as (0-based row*excelize.MaxColumns + column)<<1 | date, sorted
	rows  map[int]bool // by 0-based row
	cols  []styledCols
}

// styledCols are the columns from min to max, 1-based, that have a style.
type styledCols struct {
	min, max int
	date     bool
}

// date reports whether the cell at 0-based row and col shows a date.
func (l layout) date(row, col int) bool {
	key := row*excelize.MaxColumns + col
	if i, ok := slices.BinarySearchFunc(l.cells, key, func(e, k int) int { return cmp.Compare(e>>1, k) }); ok {
		return l.cells[i]&1 == 1
	}
	if date, ok := l.rows[row]; ok {
		return date
	}
	if i := slices.IndexFunc(l.cols, func(c styledCols) bool { return c.min <= col+1 && col+1 <= c.max }); i >= 0 {
		return l.cols[i].date
	}
	return false
}

// readLayout reads sheet's layout, stopping once it has more than MaxMerges merges. A sheet whose part it cannot
// find or read has none.
func readLayout(f *excelize.File, sheet string) layout {
	l := layout{rows: map[int]bool{}}
	b := sheetPart(f, sheet)
	if b == nil {
		return l
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = f.CharsetReader
	isDate := map[int]bool{} // by style
	date := func(style int) bool {
		if _, ok := isDate[style]; !ok {
			isDate[style] = dateFormat(f, style)
		}
		return isDate[style]
	}
	row, col := 0, 0 // the cell's, 1-based, counted as excelize counts them
	for len(l.merges) <= MaxMerges {
		tok, err := d.Token()
		if err != nil {
			break // as excelize stops reading a sheet at its first bad token
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := map[string]string{}
		for _, a := range el.Attr {
			attrs[a.Name.Local] = a.Value
		}
		style, _ := strconv.Atoi(attrs["s"])
		switch el.Name.Local {
		case "row":
			if row, col = row+1, 0; attrs["r"] != "" {
				if r, err := strconv.Atoi(attrs["r"]); err == nil && r > 0 {
					row = r
				}
			}
			if style != 0 && row <= excelize.TotalRows {
				l.rows[row-1] = date(style)
			}
		case "c":
			if col++; attrs["r"] != "" {
				col, _, _ = excelize.CellNameToCoordinates(attrs["r"])
			}
			if style != 0 && row >= 1 && col >= 1 && col <= excelize.MaxColumns {
				key := ((row-1)*excelize.MaxColumns + col - 1) << 1
				if date(style) {
					key |= 1
				}
				l.cells = append(l.cells, key)
			}
		case "col":
			lo, _ := strconv.Atoi(attrs["min"])
			hi, _ := strconv.Atoi(attrs["max"])
			if style, _ := strconv.Atoi(attrs["style"]); style != 0 {
				l.cols = append(l.cols, styledCols{lo, hi, date(style)})
			}
		case "mergeCell":
			from, to, _ := strings.Cut(attrs["ref"], ":")
			c1, r1, err1 := excelize.CellNameToCoordinates(from)
			c2, r2, err2 := excelize.CellNameToCoordinates(to)
			if err1 == nil && err2 == nil {
				l.merges = append(l.merges, [4]int{min(r1, r2) - 1, min(c1, c2) - 1, max(r1, r2) - 1, max(c1, c2) - 1})
			}
		}
	}
	slices.Sort(l.cells)
	return l
}

// sheetPart is sheet's XML as the file holds it, found the way excelize finds it: the workbook's relationship
// for the sheet points at its part. nil if there is none.
func sheetPart(f *excelize.File, sheet string) []byte {
	part := func(name string) []byte {
		b, _ := f.Pkg.Load(name)
		v, _ := b.([]byte)
		return v
	}
	type relationships struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	target := func(rels []byte, match func(id, typ string) bool) string {
		var r relationships
		xml.Unmarshal(rels, &r)
		for _, rel := range r.Rel {
			if match(rel.ID, rel.Type) {
				return rel.Target
			}
		}
		return ""
	}
	book := strings.TrimPrefix(target(part("_rels/.rels"), func(_, typ string) bool {
		return typ == excelize.SourceRelationshipOfficeDocument || typ == excelize.StrictSourceRelationshipOfficeDocument
	}), "/")
	id := ""
	if f.WorkBook != nil {
		for _, s := range f.WorkBook.Sheets.Sheet {
			if strings.EqualFold(s.Name, sheet) {
				id = s.ID
				break
			}
		}
	}
	if book == "" || id == "" {
		return nil
	}
	to := strings.ReplaceAll(target(part(path.Join(path.Dir(book), "_rels", path.Base(book)+".rels")), func(rid, _ string) bool { return rid == id }), "\\", "/")
	if to == "" {
		return nil
	}
	if strings.HasPrefix(to, "/") {
		return part(strings.TrimPrefix(path.Clean(to), "/"))
	}
	return part(strings.TrimPrefix(path.Join(path.Dir(book), to), "/"))
}

// set puts v at cells[r][c], widening the row as needed.
func set(cells [][]string, r, c int, v string) {
	if c >= len(cells[r]) {
		cells[r] = append(cells[r], make([]string, c+1-len(cells[r]))...)
	}
	cells[r][c] = v
}

// fillDown gives every cell of a merged range spanning rows, from data row first down, its top-left value, so
// grouped rows keep their group on every row. Ranges within one row are left as Excel stores them.
// ponytail: a filled cell keeps its own number format, so a merged Excel date filled down may read as a number.
// Filled and widened cells count against maxSheetCells, so ranges reaching far out are refused before they are held.
func fillDown(merges [][4]int, cells [][]string, first int, res *CleanUp) ([][]string, error) {
	n, widened := 0, 0
	for _, m := range merges {
		top, left, bottom, right := m[0], m[1], m[2], m[3]
		if top < first || bottom == top || top >= len(cells) || left >= len(cells[top]) {
			continue
		}
		v := cells[top][left]
		for r := top; r <= bottom && r < len(cells); r++ {
			if widened += max(right+1-len(cells[r]), 0); n+widened > maxSheetCells {
				return nil, Problem{"lab.err.too-many-cells", nil}
			}
			for c := left; c <= right; c++ {
				if r != top || c != left {
					set(cells, r, c, v)
					n++
				}
			}
		}
	}
	if n > 0 {
		res.change("lab.clean.merged-down", n)
	}
	return cells, nil
}

// joinHeader names each column from header rows h and h+1 joined, as "Jualan Tunai". A merged cell in either
// row is filled across first, so a group name over several columns starts each of their names; a name merged
// down both rows is used once.
func joinHeader(merges [][4]int, cells [][]string, h int, res *CleanUp) []string {
	top, bottom := slices.Clone(cells[h]), slices.Clone(cells[h+1])
	for _, m := range merges {
		for i, row := range [][]string{top, bottom} {
			if m[0] == h+i && m[1] < len(row) {
				for c := m[1] + 1; c <= m[3] && c < len(row); c++ {
					row[c] = row[m[1]]
				}
			}
		}
	}
	names := make([]string, len(top))
	for c := range names {
		t, b := strings.TrimSpace(top[c]), strings.TrimSpace(bottom[c])
		names[c] = strings.TrimSpace(t + " " + b)
		if t == b {
			names[c] = t
		}
	}
	res.change("lab.clean.two-row-header", h+1, h+2)
	return names
}

// cutOff keeps only the table whose header is at cells[h] and whose data starts at cells[first]: it stops at
// the first fully blank row below the data's start and at the first fully blank column (header included) right
// of where the header starts, so notes, legends and side tables past them are not read. Every row it keeps
// from h on is as wide as the table; a table of more than maxSheetCells cells that wide is refused before it is.
func cutOff(cells [][]string, h, first int, res *CleanUp) ([][]string, error) {
	filled := func(r, c int) bool { return c < len(cells[r]) && strings.TrimSpace(cells[r][c]) != "" }
	blankRow := func(r, from, to int) bool {
		for c := from; c < min(to, len(cells[r])); c++ {
			if filled(r, c) {
				return false
			}
		}
		return true
	}
	widest := 0
	for _, row := range cells[h:] {
		widest = max(widest, len(row))
	}
	for first < len(cells) && blankRow(first, 0, widest) {
		first++ // blank lines between the header and the first data row
	}
	end := func(width int) int {
		for r := first; r < len(cells); r++ {
			if blankRow(r, 0, width) {
				return r
			}
		}
		return len(cells)
	}
	rows := end(widest)
	width := widest
	used := make([]bool, widest) // whether each column has a filled cell from the header down, read in one pass
	for _, row := range cells[h:rows] {
		for c, v := range row {
			used[c] = used[c] || strings.TrimSpace(v) != ""
		}
	}
	for c := max(slices.IndexFunc(cells[h], func(v string) bool { return strings.TrimSpace(v) != "" }), 0); c < widest; c++ {
		if !used[c] {
			width = c
			break
		}
	}
	rows = end(width)
	if (rows-h)*width > maxSheetCells {
		return nil, Problem{"lab.err.too-many-cells", nil}
	}

	for r := h; r < rows; r++ {
		if !blankRow(r, width, widest) {
			res.change("lab.clean.cut-columns", width+1)
			break
		}
	}
	for r := rows; r < len(cells); r++ {
		if !blankRow(r, 0, len(cells[r])) {
			res.change("lab.clean.cut-rows", rows+1)
			break
		}
	}
	cells = cells[:rows]
	for r := h; r < rows; r++ {
		cells[r] = append(cells[r], make([]string, max(width-len(cells[r]), 0))...)[:width]
	}
	return cells, nil
}

// dropEmpty drops the columns with no data below the header; those with a name are listed as a change.
func dropEmpty(header []string, raw [][]cell, res *CleanUp) ([]string, [][]cell) {
	var keep []int
	named := 0
	for ci, name := range header {
		if slices.ContainsFunc(raw, func(vals []cell) bool { return vals[ci].v != "" }) {
			keep = append(keep, ci)
		} else if strings.TrimSpace(name) != "" {
			named++
		}
	}
	if len(keep) == len(header) {
		return header, raw
	}
	if named > 0 {
		res.change("lab.clean.empty-columns", named)
	}
	kept := make([]string, len(keep))
	for i, ci := range keep {
		kept[i] = header[ci]
	}
	for ri, vals := range raw {
		row := make([]cell, len(keep))
		for i, ci := range keep {
			row[i] = vals[ci]
		}
		raw[ri] = row
	}
	return kept, raw
}

var totalLabel = regexp.MustCompile(`(?i)^(grand\s+)?(total|jumlah)\b`)

// dropBottom drops the rows at the bottom of the table that hold a total (a cell starting Total or Jumlah) or
// only a label (the first cell filled, not with a number, and no other, in a table of two columns or more). A
// row with its first cell blank is a grouped table's next row, not a label. Rows above the last data row are
// kept, totals or not: only the bottom can be told apart from data.
// ponytail: a last data row with only its first cell filled reads as a label; the Learner fills a cell in Excel.
func dropBottom(raw [][]cell, rowNo []int, res *CleanUp) ([][]cell, []int) {
	n, width := len(raw), len(raw[0])
	for n > 1 {
		filled, total := 0, false
		var last string
		for _, c := range raw[n-1] {
			if c.v != "" {
				filled++
				last = c.v
				total = total || totalLabel.MatchString(c.v)
			}
		}
		_, isNumber := number(last)
		if !total && (filled != 1 || raw[n-1][0].v == "" || width < 2 || isNumber) {
			break
		}
		n--
	}
	switch {
	case n == len(raw)-1:
		res.change("lab.clean.bottom-row", rowNo[n])
	case n < len(raw)-1:
		res.change("lab.clean.bottom-rows", rowNo[n], rowNo[len(raw)-1])
	}
	return raw[:n], rowNo[:n]
}

// typed fills d's rows from raw, each column typed by its cells. Unlike Read, a lone - in a Number column is
// left empty, dates may be written in more ways (see otherDate), and a cell that does not fit its column's type is
// left empty; each is listed as a change, the last once per column.
func typed(d *Dataset, raw [][]cell, rowNo []int, res *CleanUp) *Dataset {
	dashes, others, misfits := 0, 0, make([]int, len(d.Columns))
	for ci, col := range d.Columns {
		d.Columns[ci].Type = workType(raw, ci)
		// Years are groups, not amounts: typed as Text, they can be grouped and filtered by, and never added up.
		if d.Columns[ci].Type == Number && yearNamed(col.Name) &&
			!slices.ContainsFunc(raw, func(vals []cell) bool { return vals[ci].v != "" && !isYear(vals[ci].v) }) {
			d.Columns[ci].Type = Text
		}
		if n := serials(raw, ci); d.Columns[ci].Type == Number && n > 0 && dateNamed(col.Name) {
			d.Columns[ci].Type = Date
			res.Changes = append(res.Changes, Change{Key: "lab.clean.serial-dates", Args: []int{n}, Column: col.Name})
		}
	}
	for _, vals := range raw {
		row := make([]any, len(d.Columns))
		for ci, c := range vals[:len(d.Columns)] {
			typ := d.Columns[ci].Type
			if c.v == "" {
				continue
			}
			if c.v == "-" && typ == Number { // not known, so left empty: 0 would say none
				dashes++
				continue
			}
			if day, ok := otherDate(c.v); ok && typ == Date {
				row[ci] = day
				others++
				continue
			}
			v, ok := parse(c, typ)
			if !ok {
				misfits[ci]++
				continue
			}
			row[ci] = v
		}
		d.Rows = append(d.Rows, row)
	}
	d.RowNumbers = rowNo
	if dashes > 0 {
		res.change("lab.clean.dash-empty", dashes)
	}
	if others > 0 {
		res.change("lab.clean.other-dates", others)
	}
	for ci, n := range misfits {
		if n > 0 {
			res.Changes = append(res.Changes, Change{Key: "lab.clean.not-" + string(d.Columns[ci].Type) + "s", Args: []int{n}, Column: d.Columns[ci].Name})
		}
	}
	return d
}

// workType is a column's type as Clean-up reads it: Date or Number when at least 90% of its filled cells are
// (ADR 0014), else Text. A lone - fits a Number column that has at least one real number, and otherDate's
// forms are dates.
func workType(raw [][]cell, ci int) Type {
	dates, numbers, dashes, filled := 0, 0, 0, 0
	for _, vals := range raw {
		c := vals[ci]
		if c.v == "" {
			continue
		}
		filled++
		if c.v == "-" {
			dashes++
			continue
		}
		_, isNumber := number(c.v)
		_, isDate := textDate(c.v)
		_, isOther := otherDate(c.v)
		if isNumber {
			numbers++
		}
		if isDate || isOther || c.date && isNumber {
			dates++
		}
	}
	most := func(n int) bool { return n > 0 && n*10 >= filled*9 }
	switch {
	case most(dates):
		return Date
	case numbers > 0 && most(numbers+dashes):
		return Number
	}
	return Text
}

// serials counts column ci's numbers when every one of them is an Excel date serial from 1950 to 2100; 0 if not.
// ponytail: day numbers 1 to 31 are not read as serials, which would date them in January 1900.
func serials(raw [][]cell, ci int) int {
	n := 0
	for _, vals := range raw {
		v, ok := number(vals[ci].v)
		switch {
		case vals[ci].v == "-" || ok && (v < 18264 || v > 73415):
			return 0
		case ok:
			n++
		}
	}
	return n
}

var (
	dateTime  = regexp.MustCompile(`(?i)^(\d{1,2}[/.-]\d{1,2}[/.-]\d{4}),?\s+\d{1,2}:\d{2}(:\d{2})?(\s*[ap]\.?m\.?)?$`)
	shortDate = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{2})$`)
	isoDate   = regexp.MustCompile(`^(\d{4}-\d{1,2}-\d{1,2})([T ]\d{1,2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$`)
)

// otherDate reads the date forms Clean-up reads beyond textDate's, as "2006-01-02": a day/month/year date with
// a time (5/11/2026 2:30 PM), whose time is left out; an ISO date, with or without a time (2026-11-05); and
// day.month.two-digit year (05.11.26), this century.
func otherDate(s string) (string, bool) {
	if m := dateTime.FindStringSubmatch(s); m != nil {
		return textDate(m[1])
	}
	if m := shortDate.FindStringSubmatch(s); m != nil {
		return textDate(m[1] + "/" + m[2] + "/20" + m[3])
	}
	if m := isoDate.FindStringSubmatch(s); m != nil {
		day, err := time.Parse("2006-1-2", m[1])
		return day.Format("2006-01-02"), err == nil
	}
	return "", false
}

// headerRow is the index of the first row filled across at least half the widest row (two cells at least,
// when the table is that wide), so title rows above it, usually one cell each, are skipped. -1 if none.
// ponytail: a title row as wide as half the table reads as the header; the Learner can name the row instead.
func headerRow(cells [][]string) int {
	filled := func(row []string) int {
		n := 0
		for _, v := range row {
			if strings.TrimSpace(v) != "" {
				n++
			}
		}
		return n
	}
	widest := 0
	for _, row := range cells {
		widest = max(widest, filled(row))
	}
	need := max(min(widest, 2), (widest+1)/2)
	for i, row := range cells {
		if need > 0 && filled(row) >= need {
			return i
		}
	}
	return -1
}

var (
	textDateIn = regexp.MustCompile(`(?:^|\D)(\d{1,2}[/.-]\d{1,2}[/.-]\d{4}|\d{4}-\d{1,2}-\d{1,2})(?:\D|$)`)
	compactDay = regexp.MustCompile(`(?:^|\D)((?:19|20)\d{2})(\d{2})(\d{2})(?:\d{4}(?:\d{2})?)?(?:\D|$)`)
	nameDay    = regexp.MustCompile(`(?:^|\D)(\d{4}-\d{1,2}-\d{1,2}|\d{1,2}[.-]\d{1,2}[.-](?:\d{4}|\d{2}))(?:\D|$)`)
)

// anyDate reads s as a date in any form Clean-up reads: day/month/year text, or otherDate's forms.
func anyDate(s string) (string, bool) {
	if day, ok := textDate(s); ok {
		return day, true
	}
	return otherDate(s)
}

// titleDate is the first date in the rows above a table's header, reading each row left to right, and its cell:
// a real Excel date, a cell that is a date as text (01.09.2026, 2026-09-01), or a date written inside a cell's
// text (Stock as at 05/11/2026). ponytail: the first date wins, a period's end date is not told from its start.
func titleDate(rows [][]string, l layout) (string, string) {
	for ri, row := range rows {
		for ci, v := range row {
			v = strings.TrimSpace(v)
			day, ok := "", false
			if n, err := strconv.ParseFloat(v, 64); err == nil && l.date(ri, ci) && n >= 1 && n < 2958466 {
				if t, err := excelize.ExcelDateToTime(n, false); err == nil {
					day, ok = t.Format(time.DateOnly), true
				}
			} else if day, ok = anyDate(v); !ok {
				if m := textDateIn.FindStringSubmatch(v); m != nil {
					day, ok = anyDate(m[1])
				}
			}
			if ok {
				cell, _ := excelize.CoordinatesToCellName(ci+1, ri+1)
				return day, cell
			}
		}
	}
	return "", ""
}

// partDate is the date of the Part read from table tb of the file called file, and where it was found: the title
// rows above the header, else the sheet name, else the file name. Empty when the Excel names none.
func partDate(tb *table, file string) (day, from, cell string) {
	if tb.date != "" {
		return tb.date, DateFromCell, tb.dateAt
	}
	if day, ok := sheetDate(tb.name); ok {
		return day, DateFromSheet, ""
	}
	if day, ok := fileDate(file); ok {
		return day, DateFromFile, ""
	}
	return "", "", ""
}

// fileDate reads a date from a file's name: YYYYMMDD, also with a time after it (20260702091058), YYYY-MM-DD,
// DD.MM.YY and DD-MM-YYYY (also DD.MM.YYYY and DD-MM-YY), as "2006-01-02".
func fileDate(name string) (string, bool) {
	if m := compactDay.FindStringSubmatch(name); m != nil {
		if day, err := time.Parse("20060102", m[1]+m[2]+m[3]); err == nil {
			return day.Format(time.DateOnly), true
		}
	}
	if m := nameDay.FindStringSubmatch(name); m != nil {
		if day, err := time.Parse("2006-1-2", m[1]); err == nil {
			return day.Format(time.DateOnly), true
		}
		return sheetDate(m[1])
	}
	return "", false
}
