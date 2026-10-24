package lab

import (
	"cmp"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

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
	Month        int      `json:"-"`                      // the month, 1 to 12, of every days-across grid; 0 lets Cave find it
	Year         int      `json:"-"`                      // the grids' year, with Month; 0 is this year
}

// Sheet is one sheet of an uploaded workbook.
type Sheet struct {
	Name       string `json:"name"`
	Rows       int    `json:"rows"`                 // data rows below its header; 0 if it cannot be read
	Readable   bool   `json:"readable,omitempty"`   // Clean-up can read it as a table, so it can be picked
	CanCombine bool   `json:"canCombine,omitempty"` // it has the picked sheet's columns, so it can be ticked to join it
	Reason     string `json:"reason,omitempty"`     // a text catalog key saying why it cannot join the picked sheet
}

// MaxCleanRows is the most rows Clean-up keeps and MaxColumns the most columns, both counted on the Dataset as
// made: combined, with empty columns dropped and the Sheet or Date column added.
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

// Clean is Clean-up, a Work Dashboard's way of reading an upload. Its rules run as steps, in order: read every
// sheet on its own (readTable: header row, merged cells, cut-off, totals at the bottom, empty columns), pick one
// sheet (s.Sheet, else the first flat list, else the first readable sheet), combine the sheets with the same
// columns into one Dataset, type each column from its cells (- as 0, more date forms), and guess the Template's
// blanks by name. Every step that changes something lists it in Changes. A file it cannot read gives one Problem.
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
		return nil, onSheet(Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxCleanRows), thousands(d.RowNumbers[MaxCleanRows])}}, base.name)
	}
	res.HeaderRow = base.header + 1
	for _, tb := range used {
		res.addChanges(tb.changes)
	}
	res.Dataset = typed(d, raw, d.RowNumbers, res)
	if slices.ContainsFunc(used, func(tb *table) bool { return tb.days > 0 }) {
		res.Changes = append(res.Changes, Change{"lab.clean.unpivoted", []int{len(raw), int(month), year}})
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
	if t.Build == nil {
		res.Guess = t.guessWork(d)
	}
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
	changes []Change // what reading the sheet changed

}

// readTable reads one sheet as a table, or gives the Problem that stops it. Its rules run in order: find the
// header row below any title rows (or take the Learner's, of one or two rows), fill merged data cells down,
// stop at the first fully blank row and column, then drop totals and labels at the bottom and columns with no data.
func readTable(f *excelize.File, name string, s Settings) (*table, error) {
	cells, err := f.GetRows(name, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, Problem{"lab.err.sheet-unreadable", []any{err.Error()}}
	}
	h := s.HeaderRow - 1
	if h < 0 {
		if h = headerRow(cells); s.TwoRowHeader && groupsAbove(f, name, cells, h) {
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
			tb.changes = res.Changes
		}
		return tb, err
	}

	cells = fillDown(f, name, cells, first, res)
	cells = cutOff(cells, h, first, res)
	header := cells[h]
	if s.TwoRowHeader {
		header = joinHeader(f, name, cells, h, res)
	}
	raw, rowNo := rawRows(f, name, cells, first-1, len(header))
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
	return &table{name: name, header: h, cols: cols, raw: raw, rowNo: rowNo, flat: flat, changes: res.Changes}, nil
}

// groupsAbove reports whether header row h, found by headerRow, is the bottom row of a two-row header whose
// top row holds only a few group names: data starts right below h, and the row above has two names or more, or
// a name merged across columns other than from the first (a title merged across the sheet starts at A).
func groupsAbove(f *excelize.File, sheet string, cells [][]string, h int) bool {
	if h < 1 || h+1 >= len(cells) || !slices.ContainsFunc(cells[h+1], func(v string) bool { _, ok := number(strings.TrimSpace(v)); return ok }) {
		return false
	}
	names := 0
	for _, v := range cells[h-1] {
		if strings.TrimSpace(v) != "" {
			names++
		}
	}
	return names >= 2 || slices.ContainsFunc(merged(f, sheet), func(m [4]int) bool { return m[0] == h-1 && m[1] > 0 && m[3] > m[1] })
}

