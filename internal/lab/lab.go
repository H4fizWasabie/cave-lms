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
	"time"

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
	Columns    []Column `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowNumbers []int    `json:"rowNumbers,omitempty"` // each row's number in Excel, as blank rows are skipped
}

// rowNumber is row i's number in Excel.
func (d *Dataset) rowNumber(i int) int {
	if i < len(d.RowNumbers) {
		return d.RowNumbers[i]
	}
	return i + 2
}

// emptyCell reports a blank cell in a column a required blank uses.
func emptyCell(row int, col string) string {
	return fmt.Sprintf("Row %d, %s: empty, but this column is required.", row, col)
}

// Errors is everything wrong with an upload, each in words a Learner can act on.
type Errors []string

func (e Errors) Error() string { return strings.Join(e, "; ") }

// MaxRows is the most data rows the Lab reads from one file.
const MaxRows = 5000

// WhyXLSX tells a Learner why the Lab takes .xlsx files only.
const WhyXLSX = "The Lab reads Excel workbooks only. A CSV file stores everything as plain text, so dates and RM amounts can change on the way. In Excel choose File, Save As, Excel Workbook (.xlsx) and upload that."

// Read reads the first sheet of an .xlsx file, row 1 being the column names, and checks it
// against the Template. A column named like one of a blank's guesses is read as that blank's
// type; any other column's type comes from its cells. Dates may be real Excel dates,
// day/month/year text or Excel date serial numbers; numbers may carry RM and thousands commas.
// A file Cave cannot use gives Errors, every problem listed in row order. Only fully blank rows
// are skipped.
func Read(r io.Reader, t *Template) (*Dataset, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, Errors{"This is not an .xlsx file. " + WhyXLSX}
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
	var raw [][]cell // every row with data, as wide as the widest row
	var rowNo []int  // its row number in Excel
	for ri, row := range cells[1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue // a blank line carries no data
		}
		vals := make([]cell, max(len(row), len(d.Columns)))
		for ci, v := range row {
			v = strings.TrimSpace(v)
			vals[ci] = cell{v, v != "" && dates.at(ci, ri+2)}
		}
		raw, rowNo = append(raw, vals), append(rowNo, ri+2)
	}
	if len(raw) > MaxRows {
		return nil, Errors{fmt.Sprintf("The file has %s rows. The Lab reads up to %s rows. Delete some rows and upload again.", thousands(len(raw)), thousands(MaxRows))}
	}

	for ci := range d.Columns {
		typ, named := t.blankType(d.Columns[ci].Name)
		if !named {
			typ = columnType(raw, ci)
		}
		d.Columns[ci].Type = typ
	}
	for _, b := range t.Blanks {
		if b.Required && !hasType(d.Columns, b.Type) {
			errs = append(errs, fmt.Sprintf("Column %s is missing. The %s Template needs it for %s, a column of %s. Did you rename it? Rename it back in row 1 and upload again.",
				b.SampleName(), t.Name, b.Label, typeWords[b.Type]))
		}
	}
	required, guess := map[string]bool{}, t.Guess(d)
	for _, b := range t.Blanks {
		if col, ok := guess[b.Key]; ok && b.Required {
			required[col] = true
		}
	}

	for ri, vals := range raw {
		row := make([]any, len(d.Columns))
		for ci, c := range vals {
			if ci >= len(d.Columns) {
				if c.v != "" {
					errs = append(errs, fmt.Sprintf("Row %d has %q in column %s, which has no name in row 1. Name the column or clear the cell.", rowNo[ri], c.v, colLetter(ci)))
				}
				continue
			}
			col := d.Columns[ci]
			if c.v == "" {
				if required[col.Name] {
					errs = append(errs, emptyCell(rowNo[ri], col.Name))
				}
				continue
			}
			v, problem := parse(c, col.Type)
			if problem != "" {
				errs = append(errs, fmt.Sprintf("Row %d, %s: %s", rowNo[ri], col.Name, problem))
			}
			row[ci] = v
		}
		d.Rows = append(d.Rows, row)
	}
	if errs != nil {
		return nil, errs
	}
	d.RowNumbers = rowNo
	return d, nil
}

// cell is a raw cell value; date is true when Excel shows it as a date.
type cell struct {
	v    string
	date bool
}

var typeWords = map[Type]string{Date: "dates", Number: "numbers", Text: "text"}

// parse reads a filled cell as typ, or says why it cannot.
func parse(c cell, typ Type) (any, string) {
	switch typ {
	case Date:
		if day, ok := textDate(c.v); ok {
			return day, ""
		}
		// A real Excel date and a date serial number are both a count of days.
		if n, err := strconv.ParseFloat(c.v, 64); err == nil && n >= 1 && n < 2958466 {
			if day, err := excelize.ExcelDateToTime(n, false); err == nil {
				return day.Format("2006-01-02"), ""
			}
		}
		return nil, fmt.Sprintf("not a date (%q). Type it as day/month/year, like 05/11/2026.", c.v)
	case Number:
		if n, ok := number(c.v); ok {
			return n, ""
		}
		return nil, fmt.Sprintf("not a number (%q). Type it as a plain number, like 200 or RM1,200.00.", c.v)
	}
	return c.v, ""
}

// columnType is Date when every filled cell is a date, Number when every one is a number, else Text.
func columnType(raw [][]cell, ci int) Type {
	allDates, allNumbers, filled := true, true, false
	for _, vals := range raw {
		c := vals[ci]
		if c.v == "" {
			continue
		}
		filled = true
		_, isNumber := number(c.v)
		_, isDate := textDate(c.v)
		allNumbers = allNumbers && isNumber
		allDates = allDates && (isDate || c.date && isNumber)
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

var (
	amount  = regexp.MustCompile(`(?i)^-?(rm)?\s*-?(\d{1,3}(,\d{3})+|\d*)(\.\d+)?(e[-+]?\d+)?$`)
	ringgit = regexp.MustCompile(`(?i)rm|,|\s`)
	dmy     = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$`)
)

// number reads 1200, 1,200, RM1,200.00, RM 50 and -RM12.50.
func number(s string) (float64, bool) {
	if !amount.MatchString(s) {
		return 0, false
	}
	n, err := strconv.ParseFloat(ringgit.ReplaceAllString(s, ""), 64)
	return n, err == nil
}

// textDate reads day/month/year text, so 05/11/2026 is 5 November 2026, as "2006-01-02".
func textDate(s string) (string, bool) {
	m := dmy.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	day, err := time.Parse("2/1/2006", m[1]+"/"+m[2]+"/"+m[3])
	if err != nil {
		return "", false
	}
	return day.Format("2006-01-02"), true
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
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
