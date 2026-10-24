package lab

import (
	"io"
	"slices"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/xuri/excelize/v2"
)

// Settings are the Learner's choices for one Work Dashboard upload. The zero value lets Cave choose everything.
type Settings struct {
	Sheet     string // the sheet to read; empty, or a sheet not in the file, reads the first
	HeaderRow int    // the header's row number in Excel; 0 lets Cave find it
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

// Clean is Clean-up, a Work Dashboard's way of reading an upload: it finds the header row below any title
// rows, reads each column's type from its cells, and guesses the Template's blanks. A file it cannot read
// gives one Problem.
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
	if h < 0 || h+1 >= len(cells) {
		return nil, Problem{"lab.err.no-data", nil}
	}

	d := &Dataset{}
	if d.Columns, err = columnsOf(cells[h]); err != nil {
		return nil, err
	}
	raw, rowNo := rawRows(f, sheet, cells, h, len(d.Columns))
	if len(raw) == 0 {
		return nil, Problem{"lab.err.no-data", nil}
	}
	if len(raw) > MaxRows {
		return nil, Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxRows), thousands(rowNo[MaxRows])}}
	}
	for ci := range d.Columns {
		d.Columns[ci].Type = columnType(raw, ci)
	}
	for ri, vals := range raw {
		if i := slices.IndexFunc(vals[len(d.Columns):], func(c cell) bool { return c.v != "" }); i >= 0 {
			return nil, Problem{"lab.err.stray-cell", []any{rowNo[ri], vals[len(d.Columns)+i].v, colLetter(len(d.Columns) + i)}}
		}
		row := make([]any, len(d.Columns))
		for ci, c := range vals[:len(d.Columns)] {
			if c.v != "" {
				row[ci], _ = parse(c, d.Columns[ci].Type)
			}
		}
		d.Rows = append(d.Rows, row)
	}
	d.RowNumbers = rowNo

	res := &CleanUp{Sheets: []Sheet{{sheet, len(d.Rows)}}, Used: []string{sheet}, HeaderRow: h + 1, Dataset: d}
	switch {
	case h == 1:
		res.Changes = append(res.Changes, Change{"lab.clean.title-row", []int{h + 1}})
	case h > 1:
		res.Changes = append(res.Changes, Change{"lab.clean.title-rows", []int{h, h + 1}})
	}
	if t.Build == nil {
		res.Guess = t.Guess(d)
	}
	return res, nil
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
