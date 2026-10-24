package lab

import (
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/xuri/excelize/v2"
)

// Settings are the Learner's choices for one Work Dashboard upload. The zero value lets Cave choose everything.
// The Dashboard remembers the JSON fields for its next upload; the others hold for one upload only.
type Settings struct {
	Sheet        string   `json:"sheet,omitempty"`        // the sheet to read; empty, or not a readable sheet in the file, lets Cave pick
	Alone        bool     `json:"alone,omitempty"`        // read only Sheet; else every sheet with the same columns joins it
	TwoRowHeader bool     `json:"twoRowHeader,omitempty"` // the header is two rows, joined into one name per column
	Ticked       []string `json:"-"`                      // of the sheets that can join, just these; nil is all of them
	HeaderRow    int      `json:"-"`                      // the header's row number in Excel, on every sheet; 0 lets Cave find each
}

// Sheet is one sheet of an uploaded workbook.
type Sheet struct {
	Name       string `json:"name"`
	Rows       int    `json:"rows"`                 // data rows below its header; 0 if it cannot be read
	Readable   bool   `json:"readable,omitempty"`   // Clean-up can read it as a table, so it can be picked
	CanCombine bool   `json:"canCombine,omitempty"` // it has the picked sheet's columns, so it can be ticked to join it
	Reason     string `json:"reason,omitempty"`     // a text catalog key saying why it cannot join the picked sheet
}

// MaxCleanRows is the most rows Clean-up keeps, counted on the combined Dataset; MaxColumns the most columns.
const (
	MaxCleanRows = 50000
	MaxColumns   = 100
)

// Change is one thing Clean-up changed, as a text catalog key and the numbers it fills in,
// so the preview can word it in the Learner's language.
type Change struct {
	Key  string `json:"key"`
	Args []int  `json:"args,omitempty"`
}

// In words c in lang.
func (c Change) In(lang string) string {
	args := make([]any, len(c.Args))
	for i, a := range c.Args {
		args[i] = a
	}
	return text.T(lang, c.Key, args...)
}

// CleanUp is what Clean-up made of an upload: the Dataset, every change it made to get there, and Cave's
// guess for each Template blank.
type CleanUp struct {
	Sheets    []Sheet  // every sheet in the workbook, in order
	Used      []string // the sheets that went into the Dataset, in workbook order
	HeaderRow int      // the picked sheet's header row number in Excel
	Dataset   *Dataset
	Changes   []Change
	Guess     Choices
}

// Clean is Clean-up, a Work Dashboard's way of reading an upload. It reads every sheet on its own, finding
// each header row below any title rows, picks one sheet (s.Sheet, else the first flat list, else the first
// readable sheet), combines the sheets with the same columns into one Dataset, reads each column's type from
// its cells, and guesses the Template's blanks. A file it cannot read gives one Problem.
func Clean(r io.Reader, t *Template, s Settings) (*CleanUp, error) {
	f, err := openWorkbook(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names := f.GetSheetList()
	tables, problems := make([]*table, len(names)), make([]error, len(names))
	res := &CleanUp{}
	for i, name := range names {
		tables[i], problems[i] = readTable(f, name, s)
		sh := Sheet{Name: name, Reason: "lab.clean.sheet-unreadable"}
		if tb := tables[i]; tb != nil {
			sh = Sheet{Name: name, Rows: len(tb.raw), Readable: true}
		}
		res.Sheets = append(res.Sheets, sh)
	}

	pick := slices.Index(names, s.Sheet)
	if pick < 0 || tables[pick] == nil {
		if pick = slices.IndexFunc(tables, func(tb *table) bool { return tb != nil && tb.flat }); pick < 0 {
			pick = slices.IndexFunc(tables, func(tb *table) bool { return tb != nil })
		}
	}
	if pick < 0 {
		return nil, problems[max(slices.Index(names, s.Sheet), 0)]
	}
	base := tables[pick]
	var used []*table
	for i, tb := range tables {
		switch sh := &res.Sheets[i]; {
		case tb == nil:
			continue
		case i == pick:
		case !sameColumns(tb.cols, base.cols):
			sh.Reason = "lab.clean.sheet-differs"
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
		return nil, Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxCleanRows), thousands(d.RowNumbers[MaxCleanRows])}}
	}
	for ci := range d.Columns {
		d.Columns[ci].Type = columnType(raw, ci)
	}
	for _, vals := range raw {
		row := make([]any, len(d.Columns))
		for ci, c := range vals {
			if c.v != "" {
				row[ci], _ = parse(c, d.Columns[ci].Type)
			}
		}
		d.Rows = append(d.Rows, row)
	}

	h := base.header
	res.HeaderRow, res.Dataset = h+1, d
	// ponytail: only the picked sheet's title rows are listed; the other sheets' may differ.
	switch {
	case h == 1:
		res.Changes = append(res.Changes, Change{"lab.clean.title-row", []int{h + 1}})
	case h > 1:
		res.Changes = append(res.Changes, Change{"lab.clean.title-rows", []int{h, h + 1}})
	}
	if len(used) > 1 {
		res.Changes = append(res.Changes, addSheetColumn(d, used))
	}
	if t.Build == nil {
		res.Guess = t.Guess(d)
	}
	return res, nil
}

// table is one sheet as Clean-up reads it, before its columns are typed.
type table struct {
	name   string
	header int // the header row's index
	cols   []Column
	raw    [][]cell // each data row, as wide as cols
	rowNo  []int    // each data row's number in Excel
	flat   bool     // a flat list, not a grouped notes table with a label on a group's first row only
}

// readTable reads one sheet as a table, or gives the Problem that stops it.
func readTable(f *excelize.File, name string, s Settings) (*table, error) {
	cells, err := f.GetRows(name, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, Problem{"lab.err.unreadable", []any{err.Error()}}
	}
	h := s.HeaderRow - 1
	if h < 0 {
		h = headerRow(cells)
	}
	if h < 0 || h+1 >= len(cells) {
		return nil, Problem{"lab.err.no-data", nil}
	}
	cols, err := columnsOf(cells[h])
	if err != nil {
		return nil, err
	}
	if len(cols) > MaxColumns {
		return nil, Problem{"lab.err.too-many-columns", []any{len(cols), MaxColumns}}
	}
	raw, rowNo := rawRows(f, name, cells, h, len(cols))
	if len(raw) == 0 {
		return nil, Problem{"lab.err.no-data", nil}
	}
	for ri, vals := range raw {
		if i := slices.IndexFunc(vals[len(cols):], func(c cell) bool { return c.v != "" }); i >= 0 {
			return nil, Problem{"lab.err.stray-cell", []any{rowNo[ri], vals[len(cols)+i].v, colLetter(len(cols) + i)}}
		}
		raw[ri] = vals[:len(cols)]
	}
	// ponytail: a blank first cell on any row reads as a grouped table; the Learner can pick the sheet instead.
	flat := !slices.ContainsFunc(raw, func(vals []cell) bool { return vals[0].v == "" })
	return &table{name, h, cols, raw, rowNo, flat}, nil
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

// addSheetColumn puts each row's sheet in a new first column of combined d: a Date column when every sheet's
// name reads as a date, else a Sheet column of names. It gives the change to list.
func addSheetColumn(d *Dataset, used []*table) Change {
	col, change := Column{"Date", Date}, Change{"lab.clean.combined-dates", []int{len(used)}}
	values := make([]any, len(used))
	for i, tb := range used {
		day, ok := sheetDate(tb.name)
		if !ok {
			col, change = Column{"Sheet", Text}, Change{"lab.clean.combined", []int{len(used)}}
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
