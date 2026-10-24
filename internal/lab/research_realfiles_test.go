package lab_test

// Research harness for issue #207: runs lab.Read on real spreadsheets kept outside the repo.
// Skipped unless LAB_SAMPLES names the folder. Prints shapes and error kinds, never cell values.
//
//	LAB_SAMPLES=~/cave-lab-samples go test ./internal/lab/ -run TestResearchRealFiles -v

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/xuri/excelize/v2"
)

var (
	ampm      = regexp.MustCompile(`(?i)\d{1,2}:\d{2}(:\d{2})?\s*[ap]m`)
	totalWord = regexp.MustCompile(`(?i)^(grand\s+)?(sub\s*-?\s*)?(total|jumlah|summary)\b`)
	numText   = regexp.MustCompile(`^-?(RM\s*)?[\d,]+(\.\d+)?%?$`)
	dateText  = regexp.MustCompile(`^\d{1,4}[/.-]\d{1,2}[/.-]\d{1,4}`)
)

func TestResearchRealFiles(t *testing.T) {
	dir := os.Getenv("LAB_SAMPLES")
	if dir == "" {
		t.Skip("LAB_SAMPLES not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.xlsx"))
	sort.Strings(files)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("\n######## %s (%d KB)\n", filepath.Base(path), len(raw)/1024)
		fmt.Printf("as uploaded (first sheet):\n")
		readAll(raw)

		f, err := excelize.OpenReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		sheets := f.GetSheetList()
		f.Close()
		for _, s := range sheets {
			only, header := sheetFirst(raw, s, false, false)
			fmt.Printf("\n== sheet %q\n", s)
			profile(only, header)
			fmt.Printf("  read as is:\n")
			readAll(only)
			if header > 1 {
				trimmed, _ := sheetFirst(raw, s, true, false)
				fmt.Printf("  read with rows above row %d removed:\n", header)
				readAll(trimmed)
			}
			cleaned, _ := sheetFirst(raw, s, true, true)
			fmt.Printf("  read with rows above row %d and unnamed columns removed:\n", header)
			readAll(cleaned)
		}
	}
}

// sheetFirst is the workbook with only sheet s left, and the row the header seems to be on.
// With trim, rows above that header are removed; with unnamed, so are columns with a blank header cell.
func sheetFirst(raw []byte, s string, trim, unnamed bool) ([]byte, int) {
	f, _ := excelize.OpenReader(bytes.NewReader(raw))
	defer f.Close()
	for _, other := range f.GetSheetList() {
		if other != s {
			f.DeleteSheet(other)
		}
	}
	rows, _ := f.GetRows(s, excelize.Options{RawCellValue: true})
	header := guessHeader(rows)
	if trim {
		for i := 1; i < header; i++ {
			f.RemoveRow(s, 1)
		}
	}
	if unnamed && header <= len(rows) {
		hdr := rows[header-1]
		for ci := len(hdr) - 1; ci >= 0; ci-- {
			if strings.TrimSpace(hdr[ci]) == "" {
				col, _ := excelize.ColumnNumberToName(ci + 1)
				f.RemoveCol(s, col)
			}
		}
	}
	var b bytes.Buffer
	f.Write(&b)
	return b.Bytes(), header
}

// guessHeader is the 1-based row, among the first 15, that is the first to fill at least 60% of the
// widest of those rows: the first row that looks like a header rather than a title.
func guessHeader(rows [][]string) int {
	filled := func(r []string) int {
		n := 0
		for _, v := range r {
			if strings.TrimSpace(v) != "" {
				n++
			}
		}
		return n
	}
	most := 0
	for i := 0; i < len(rows) && i < 15; i++ {
		most = max(most, filled(rows[i]))
	}
	for i := 0; i < len(rows) && i < 15; i++ {
		if most > 0 && filled(rows[i])*10 >= most*6 {
			return i + 1
		}
	}
	return 1
}

