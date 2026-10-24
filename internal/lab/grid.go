package lab

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// minGridDays is how many day numbers in a row, from 1, make a header a days-across grid.
const minGridDays = 28

// gridDays finds a days-across grid header: the column of day 1 and how many days follow it in order (1, 2, 3,
// ...), 0 days when fewer than minGridDays or no column comes before day 1 to name the staff.
func gridDays(header []string) (first, days int) {
	first = slices.IndexFunc(header, func(v string) bool { return strings.TrimSpace(v) == "1" })
	if first < 1 {
		return 0, 0
	}
	for first+days < len(header) && strings.TrimSpace(header[first+days]) == strconv.Itoa(days+1) {
		days++
	}
	if days < minGridDays {
		return 0, 0
	}
	return first, days
}

// readGrid reads a days-across grid whose header at cells[h] has days from column first: the columns before
// day 1 (a blank first one is Staff) then Date and Code. Its rows keep their day cells until unpivot dates them.
// The grid ends at the first fully blank row, so a legend below it is left out, and at its last day, so counts
// beside it are too. A row with no staff, like weekday names under the days, is skipped.
func readGrid(name string, cells [][]string, h, first, days int) (*table, error) {
	names := append(slices.Clone(cells[h][:first]), "Date", "Code")
	if strings.TrimSpace(names[0]) == "" {
		names[0] = "Staff"
	}
	cols, err := columnsOf(names)
	if err != nil {
		return nil, err
	}
	tb := &table{name: name, header: h, cols: cols, flat: true, days: days}
	for r := h + 1; r < len(cells); r++ {
		vals := make([]cell, first+days)
		for c := range vals {
			if c < len(cells[r]) {
				vals[c].v = strings.TrimSpace(cells[r][c])
			}
		}
		if strings.TrimSpace(strings.Join(cells[r], "")) == "" {
			if len(tb.raw) > 0 {
				break
			}
			continue
		}
		if slices.ContainsFunc(vals[:first], func(c cell) bool { return c.v != "" }) {
			tb.raw, tb.rowNo = append(tb.raw, vals), append(tb.rowNo, r+1)
		}
	}
	if len(tb.raw) == 0 {
		return nil, Problem{"lab.err.no-data", nil}
	}
	for _, row := range cells[:h] {
		tb.title += " " + strings.Join(row, " ")
	}
	tb.title += " " + name
	return tb, nil
}

// unpivot turns grid tb's rows into one row per staff per filled day, dated in month and year. Days past the
// month's end are left out.
func (tb *table) unpivot(year int, month time.Month) {
	ids, last := len(tb.cols)-2, time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var raw [][]cell
	var rowNo []int
	for ri, vals := range tb.raw {
		for d := 1; d <= min(tb.days, last); d++ {
			if code := vals[ids+d-1]; code.v != "" {
				raw = append(raw, append(slices.Clone(vals[:ids]), cell{v: fmt.Sprintf("%d/%d/%d", d, month, year)}, code))
				rowNo = append(rowNo, tb.rowNo[ri])
			}
		}
	}
	tb.raw, tb.rowNo = raw, rowNo
}

var (
	months = map[string]time.Month{
		"jan": 1, "january": 1, "januari": 1, "feb": 2, "february": 2, "februari": 2, "mar": 3, "march": 3, "mac": 3,
		"apr": 4, "april": 4, "may": 5, "mei": 5, "jun": 6, "june": 6, "jul": 7, "july": 7, "julai": 7,
		"aug": 8, "august": 8, "ogos": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10, "okt": 10,
		"oktober": 10, "nov": 11, "november": 11, "dec": 12, "december": 12, "dis": 12, "disember": 12,
	}
	fullYear = regexp.MustCompile(`^(19|20)\d\d$`)
)

// gridMonth is the month and year the grids' days fall in: the first month named, in Malay or English, in a
// grid's title rows or sheet name, with the year beside it; else this month. The year is this year if none is named.
// ponytail: names only; a title that is a real Excel date cell is not read, the Learner can set the month.
func gridMonth(tables []*table, now time.Time) (int, time.Month) {
	for _, tb := range tables {
		if tb == nil || tb.days == 0 {
			continue
		}
		year, month := now.Year(), time.Month(0)
		for _, w := range strings.FieldsFunc(strings.ToLower(tb.title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if m, ok := months[w]; ok && month == 0 {
				month = m
			} else if fullYear.MatchString(w) {
				year, _ = strconv.Atoi(w)
			}
		}
		if month != 0 {
			return year, month
		}
	}
	return now.Year(), now.Month()
}
