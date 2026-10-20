// Package lab is the Lab's pure logic: no database, no HTTP. Read turns an uploaded .xlsx into a
// Dataset for a Template; Generate turns a Template, column choices and a Dataset into the
// Dashboard's index.html and data.json. See ADR 0003.
package lab

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Type is what a column holds.
type Type string

const (
	Date   Type = "date" // stored as "2006-01-02"
	Number Type = "number"
	Text   Type = "text"
)

// Column is one column of a Dataset: its header in row 1 and the type read from its cells.
type Column struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

// Dataset is an uploaded spreadsheet as Cave reads it. Each row has one value per column:
// a string for Date ("2006-01-02") and Text, a float64 for Number, nil for an empty cell.
// It is stored as JSON, so json.Unmarshal gives back the same shape.
type Dataset struct {
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Errors is everything wrong with an upload, each in words a Learner can act on.
type Errors []string

func (e Errors) Error() string { return strings.Join(e, "; ") }

// Read reads the first sheet of an .xlsx file, row 1 being the column names, and checks it
// against the Template. A file Cave cannot use gives Errors.
// ponytail: basic checks only; #42 adds the row and size caps, Malaysian date and RM forms, per-cell errors.
func Read(r io.Reader, t *Template) (*Dataset, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, Errors{"This is not an Excel workbook. The Lab reads .xlsx files only: in Excel choose File, Save As, Excel Workbook (.xlsx)."}
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	cells, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, Errors{"The first sheet could not be read: " + err.Error()}
	}
	if len(cells) < 2 {
		return nil, Errors{"The first sheet needs column names in row 1 and at least one row of data below them."}
	}

	var errs Errors
	d := &Dataset{}
	seen := map[string]bool{}
	for i, name := range cells[0] {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			errs = append(errs, fmt.Sprintf("Column %s has no name in row 1.", colLetter(i)))
		case seen[strings.ToLower(name)]:
			errs = append(errs, fmt.Sprintf("Two columns are called %s. Give each column its own name in row 1.", name))
		}
		seen[strings.ToLower(name)] = true
		d.Columns = append(d.Columns, Column{Name: name})
	}
	if errs != nil {
		return nil, errs
	}

	dates := dateStyles{f: f, sheet: sheet, known: map[int]bool{}}
	n := len(d.Columns)
	var raw [][]string
	var isDate [][]bool
	for ri, row := range cells[1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue // a blank line carries no data
		}
		vals, flags := make([]string, n), make([]bool, n)
		for ci := range vals {
			if ci < len(row) {
				vals[ci] = strings.TrimSpace(row[ci])
				flags[ci] = vals[ci] != "" && dates.at(ci, ri+2)
			}
		}
		raw, isDate = append(raw, vals), append(isDate, flags)
	}

	for ci := range d.Columns {
		d.Columns[ci].Type = columnType(raw, isDate, ci)
	}
	for ri, vals := range raw {
		row := make([]any, n)
		for ci, v := range vals {
			if v == "" {
				continue
			}
			switch d.Columns[ci].Type {
			case Date:
				serial, _ := strconv.ParseFloat(v, 64)
				day, err := excelize.ExcelDateToTime(serial, false)
				if err != nil {
					errs = append(errs, fmt.Sprintf("Row %d, %s: not a date.", ri+2, d.Columns[ci].Name))
					continue
				}
				row[ci] = day.Format("2006-01-02")
			case Number:
				row[ci], _ = strconv.ParseFloat(v, 64)
			default:
				row[ci] = v
			}
		}
		d.Rows = append(d.Rows, row)
	}

	for _, b := range t.Blanks {
		if b.Required && !hasType(d.Columns, b.Type) {
			errs = append(errs, fmt.Sprintf("The %s Template needs a %s column for %s, and this file has none.", t.Name, b.Type, b.Label))
		}
	}
	if errs != nil {
		return nil, errs
	}
	return d, nil
}

// columnType is Date when every filled cell is a date-formatted number, Number when every one is a number, else Text.
func columnType(raw [][]string, isDate [][]bool, ci int) Type {
	allDates, allNumbers, filled := true, true, false
	for ri, vals := range raw {
		if vals[ci] == "" {
			continue
		}
		filled = true
		_, err := strconv.ParseFloat(vals[ci], 64)
		allNumbers = allNumbers && err == nil
		allDates = allDates && err == nil && isDate[ri][ci]
	}
	switch {
	case !filled:
		return Text
	case allDates:
		return Date
	case allNumbers:
		return Number
	}
	return Text
}

func hasType(cols []Column, t Type) bool {
	for _, c := range cols {
		if c.Type == t {
			return true
		}
	}
	return false
}

func colLetter(i int) string {
	name, _ := excelize.ColumnNumberToName(i + 1)
	return name
}

// dateStyles tells whether a cell's number format shows a date, caching the answer per style.
type dateStyles struct {
	f     *excelize.File
	sheet string
	known map[int]bool
}

var quotedOrBracketed = regexp.MustCompile(`"[^"]*"|\[[^\]]*\]`)

func (s dateStyles) at(col, row int) bool {
	cell, _ := excelize.CoordinatesToCellName(col+1, row)
	idx, err := s.f.GetCellStyle(s.sheet, cell)
	if err != nil {
		return false
	}
	if is, ok := s.known[idx]; ok {
		return is
	}
	is := false
	if style, err := s.f.GetStyle(idx); err == nil {
		switch {
		case style.CustomNumFmt != nil:
			// ponytail: a custom format with a y or d outside quotes and brackets is a date; minutes-only formats are not.
			code := strings.ToLower(quotedOrBracketed.ReplaceAllString(*style.CustomNumFmt, ""))
			is = strings.ContainsAny(code, "yd")
		default:
			n := style.NumFmt
			is = (n >= 14 && n <= 17) || n == 22 || (n >= 27 && n <= 36) || (n >= 50 && n <= 58)
		}
	}
	s.known[idx] = is
	return is
}
