// Package lab is the Lab's pure logic: no database, no HTTP. Read turns an uploaded .xlsx into a
// Dataset for a Template; Generate turns a Template, column choices and a Dataset into the
// Dashboard's index.html and data.json. See ADR 0003.
package lab

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
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
func emptyCell(row int, col string) Problem {
	return Problem{"lab.err.empty-cell", []any{row, col}}
}

// Problem is what is wrong with an upload or a column choice, as a text catalog key and the
// values it fills in, so the page can word it in the Learner's language. A value that is itself
// a Problem is worded too. As an error it reads in English, for logs.
//
// A file Cave cannot read gives one Problem, not a list: the likely cause and, where a row is to
// blame, the first such row's number in Excel, so the Learner knows what to fix and where.
type Problem struct {
	Key  string
	Args []any
}

// In words p in lang.
func (p Problem) In(lang string) string {
	args := make([]any, len(p.Args))
	for i, a := range p.Args {
		if inner, ok := a.(Problem); ok {
			a = inner.In(lang)
		}
		args[i] = a
	}
	return text.T(lang, p.Key, args...)
}

func (p Problem) Error() string { return p.In("en") }

// typeWord is typ as a word, like "number".
func typeWord(typ Type) Problem { return Problem{"lab.type." + string(typ), nil} }

// MaxRows is the most data rows the Lab reads from one file.
const MaxRows = 5000

// MaxUnzipped is the most an .xlsx may unpack to. A 5,000-row workbook unpacks to a few MB; without a cap, a
// 10 MB upload of zeros or repeated cells can unpack to gigabytes and take the server down.
const MaxUnzipped = 50 << 20

// Read reads the first sheet of an .xlsx file, row 1 being the column names, and checks it
// against the Template. A column named like one of a blank's guesses is read as that blank's
// type; any other column's type comes from its cells. Dates may be real Excel dates,
// day/month/year text or Excel date serial numbers; numbers may carry RM and thousands commas.
// A file Cave cannot use gives one Problem, the first in row order; failing to read r at all gives
// that plain error. Only fully blank rows are skipped.
func Read(r io.Reader, t *Template) (*Dataset, error) {
	f, err := openWorkbook(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	cells, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, Problem{"lab.err.unreadable", []any{err.Error()}}
	}
	if len(cells) < 2 {
		return nil, Problem{"lab.err.no-data", nil}
	}

	d := &Dataset{}
	if d.Columns, err = columnsOf(cells[0]); err != nil {
		return nil, err
	}
	raw, rowNo := rawRows(f, sheet, cells, 0, len(d.Columns))
	if len(raw) > MaxRows {
		return nil, Problem{"lab.err.too-many-rows", []any{thousands(len(raw)), thousands(MaxRows), thousands(rowNo[MaxRows])}}
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
			return nil, Problem{"lab.err.missing-column", []any{b.SampleName(), t.Name, b.Label, Problem{"lab.types." + string(b.Type), nil}}}
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
					return nil, Problem{"lab.err.stray-cell", []any{rowNo[ri], c.v, colLetter(ci)}}
				}
				continue
			}
			col := d.Columns[ci]
			if c.v == "" {
				if required[col.Name] {
					return nil, emptyCell(rowNo[ri], col.Name)
				}
				continue
			}
			v, problem := parse(c, col.Type)
			if problem != "" {
				return nil, Problem{problem, []any{rowNo[ri], col.Name, c.v}}
			}
			row[ci] = v
		}
		d.Rows = append(d.Rows, row)
	}
	d.RowNumbers = rowNo
	return d, nil
}

// openWorkbook opens an .xlsx file within the size limits; a file that is not one gives a Problem.
func openWorkbook(r io.Reader) (*excelize.File, error) {
	file, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return nil, Problem{"lab.err.not-workbook", nil}
	}
	var unzipped uint64
	for _, entry := range z.File {
		unzipped += entry.UncompressedSize64
	}
	if unzipped > MaxUnzipped {
		return nil, Problem{"lab.err.too-big-unzipped", nil}
	}
	// The limit holds while excelize unpacks too: archive/zip refuses an entry longer than its header claims.
	f, err := excelize.OpenReader(bytes.NewReader(file), excelize.Options{UnzipSizeLimit: MaxUnzipped})
	if err != nil {
		return nil, Problem{"lab.err.not-workbook", nil}
	}
	return f, nil
}

// columnsOf names the columns from a header row; the first blank or repeated name is a Problem. Types come later.
func columnsOf(header []string) ([]Column, error) {
	var cols []Column
	seen := map[string]bool{}
	for i, name := range header {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return nil, Problem{"lab.err.unnamed-column", []any{colLetter(i)}}
		case seen[strings.ToLower(name)]:
			return nil, Problem{"lab.err.same-name", []any{name}}
		}
		seen[strings.ToLower(name)] = true
		cols = append(cols, Column{Name: name})
	}
	return cols, nil
}

// rawRows is every row with data below the header at cells[h], as wide as the widest row and at least width,
// with its row number in Excel. Fully blank rows are skipped.
func rawRows(f *excelize.File, sheet string, cells [][]string, h, width int) (raw [][]cell, rowNo []int) {
	dates := dateStyles{f: f, sheet: sheet, known: map[int]bool{}}
	for ri := h + 1; ri < len(cells); ri++ {
		row := cells[ri]
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue // a blank line carries no data
		}
		vals := make([]cell, max(len(row), width))
		for ci, v := range row {
			v = strings.TrimSpace(v)
			vals[ci] = cell{v, v != "" && dates.at(ci, ri+1)}
		}
		raw, rowNo = append(raw, vals), append(rowNo, ri+1)
	}
	return raw, rowNo
}

// cell is a raw cell value; date is true when Excel shows it as a date.
type cell struct {
	v    string
	date bool
}

// parse reads a filled cell as typ, or gives the catalog key that says why it cannot.
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
		return nil, "lab.err.not-date"
	case Number:
		if n, ok := number(c.v); ok {
			return n, ""
		}
		return nil, "lab.err.not-number"
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
