package lab

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// one is a Dataset of a single column, a row for each value.
func one(name string, typ Type, vals ...any) *Dataset {
	d := &Dataset{Columns: []Column{{Name: name, Type: typ}}}
	for _, v := range vals {
		d.Rows = append(d.Rows, []any{v})
	}
	return d
}

// values are n distinct strings, each times over.
func values(n, times int) []any {
	var out []any
	for range times {
		for i := range n {
			out = append(out, fmt.Sprintf("v%02d", i))
		}
	}
	return out
}

func TestProfilesGiveEachColumnItsRoleAndWhyItIsLeftOut(t *testing.T) {
	long := func(r string, n int) string { return strings.Repeat(r, n) }
	for _, c := range []struct {
		name string
		d    *Dataset
		role Role
		note string
	}{
		{"one value", one("Cawangan", Text, "Ipoh", "Ipoh"), Constant, "lab.note.constant"},
		{"no value", one("Cawangan", Text, nil, nil), Constant, "lab.note.constant"},
		{"two values", one("Cawangan", Text, "Ipoh", "Kuala Lumpur"), Dimension, ""},
		{"a date", one("Tarikh", Date, "2026-01-02", "2026-01-01"), DateRole, ""},
		{"years", one("Tahun", Text, "1900", "2100"), Dimension, "lab.note.year"},
		{"years kept as numbers", one("Year", Number, 2020.0, 2021.0), Dimension, "lab.note.year"},
		{"before the first year", one("Tahun", Text, "1899", "2000"), Dimension, ""},
		{"after the last year", one("Tahun", Text, "2000", "2101"), Dimension, ""},
		{"a number in a year column", one("Tahun", Number, 2020.0, 20.5), Measure, ""},
		{"a year column with other text", one("Tahun", Text, "2020", "n/a"), Dimension, ""},
		{"days of the month", one("Tarikh", Number, 1.0, 15.0), Identifier, "lab.note.serial-date"},
		{"a numbered ID", one("No Pekerja", Number, 1.0, 2.0), Identifier, "lab.note.id"},
		{"a number to add up", one("Jumlah", Number, 1.0, 2.0), Measure, ""},
		{"a number to average", one("Price", Number, 1.0, 2.0), Measure, "lab.note.average"},
		{"a total price", one("Total Price", Number, 1.0, 2.0), Measure, ""},
		{"a percentage", one("Growth %", Number, 1.0, 2.0), Measure, "lab.note.average"},
		{"free text by its name", one("Remarks", Text, "ok", "late"), FreeText, "lab.note.free-text"},
		{"40 characters each", one("Nama", Text, long("a", 40), long("b", 40)), Dimension, ""},
		{"40 letters of 2 bytes each", one("Nama", Text, long("é", 40), long("è", 40)), Dimension, ""},
		{"41 characters each", one("Nama", Text, long("a", 41), long("b", 41)), FreeText, "lab.note.free-text"},
		{"40 characters on average", one("Nama", Text, long("a", 10), long("b", 70)), Dimension, ""},
		{"an ID by its name", one("ID", Text, "a", "b"), Identifier, "lab.note.id"},
		{"two codes", one("Kod", Text, "a", "b"), Dimension, ""},
		{"15 codes", one("Kod", Text, values(15, 2)...), Dimension, ""},
		{"16 codes", one("Kod", Text, values(16, 2)...), Identifier, "lab.note.id"},
		{"10 different values", one("Nama", Text, values(10, 1)...), Identifier, "lab.note.unique"},
		{"9 different values", one("Nama", Text, values(9, 1)...), Dimension, ""},
		{"10 values, one twice", one("Nama", Text, append(values(9, 1), "v00")...), Dimension, ""},
		{"50 values", one("Nama", Text, values(50, 2)...), Dimension, ""},
		{"51 values", one("Nama", Text, values(51, 2)...), Identifier, "lab.note.many"},
	} {
		p := profiles(c.d)[0]
		if p.Role != c.role || p.note != c.note {
			t.Errorf("%s: role %q and note %q, want %q and %q", c.name, p.Role, p.note, c.role, c.note)
		}
	}
}