var dayMonth = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-]\d{2,4}`)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

var oneDigit = regexp.MustCompile(`\d`)

// mask is v with letters as "a" and each digit as "9", so its shape shows and its contents do not.
func mask(v string) string {
	return oneDigit.ReplaceAllString(letters.ReplaceAllString(v, "a"), "9")
}

func profile(raw []byte, header int) {
	f, _ := excelize.OpenReader(bytes.NewReader(raw))
	defer f.Close()
	s := f.GetSheetList()[0]
	rows, _ := f.GetRows(s, excelize.Options{RawCellValue: true})
	merged, _ := f.GetMergeCells(s)
	mergedHeader := 0
	for _, m := range merged {
		_, r, _ := excelize.CellNameToCoordinates(m.GetStartAxis())
		if r <= header {
			mergedHeader++
		}
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	var hdr []string
	if header <= len(rows) {
		hdr = rows[header-1]
	}
	blankHeader, numHeader := 0, 0
	for _, h := range hdr {
		h = strings.TrimSpace(h)
		if h == "" {
			blankHeader++
		} else if _, err := strconv.ParseFloat(h, 64); err == nil {
			numHeader++
		}
	}
	// columns blank all the way down, inside the header's span
	blankCols := 0
	for ci := 0; ci < len(hdr); ci++ {
		empty := true
		for _, r := range rows[min(header, len(rows)):] {
			if ci < len(r) && strings.TrimSpace(r[ci]) != "" {
				empty = false
				break
			}
		}
		if empty {
			blankCols++
		}
	}
	shapes, firstOver12, secondOver12 := map[string]int{}, 0, 0
	dataRows, totals, dash, ampmN, textNum, textDate, blankLead := 0, 0, 0, 0, 0, 0, 0
	for ri := header; ri < len(rows); ri++ {
		r := rows[ri]
		if strings.TrimSpace(strings.Join(r, "")) == "" {
			continue
		}
		dataRows++
		if len(r) > 0 && strings.TrimSpace(r[0]) == "" {
			blankLead++
		}
		for ci, v := range r {
			v = strings.TrimSpace(v)
			if totalWord.MatchString(v) && ci < 3 {
				totals++
			}
			if v == "-" {
				dash++
			}
			if ampm.MatchString(v) {
				ampmN++
			}
			if v == "" {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(ci+1, ri+1)
			if typ, _ := f.GetCellType(s, cell); typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString {
				if numText.MatchString(v) {
					textNum++
				} else if dateText.MatchString(v) || ampm.MatchString(v) {
					textDate++
					shapes[mask(v)]++
					if m := dayMonth.FindStringSubmatch(v); m != nil {
						a, _ := strconv.Atoi(m[1])
						b, _ := strconv.Atoi(m[2])
						firstOver12 += b2i(a > 12)
						secondOver12 += b2i(b > 12)
					}
				}
			}
		}
	}
	fmt.Printf("  rows=%d width=%d header-row=%d header-cells=%d blank-headers=%d numeric-headers=%d merged=%d (in/above header %d)\n",
		len(rows), width, header, len(hdr), blankHeader, numHeader, len(merged), mergedHeader)
	fmt.Printf("  data-rows=%d blank-cols=%d total-rows=%d dash-cells=%d ampm-cells=%d text-numbers=%d text-dates=%d blank-first-cell-rows=%d\n",
		dataRows, blankCols, totals, dash, ampmN, textNum, textDate, blankLead)
	fmt.Printf("  headers: %q\n", hdr)
	if len(shapes) > 0 {
		fmt.Printf("  text date shapes: %v (first part over 12: %d, second part over 12: %d)\n", shapes, firstOver12, secondOver12)
	}
}

func readAll(raw []byte) {
	for _, tpl := range lab.Templates {
		d, err := lab.Read(bytes.NewReader(raw), tpl)
		if err != nil {
			fmt.Printf("    %-9s FAIL %s\n", tpl.Key, describe(err))
			continue
		}
		var cols []string
		for _, c := range d.Columns {
			cols = append(cols, c.Name+":"+string(c.Type))
		}
		guess := tpl.Guess(d)
		_, _, gerr := lab.Generate(tpl, "x", guess, d, "data.json")
		g := "ok"
		if gerr != nil {
			g = describe(gerr)
		}
		blocks := ""
		if tpl.Build != nil {
			bt, _ := tpl.Build(d)
			for _, b := range bt.Blocks {
				blocks += b.Kind + "(" + b.Label + ") "
			}
		}
		fmt.Printf("    %-9s OK rows=%d cols=%v guess=%v generate=%s %s\n", tpl.Key, len(d.Rows), cols, guess, g, blocks)
	}
}

// describe is an error as its Problem keys with counts and the column each names, never cell values.
func describe(err error) string {
	var errs lab.Errors
	var p lab.Problem
	switch {
	case errors.As(err, &errs):
	case errors.As(err, &p):
		errs = lab.Errors{p}
	default:
		return err.Error()
	}
	count, where := map[string]int{}, map[string]map[string]bool{}
	for _, p := range errs {
		count[p.Key]++
		var at string
		switch p.Key {
		case "lab.err.unnamed-column":
			at = fmt.Sprint(p.Args[0])
		case "lab.err.same-name", "lab.err.missing-column", "lab.err.choose-column":
			at = fmt.Sprint(p.Args[0])
		case "lab.err.not-date", "lab.err.not-number", "lab.err.empty-cell":
			at = fmt.Sprint(p.Args[1])
		case "lab.err.stray-cell":
			at = fmt.Sprint(p.Args[2])
		case "lab.err.too-many-rows":
			at = fmt.Sprint(p.Args[0])
		}
		if where[p.Key] == nil {
			where[p.Key] = map[string]bool{}
		}
		if at != "" && len(where[p.Key]) < 8 {
			where[p.Key][at] = true
		}
	}
	var keys []string
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		var at []string
		for a := range where[k] {
			at = append(at, a)
		}
		sort.Strings(at)
		out = append(out, fmt.Sprintf("%s x%d %v", strings.TrimPrefix(k, "lab.err."), count[k], at))
	}
	return strings.Join(out, "; ")
}

var (
	letters = regexp.MustCompile(`\p{L}+`)
	digits  = regexp.MustCompile(`\d+`)
)

// TestResearchShapes prints a sheet's first rows as shapes: letters as "a", digits as "9", so a layout can be
// seen without its contents. LAB_SHAPE is "file|sheet|rows", e.g. "roster-2026-10.xlsx|Veterinary|10".
func TestResearchShapes(t *testing.T) {
	dir, spec := os.Getenv("LAB_SAMPLES"), os.Getenv("LAB_SHAPE")
	if dir == "" || spec == "" {
		t.Skip("LAB_SAMPLES or LAB_SHAPE not set")
	}
	parts := strings.Split(spec, "|")
	n, _ := strconv.Atoi(parts[2])
	f, err := excelize.OpenFile(filepath.Join(dir, parts[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(parts[1], excelize.Options{RawCellValue: true})
	merged, _ := f.GetMergeCells(parts[1])
	for _, m := range merged {
		if _, r, _ := excelize.CellNameToCoordinates(m.GetStartAxis()); r <= n {
			fmt.Printf("merged %s:%s\n", m.GetStartAxis(), m.GetEndAxis())
		}
	}
	for i := 0; i < n && i < len(rows); i++ {
		var out []string
		for _, v := range rows[i] {
			out = append(out, digits.ReplaceAllString(letters.ReplaceAllString(strings.TrimSpace(v), "a"), "9"))
		}
		fmt.Printf("%3d %q\n", i+1, out)
	}
}

// TestResearchColumnShapes prints, for each column of a sheet below its header row, its five commonest shapes.
// LAB_SHAPE is "file|sheet" or "file|sheet|header row".
func TestResearchColumnShapes(t *testing.T) {
	dir, spec := os.Getenv("LAB_SAMPLES"), os.Getenv("LAB_SHAPE")
	if dir == "" || spec == "" {
		t.Skip("LAB_SAMPLES or LAB_SHAPE not set")
	}
	parts := strings.Split(spec, "|")
	f, err := excelize.OpenFile(filepath.Join(dir, parts[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(parts[1], excelize.Options{RawCellValue: true})
	if len(parts) > 2 {
		n, _ := strconv.Atoi(parts[2])
		rows = rows[n-1:]
	}
	for ci, h := range rows[0] {
		count := map[string]int{}
		for _, r := range rows[1:] {
			v := ""
			if ci < len(r) {
				v = strings.TrimSpace(r[ci])
			}
			count[mask(v)]++
		}
		var shapes []string
		for s := range count {
			shapes = append(shapes, s)
		}
		sort.Slice(shapes, func(i, j int) bool { return count[shapes[i]] > count[shapes[j]] })
		var out []string
		for _, s := range shapes[:min(5, len(shapes))] {
			out = append(out, fmt.Sprintf("%q:%d", s, count[s]))
		}
		fmt.Printf("%-24q %d shapes: %s\n", h, len(count), strings.Join(out, " "))
	}
}
