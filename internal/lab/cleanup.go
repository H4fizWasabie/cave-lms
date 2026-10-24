package lab

import (
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/xuri/excelize/v2"
)

// Settings are the Learner's choices for one Work Dashboard upload. The zero value lets Cave choose everything.
type Settings struct {
	Sheet     string // the sheet to read; empty, or a sheet not in the file, reads the first
	HeaderRow int    // the header's row number in Excel; 0 lets Cave find it
	// TwoRowHeader joins the header row and the row below it into one name per column.
	TwoRowHeader bool
}

// Sheet is one sheet of an uploaded workbook.
type Sheet struct {
	Name string
	Rows int // data rows below its header
}

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
	Sheets    []Sheet
	Used      []string // the sheets that went into the Dataset
	HeaderRow int      // the header's row number in Excel
	Dataset   *Dataset
	Changes   []Change
	Guess     Choices
}

// Clean is Clean-up, a Work Dashboard's way of reading an upload. Its rules run as steps, in order: find the
// header row below any title rows (or take the Learner's, of one or two rows), fill merged data cells down,
// stop at the first fully blank row and column, drop totals and labels at the bottom and columns with no data,
// then type each column from its cells (- as 0, more date forms) and guess the Template's blanks by name.
// Every step that changes something lists it in Changes. A file it cannot read gives one Problem.
func Clean(r io.Reader, t *Template, s Settings) (*CleanUp, error) {
	f, err := openWorkbook(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	if slices.Contains(f.GetSheetList(), s.Sheet) {
		sheet = s.Sheet
	}
	cells, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, Problem{"lab.err.unreadable", []any{err.Error()}}
	}
	h := s.HeaderRow - 1
	if h < 0 {
		h = headerRow(cells)
	}
	first := h + 1 // the first data row
	if s.TwoRowHeader {
		first++
	}
	if h < 0 || first >= len(cells) {
		return nil, Problem{"lab.err.no-data", nil}
	}

	res := &CleanUp{Used: []string{sheet}, HeaderRow: h + 1}
	if s.HeaderRow > 0 {
		res.change("lab.clean.header-chosen", h+1)
	}
	switch {
	case h == 1:
		res.change("lab.clean.title-row", h+1)
	case h > 1:
		res.change("lab.clean.title-rows", h, h+1)
	}

	cells = fillDown(f, sheet, cells, first, res)
	cells = cutOff(cells, h, first, res)
	header := cells[h]
	if s.TwoRowHeader {
		header = joinHeader(f, sheet, cells, h, res)
	}
	raw, rowNo := rawRows(f, sheet, cells, first-1, len(header))
	if len(raw) == 0 {
		return nil, Problem{"lab.err.no-data", nil}
	}
	if len(raw) > MaxRows {
		return nil, Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxRows), thousands(rowNo[MaxRows])}}
	}
	raw, rowNo = dropBottom(raw, rowNo, res)
	header, raw = dropEmpty(header, raw, res)

	d := &Dataset{}
	if d.Columns, err = columnsOf(header); err != nil {
		return nil, err
	}
	res.Dataset = typed(d, raw, rowNo, res)
	res.Sheets = []Sheet{{sheet, len(d.Rows)}}
	if t.Build == nil {
		res.Guess = t.guessWork(d)
	}
	return res, nil
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
// only a label (one filled cell, not a number, in a table of two columns or more). Rows above the last data
// row are kept, totals or not: only the bottom can be told apart from data.
// ponytail: a last data row with one filled cell reads as a label; the Learner fills the cell in Excel.
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
		if !total && (filled != 1 || width < 2 || isNumber) {
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