func TestProfilesCountValuesAverageMeasuresAndNoteTheFirstAndLastDay(t *testing.T) {
	for name, c := range map[string]struct {
		d      *Dataset
		values int
		avg    bool
	}{
		"Price":       {one("Price", Number, 1.0, 1.0, 2.0, nil), 2, true},
		"Total Price": {one("Total Price", Number, 1.0, 2.0, 3.0), 3, false},
		"Growth %":    {one("Growth %", Number, 1.0, 2.0), 2, true},
		"Jumlah":      {one("Jumlah", Number, 1.0, 2.0), 2, false},
	} {
		if p := profiles(c.d)[0]; p.Values != c.values || p.Avg != c.avg {
			t.Errorf("%s: %d values, averaged %t; want %d, %t", name, p.Values, p.Avg, c.values, c.avg)
		}
	}
	p := profiles(one("Tarikh", Date, "2026-01-02", nil, "2026-01-01", "2026-03-04", "2026-01-02"))[0]
	if p.from != "2026-01-01" || p.to != "2026-03-04" || p.Values != 3 {
		t.Errorf("date column: %q to %q, %d values; want 2026-01-01 to 2026-03-04, 3 values", p.from, p.to, p.Values)
	}
}

func TestProfilesMeasureHowUnevenTheGroupsOfADimensionAre(t *testing.T) {
	// Groups of 5 and 1 around a mean of 3: the spread of their sizes is 2, and 2/3 of the mean.
	d := one("Jenis", Text, "A", "A", "A", "A", "A", "B")
	if p := profiles(d)[0]; p.Role != Dimension || math.Abs(p.spread-2.0/3) > 1e-12 {
		t.Errorf("role %q, spread %v; want dimension, 2/3", p.Role, p.spread)
	}
	if p := profiles(one("Jenis", Text, "A", "A", "B", "B"))[0]; p.spread != 0 {
		t.Errorf("even groups have spread %v, want 0", p.spread)
	}
	if p := profiles(one("Jumlah", Number, 1.0, 1.0, 2.0))[0]; p.spread != 0 {
		t.Errorf("a measure has spread %v, want 0", p.spread)
	}
}

func TestPricesOnAListWithNoDateStayAveraged(t *testing.T) {
	items := &Dataset{Columns: []Column{{"Item", Text}, {"Price", Number}}}
	for i := range 6 {
		items.Rows = append(items.Rows, []any{[]string{"Teh", "Kopi", "Roti"}[i%3], float64(i + 1)})
	}
	if p := profiles(items)[1]; !p.Avg || p.note != "lab.note.average" {
		t.Errorf("Price of a list with no date: averaged %t, note %q; want averaged", p.Avg, p.note)
	}
	// With a date it is a list of sales, and each sale's price is added up.
	items.Columns = append(items.Columns, Column{"Tarikh", Date})
	for i := range items.Rows {
		items.Rows[i] = append(items.Rows[i], fmt.Sprintf("2026-01-0%d", i+1))
	}
	if p := profiles(items)[1]; p.Avg || p.note != "" {
		t.Errorf("Price of a list of sales: averaged %t, note %q; want added up", p.Avg, p.note)
	}
}