// likeliest is the index of the sheet whose Problem to give when no sheet reads: the one the Learner picked,
// else the one with the most filled rows, likely the table rather than a cover or notes sheet.
func likeliest(f *excelize.File, names []string, picked string) int {
	if i := slices.Index(names, picked); i >= 0 {
		return i
	}
	best, most := 0, -1
	for i, name := range names {
		cells, _ := f.GetRows(name)
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

// counted are the changes whose one number counts cells or columns, so the sheets' counts add up.
var counted = []string{"lab.clean.merged-down", "lab.clean.empty-columns"}

// addChanges lists one used sheet's changes: a counted change adds to the same change of a sheet before it,
// and any other is listed once however many sheets made it.
func (res *CleanUp) addChanges(changes []Change) {
	for _, c := range changes {
		i := slices.IndexFunc(res.Changes, func(o Change) bool {
			return o.Key == c.Key && (slices.Contains(counted, c.Key) || slices.Equal(o.Args, c.Args))
		})
		switch {
		case i < 0:
			res.Changes = append(res.Changes, Change{c.Key, slices.Clone(c.Args)})
		case slices.Contains(counted, c.Key):
			res.Changes[i].Args[0] += c.Args[0]
		}
	}
}

// change lists one thing Clean-up changed.
func (res *CleanUp) change(key string, args ...int) {
	res.Changes = append(res.Changes, Change{key, args})
}

// merged is each merged range of the sheet as 0-based [top row, left column, bottom row, right column].
func merged(f *excelize.File, sheet string) [][4]int {
	ranges, _ := f.GetMergeCells(sheet)
	var out [][4]int
	for _, m := range ranges {
		c1, r1, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		c2, r2, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 == nil && err2 == nil {
			out = append(out, [4]int{r1 - 1, c1 - 1, r2 - 1, c2 - 1})
		}
	}
	return out
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
func fillDown(f *excelize.File, sheet string, cells [][]string, first int, res *CleanUp) [][]string {
	n := 0
	for _, m := range merged(f, sheet) {
		top, left, bottom, right := m[0], m[1], m[2], m[3]
		if top < first || bottom == top || top >= len(cells) || left >= len(cells[top]) {
			continue
		}
		v := cells[top][left]
		for r := top; r <= bottom; r++ {
			for c := left; c <= right; c++ {
				if (r != top || c != left) && r < len(cells) {
					set(cells, r, c, v)
					n++
				}
			}
		}
	}
	if n > 0 {
		res.change("lab.clean.merged-down", n)
	}
	return cells
}

// joinHeader names each column from header rows h and h+1 joined, as "Jualan Tunai". A merged cell in either
// row is filled across first, so a group name over several columns starts each of their names; a name merged
// down both rows is used once.
func joinHeader(f *excelize.File, sheet string, cells [][]string, h int, res *CleanUp) []string {
	top, bottom := slices.Clone(cells[h]), slices.Clone(cells[h+1])
	for _, m := range merged(f, sheet) {
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
// from h on is as wide as the table.
func cutOff(cells [][]string, h, first int, res *CleanUp) [][]string {
	filled := func(r, c int) bool { return c < len(cells[r]) && strings.TrimSpace(cells[r][c]) != "" }
	blankRow := func(r, from, to int) bool {
		for c := from; c < to; c++ {
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
	for c := max(slices.IndexFunc(cells[h], func(v string) bool { return strings.TrimSpace(v) != "" }), 0); c < widest; c++ {
		if !slices.ContainsFunc(cells[h:rows], func(row []string) bool { return c < len(row) && strings.TrimSpace(row[c]) != "" }) {
			width = c
			break
		}
	}
	rows = end(width)

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
	return cells
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

// typed fills d's rows from raw, each column typed by its cells. Unlike Read, a lone - in a Number column reads
// as 0, and dates may be written in more ways (see otherDate); both are listed as changes.
func typed(d *Dataset, raw [][]cell, rowNo []int, res *CleanUp) *Dataset {
	dashes, others := 0, 0
	for ci := range d.Columns {
		d.Columns[ci].Type = workType(raw, ci)
	}
	for _, vals := range raw {
		row := make([]any, len(d.Columns))
		for ci, c := range vals[:len(d.Columns)] {
			typ := d.Columns[ci].Type
			if c.v == "" {
				continue
			}
			if c.v == "-" && typ == Number {
				row[ci] = 0.0
				dashes++
				continue
			}
			if day, ok := otherDate(c.v); ok && typ == Date {
				row[ci] = day
				others++
				continue
			}
			row[ci], _ = parse(c, typ)
		}
		d.Rows = append(d.Rows, row)
	}
	d.RowNumbers = rowNo
	if dashes > 0 {
		res.change("lab.clean.dash-zero", dashes)
	}
	if others > 0 {
		res.change("lab.clean.other-dates", others)
	}
	return d
}

// workType is a column's type as Clean-up reads it: as columnType, but a lone - fits a Number column that has
// at least one real number, and otherDate's forms are dates.
func workType(raw [][]cell, ci int) Type {
	allDates, allNumbers, numbers, filled := true, true, false, false
	for _, vals := range raw {
		c := vals[ci]
		if c.v == "" {
			continue
		}
		filled = true
		if c.v == "-" {
			allDates = false
			continue
		}
		_, isNumber := number(c.v)
		_, isDate := textDate(c.v)
		_, isOther := otherDate(c.v)
		numbers = numbers || isNumber
		allNumbers = allNumbers && isNumber
		allDates = allDates && (isDate || isOther || c.date && isNumber)
	}
	switch {
	case !filled:
		return Text
	case allDates:
		return Date
	case allNumbers && numbers:
		return Number
	}
	return Text
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
