// Package lab is the Lab's pure logic: no database, no HTTP. Clean turns an uploaded .xlsx into a Dataset; Build
// makes a built Dashboard's blocks from it by the General rules; Page and DataJSON turn the blocks and the Dataset
// into the Dashboard's index.html and data.json. See ADR 0019.
package lab

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"slices"
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

// MaxUnzipped is the most an .xlsx may unpack to. A 5,000-row workbook unpacks to a few MB; without a cap, a
// 10 MB upload of zeros or repeated cells can unpack to gigabytes and take the server down.
const MaxUnzipped = 50 << 20

// maxSheetCells is the most cells the Lab holds from one sheet, counted as excelize gives them: each row padded
// with "" out to its last filled cell. MaxCleanRows rows of MaxColumns columns fit; a small file with one cell
// far out on every row, padded to millions of cells, does not.
const maxSheetCells = MaxCleanRows * MaxColumns

// sheetRows is excelize's GetRows read row by row, so a sheet past maxSheetCells is refused before it is held whole.
func sheetRows(f *excelize.File, name string, opts ...excelize.Options) ([][]string, error) {
	rows, err := f.Rows(name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cells [][]string
	kept, n := 0, 0 // rows up to the last with a cell, as GetRows gives; cells read
	for rows.Next() {
		row, err := rows.Columns(opts...)
		if err != nil {
			break // as GetRows does
		}
		if n += len(row); n > maxSheetCells {
			return nil, Problem{"lab.err.too-many-cells", nil}
		}
		if cells = append(cells, row); len(row) > 0 {
			kept = len(cells)
		}
	}
	return cells[:kept], nil
}

// sharedText is how many bytes of text the cells that point at a shared string come to, each given its own copy
// as excelize gives them; it stops counting once past limit. One long string shared by thousands of cells is a few
// KB in the file and gigabytes in memory. Every part of the archive is read, not just the worksheets, as a workbook
// can keep a sheet anywhere; a part that is not XML ends its own read at its first bad byte.
// ponytail: excelize lengthens a Strict namespace name written in the text by a quarter; the count leaves it out.
func sharedText(f *excelize.File, z *zip.Reader, limit int) int {
	decoder := func(entry *zip.File) (*xml.Decoder, io.Closer) {
		r, err := entry.Open()
		if err != nil {
			return nil, nil
		}
		d := xml.NewDecoder(r)
		d.CharsetReader = f.CharsetReader // as excelize reads a part saved in another encoding
		return d, r
	}
	isStrings := func(entry *zip.File) bool {
		return strings.EqualFold(strings.ReplaceAll(entry.Name, "\\", "/"), "xl/sharedStrings.xml")
	}
	// Each shared string's length, at its most: excelize may read any shared strings part the archive repeats, and
	// counts either the strings right under the root or, for a part kept on disk, every string at any depth.
	var sizes []int
	most := func(i, n int) {
		if i >= len(sizes) {
			sizes = append(sizes, make([]int, i+1-len(sizes))...)
		}
		sizes[i] = max(sizes[i], n)
	}
	for _, entry := range z.File {
		if !isStrings(entry) {
			continue
		}
		d, r := decoder(entry)
		if d == nil {
			continue
		}
		depth, top, all := 0, 0, 0 // in the part; the strings counted right under its root, and at any depth
		for {
			tok, err := d.Token()
			if err != nil {
				break
			}
			switch el := tok.(type) {
			case xml.EndElement:
				depth--
			case xml.StartElement:
				if el.Name.Local != "si" {
					depth++
					continue
				}
				var si struct {
					T string `xml:"t"`
					R []struct {
						T string `xml:"t"`
					} `xml:"r"`
				}
				d.DecodeElement(&si, &el)
				n := len(si.T)
				for _, run := range si.R {
					n += len(run.T)
				}
				if most(all, n); depth == 1 {
					most(top, n)
					top++
				}
				all++
			}
		}
		r.Close()
	}
	if len(sizes) == 0 {
		return 0
	}
	total := 0
	for _, entry := range z.File {
		if total > limit {
			break
		}
		if isStrings(entry) {
			continue
		}
		d, r := decoder(entry)
		if d == nil {
			continue
		}
		for total <= limit {
			tok, err := d.Token()
			if err != nil {
				break
			}
			el, ok := tok.(xml.StartElement)
			if !ok || el.Name.Local != "c" || !slices.ContainsFunc(el.Attr, func(a xml.Attr) bool { return a.Name.Local == "t" && a.Value == "s" }) {
				continue
			}
			var c struct {
				V string `xml:"v"`
			}
			d.DecodeElement(&c, &el)
			if i, err := strconv.Atoi(strings.TrimSpace(c.V)); err == nil && i >= 0 && i < len(sizes) {
				total += sizes[i]
			}
		}
		r.Close()
	}
	return total
}

// oldExcel is how an .xls file starts: the signature of the compound file Excel 97-2003 saved.
var oldExcel = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// openWorkbook opens an .xlsx file within the size limits; a file that is not one gives a Problem.
func openWorkbook(r io.Reader) (*excelize.File, error) {
	file, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	switch {
	case err != nil && bytes.HasPrefix(file, oldExcel):
		return nil, Problem{"lab.err.xls", nil}
	case err != nil:
		return nil, Problem{"lab.err.not-workbook", nil}
	}
	var unzipped uint64
	for _, entry := range z.File {
		unzipped += entry.UncompressedSize64
	}
	if unzipped > MaxUnzipped {
		return nil, Problem{"lab.err.too-big-unzipped", nil}
	}
	// The limit holds while excelize unpacks too: archive/zip refuses an entry longer than its header claims. Every
	// part stays in memory, in f.Pkg, so readLayout can read a sheet's XML however big.
	f, err := excelize.OpenReader(bytes.NewReader(file), excelize.Options{UnzipSizeLimit: MaxUnzipped, UnzipXMLSizeLimit: MaxUnzipped})
	if err != nil {
		return nil, Problem{"lab.err.not-workbook", nil}
	}
	if sharedText(f, z, MaxUnzipped) > MaxUnzipped {
		f.Close()
		return nil, Problem{"lab.err.too-big-unzipped", nil}
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
func rawRows(l layout, cells [][]string, h, width int) (raw [][]cell, rowNo []int) {
	for ri := h + 1; ri < len(cells); ri++ {
		row := cells[ri]
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue // a blank line carries no data
		}
		vals := make([]cell, max(len(row), width))
		for ci, v := range row {
			v = strings.TrimSpace(v)
			vals[ci] = cell{v, v != "" && l.date(ri, ci)}
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

// parse reads a filled cell as typ; false when it does not fit.
func parse(c cell, typ Type) (any, bool) {
	switch typ {
	case Date:
		if day, ok := textDate(c.v); ok {
			return day, true
		}
		// A real Excel date and a date serial number are both a count of days.
		if n, err := strconv.ParseFloat(c.v, 64); err == nil && n >= 1 && n < 2958466 {
			if day, err := excelize.ExcelDateToTime(n, false); err == nil {
				return day.Format("2006-01-02"), true
			}
		}
		return nil, false
	case Number:
		if n, ok := number(c.v); ok {
			return n, true
		}
		return nil, false
	}
	return c.v, true
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

func colLetter(i int) string {
	name, _ := excelize.ColumnNumberToName(i + 1)
	return name
}

var quotedOrBracketed = regexp.MustCompile(`"[^"]*"|\[[^\]]*\]`)

// dateFormat reports whether style idx's number format shows a date.
func dateFormat(f *excelize.File, idx int) bool {
	style, err := f.GetStyle(idx)
	switch {
	case err != nil:
		return false
	case style.CustomNumFmt != nil:
		// ponytail: a custom format with a y or d outside quotes and brackets is a date; minutes-only formats are not.
		code := strings.ToLower(quotedOrBracketed.ReplaceAllString(*style.CustomNumFmt, ""))
		return strings.ContainsAny(code, "yd")
	}
	n := style.NumFmt
	return (n >= 14 && n <= 17) || n == 22 || (n >= 27 && n <= 36) || (n >= 50 && n <= 58)
}