func TestIdNameTellsAnIDFromAnythingElseNamed(t *testing.T) {
	for name, want := range map[string]bool{
		"Invoice":         true,
		"ID":              true,
		"Kod Produk":      true,
		"#":               true,
		"No.":             true,
		"Nombor":          true,
		"No":              true,
		"Customer No":     true, // No last
		"No Pekerja":      true, // No first
		"Bil":             true,
		"No of Items":     false,
		"Items No of Pcs": false,
		"Number of Items": false,
		"Bil Pekerja":     false, // a name by its whole key
		"Nama":            false,
		"":                false,
	} {
		if got := idName(name); got != want {
			t.Errorf("idName(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestRankOfIsThePlaceOfTheFirstNameTheColumnNameHolds(t *testing.T) {
	for name, want := range map[string]int{"Tarikh Jualan": 0, "DATE": 1, " Date  Created": 1, "Created": 2, "Hari": 3, "Nama": len(dateNames)} {
		if got := rankOf(name, dateNames); got != want {
			t.Errorf("rankOf(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestBoolCmpPutsFalseBeforeTrue(t *testing.T) {
	for _, c := range []struct {
		a, b bool
		want int
	}{{false, false, 0}, {true, true, 0}, {true, false, 1}, {false, true, -1}} {
		if got := boolCmp(c.a, c.b); got != c.want {
			t.Errorf("boolCmp(%t, %t) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// names are the names of the profiles, in order.
func names(ps []profile) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestRankedPutsMeasuresAddedUpBeforeAveragedOnesThenByName(t *testing.T) {
	d := &Dataset{Columns: []Column{{"Balance", Number}, {"Unit", Number}, {"Closing", Number}, {"Jumlah", Number}, {"Bulan", Text}, {"Tarikh", Date}},
		Rows: [][]any{{1.0, 1.0, 1.0, 1.0, "Jan", "2026-01-01"}, {2.0, 2.0, 2.0, 2.0, "Feb", "2026-01-02"}}}
	got := names(ranked(profiles(d), Measure, Number))
	if want := []string{"Jumlah", "Closing", "Unit", "Balance"}; !slices.Equal(got, want) {
		t.Errorf("measures = %v, want %v", got, want)
	}
}

func TestRankedPutsDatesByName(t *testing.T) {
	d := &Dataset{Columns: []Column{{"Created", Date}, {"Tarikh", Date}, {"Date", Date}}}
	for i := range 2 {
		day := fmt.Sprintf("2026-01-0%d", i+1)
		d.Rows = append(d.Rows, []any{day, day, day})
	}
	got := names(ranked(profiles(d), DateRole, Date))
	if want := []string{"Tarikh", "Date", "Created"}; !slices.Equal(got, want) {
		t.Errorf("dates = %v, want %v", got, want)
	}
}

func TestRankedPutsDimensionsOfThreeToFifteenValuesFirstThenTheMostUneven(t *testing.T) {
	dim := func(name string, values int, spread float64) profile {
		return profile{Column: Column{Name: name, Type: Text}, Role: Dimension, Values: values, spread: spread}
	}
	ps := []profile{
		dim("16", 16, 0), dim("15", 15, 0), dim("3", 3, 0), dim("2", 2, 0),
		dim("Sheet", 2, 0), dim("Even", 4, 0.1), dim("Uneven", 4, 0.9),
		{Column: Column{Name: "Jumlah", Type: Number}, Role: Measure},
	}
	got := names(ranked(ps, Dimension, Text))
	// Sheet first, then 3 to 15 values by spread, then the rest as they come.
	if want := []string{"Sheet", "Uneven", "Even", "15", "3", "16", "2"}; !slices.Equal(got, want) {
		t.Errorf("dimensions = %v, want %v", got, want)
	}
}

// twoRows is a Dataset of the columns, two rows of values each: row 1 the first of each pair, row 2 the second.
func twoRows(cols ...any) *Dataset {
	d := &Dataset{Rows: [][]any{nil, nil}}
	for i := 0; i < len(cols); i += 3 {
		d.Columns = append(d.Columns, Column{Name: cols[i].(string), Type: cols[i+1].(Type)})
		pair := cols[i+2].([2]any)
		d.Rows[0], d.Rows[1] = append(d.Rows[0], pair[0]), append(d.Rows[1], pair[1])
	}
	return d
}

func TestTableColsAreTheDateAnIdentifierThenTheTopDimensionsAndMeasures(t *testing.T) {
	date, id := [2]any{"2026-01-01", "2026-01-02"}, [2]any{"a", "b"}
	dim, num, notes, ipoh := [2]any{"x", "y"}, [2]any{1.0, 2.0}, [2]any{"ok", "late"}, [2]any{"Ipoh", "Ipoh"}
	for _, c := range []struct {
		name string
		d    *Dataset
		want []string
	}{
		{"four columns are a full table, whatever else there is", twoRows(
			"Tarikh", Date, date, "ID", Text, id, "Cawangan", Text, dim, "Jumlah", Number, num, "Remarks", Text, notes, "Branch", Text, ipoh),
			[]string{"Tarikh", "ID", "Cawangan", "Jumlah"}},
		{"fewer than four fill up in column order", twoRows(
			"Tarikh", Date, date, "ID", Text, id, "Jumlah", Number, num, "Remarks", Text, notes, "Branch", Text, ipoh),
			[]string{"Tarikh", "ID", "Jumlah", "Remarks", "Branch"}},
		{"an identifier in the first column", twoRows(
			"ID", Text, id, "Remarks", Text, notes, "Cawangan", Text, dim, "Jumlah", Number, num),
			[]string{"ID", "Cawangan", "Jumlah", "Remarks"}},
		{"no identifier", twoRows(
			"Tarikh", Date, date, "Cawangan", Text, dim, "Jumlah", Number, num, "Remarks", Text, notes),
			[]string{"Tarikh", "Cawangan", "Jumlah", "Remarks"}},
	} {
		if got := tableCols(c.d, profiles(c.d)); !slices.Equal(got, c.want) {
			t.Errorf("%s: columns %v, want %v", c.name, got, c.want)
		}
	}
}

// evenly is a Dataset of 84 rows: a date over two days, a Jumlah, and for each count a column of that many values
// in equal groups, called Kolum and the count.
func evenly(counts ...int) *Dataset {
	d := &Dataset{Columns: []Column{{"Tarikh", Date}, {"Jumlah", Number}}}
	for _, n := range counts {
		d.Columns = append(d.Columns, Column{fmt.Sprint("Kolum ", n), Text})
	}
	for i := range 84 {
		row := []any{fmt.Sprintf("2026-01-0%d", i%2+1), float64(i)}
		for _, n := range counts {
			row = append(row, fmt.Sprint("v", i%n))
		}
		d.Rows = append(d.Rows, row)
	}
	return d
}

// block is the Config's first block of kind.
func block(cfg *Config, kind string) (WorkBlock, bool) {
	i := slices.IndexFunc(cfg.Blocks, func(b WorkBlock) bool { return b.Kind == kind })
	if i < 0 {
		return WorkBlock{}, false
	}
	return cfg.Blocks[i], true
}

func TestGeneralSplitsTheLineOnlyByADimensionOfUpToFiveValues(t *testing.T) {
	for values, want := range map[int]string{5: "Kolum 5", 6: ""} {
		line, ok := block(Build(evenly(values)), "line")
		if !ok || line.Split != want {
			t.Errorf("a dimension of %d values: line %+v, split %q; want %q", values, line, line.Split, want)
		}
	}
	// The first dimension that is small enough, not the first.
	if line, _ := block(Build(evenly(12, 4)), "line"); line.Split != "Kolum 4" {
		t.Errorf("line split by %q, want Kolum 4", line.Split)
	}
}

func TestGeneralDrawsADonutOnlyForADimensionOfUpToSixValuesBesideTheBars(t *testing.T) {
	donut := func(d *Dataset) string {
		b, _ := block(Build(d), "donut")
		return b.Group
	}
	for _, c := range []struct {
		d    *Dataset
		want string
	}{
		{evenly(3, 4, 6), "Kolum 6"},
		{evenly(3, 4, 7), ""},
		{evenly(3, 4), ""},
		{evenly(3), ""},
		{evenly(12, 4, 3, 6), "Kolum 3"},
	} {
		if got := donut(c.d); got != c.want {
			t.Errorf("columns %v: donut by %q, want %q", c.d.Columns, got, c.want)
		}
	}
}

func TestGeneralNeedsAPriceThatIsAMeasureToMultiplyByTheQuantity(t *testing.T) {
	d := twoRows("Price ID", Number, [2]any{1.0, 2.0}, "Quantity", Number, [2]any{3.0, 4.0}, "Balance", Number, [2]any{5.0, 6.0})
	if q, p := priceAndQuantity(d, profiles(d)); q != "" || p != "" {
		t.Errorf("a price that is an ID: quantity %q, price %q; want none", q, p)
	}
	d = twoRows("Unit Price", Number, [2]any{1.0, 2.0}, "Quantity", Number, [2]any{3.0, 4.0})
	if q, p := priceAndQuantity(d, profiles(d)); q != "Quantity" || p != "Unit Price" {
		t.Errorf("quantity %q, price %q; want Quantity, Unit Price", q, p)
	}
	d = twoRows("Unit Price", Number, [2]any{1.0, 2.0}, "Quantity ID", Number, [2]any{3.0, 4.0}, "Balance", Number, [2]any{5.0, 6.0})
	if q, p := priceAndQuantity(d, profiles(d)); q != "" || p != "" {
		t.Errorf("a quantity that is an ID: quantity %q, price %q; want none", q, p)
	}
}

func TestLineByIsDayForUpTo62DaysAndMonthBeyond(t *testing.T) {
	for _, c := range []struct {
		name     string
		p        profile
		by       string
		coverage bool
	}{
		{"62 days", profile{from: "2026-01-01", to: "2026-03-04", Values: 3}, "day", true},
		{"63 days", profile{from: "2026-01-01", to: "2026-03-05", Values: 3}, "month", true},
		{"two days", profile{from: "2026-01-05", to: "2026-01-06", Values: 2}, "day", true},
		{"one day", profile{from: "2026-01-05", to: "2026-01-05", Values: 1}, "day", false},
		{"months apart", profile{from: "2025-12-31", to: "2027-01-01", Values: 3}, "month", true},
		{"not dates", profile{from: "a", to: "b", Values: 3}, "", false},
	} {
		if by, ok := lineBy(c.p); by != c.by || ok != c.coverage {
			t.Errorf("%s: lineBy = %q, %t; want %q, %t", c.name, by, ok, c.by, c.coverage)
		}
	}
}
