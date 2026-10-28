package lab

// Tests of Clean-up's small rules at their edges, for what mutation testing (gremlins) found no test noticing. They
// call the unexported steps directly where Clean would need a workbook built just to reach one boundary.

import (
	"archive/zip"
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

func TestGroupsAboveTellsAGroupRowOverTheHeaderFromADataRow(t *testing.T) {
	// A header at index h, data right below it, and the row above it holding group names (or not).
	rows := func(above, header, data []string) [][]string { return [][]string{above, header, data} }
	cases := []struct {
		name   string
		cells  [][]string
		merges [][4]int
		h      int
		want   bool
	}{
		{"two group names", rows([]string{"Jualan", "", "Kos"}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), nil, 1, true},
		{"three group names", rows([]string{"Jualan", "Kos", "Untung"}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), nil, 1, true},
		{"one name, no merge", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), nil, 1, false},
		{"blank cells are not names", rows([]string{"Laporan", " ", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), nil, 1, false},
		{"the row below the header is not the group row", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "Teh", "6"}), nil, 1, false},
		{"one name merged across columns after the first", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), [][4]int{{0, 1, 0, 2}}, 1, true},
		{"merged across the sheet from A is a title", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), [][4]int{{0, 0, 0, 2}}, 1, false},
		{"merged over one column only", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), [][4]int{{0, 1, 0, 1}}, 1, false},
		{"merged on another row", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), [][4]int{{2, 1, 2, 2}}, 1, false},
		{"merged on the header row", rows([]string{"Laporan", "", ""}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}), [][4]int{{1, 1, 1, 2}}, 1, false},
		{"no row above", rows([]string{"Item", "Tunai", "Kad"}, []string{"Kopi", "5", "6"}, nil)[:2], nil, 0, false},
		{"no row below", [][]string{{"Jualan", "", "Kos"}, {"Item", "Tunai", "Kad"}}, nil, 1, false},
		{"no number below", rows([]string{"Jualan", "", "Kos"}, []string{"Item", "Tunai", "Kad"}, []string{"Kopi", "Teh", "Susu"}), nil, 1, false},
	}
	for _, c := range cases {
		if got := groupsAbove(c.merges, c.cells, c.h); got != c.want {
			t.Errorf("%s: groupsAbove = %v, want %v", c.name, got, c.want)
		}
	}
	// Two rows above: only the one right above the header counts, and a longer sheet below does no harm.
	long := [][]string{{"Jualan", "Kos"}, {"x"}, {"Item", "Tunai"}, {"Kopi", "5"}, {"Teh", "6"}}
	if groupsAbove(nil, long, 2) {
		t.Errorf("a group row two rows above the header was read")
	}
}

func mutantsWorkbook(t *testing.T, sheets ...testutil.Sheet) (*excelize.File, []string) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(testutil.Workbook(t, sheets...)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	var names []string
	for _, s := range sheets {
		names = append(names, s.Name)
	}
	return f, names
}

func TestLikeliestGivesThePickedSheetElseTheOneWithTheMostFilledRows(t *testing.T) {
	sheet := func(name string, rows ...string) testutil.Sheet {
		s := testutil.Sheet{Name: name}
		for _, r := range rows {
			s.Rows = append(s.Rows, []any{r})
		}
		return s
	}
	f, names := mutantsWorkbook(t, sheet("A", "x", "y"), sheet("B", "x", "y", "z"))
	if got := likeliest(f, names, "A"); got != 0 {
		t.Errorf("the picked first sheet: likeliest = %d, want 0", got)
	}
	if got := likeliest(f, names, "B"); got != 1 {
		t.Errorf("the picked second sheet: likeliest = %d, want 1", got)
	}
	if got := likeliest(f, names, ""); got != 1 {
		t.Errorf("no pick, the fuller sheet: likeliest = %d, want 1", got)
	}

	f, names = mutantsWorkbook(t, sheet("A", "x", "y"), sheet("B", "x", "y"))
	if got := likeliest(f, names, ""); got != 0 {
		t.Errorf("a tie goes to the first sheet: likeliest = %d, want 0", got)
	}

	// A sheet with nothing filled is still a candidate, behind any sheet with a row.
	f, names = mutantsWorkbook(t, sheet("A"), sheet("B", "x"))
	if got := likeliest(f, names, ""); got != 1 {
		t.Errorf("an empty sheet before a one-row sheet: likeliest = %d, want 1", got)
	}
	f, names = mutantsWorkbook(t, sheet("A"), sheet("B"))
	if got := likeliest(f, names, ""); got != 0 {
		t.Errorf("all empty: likeliest = %d, want 0", got)
	}
}

func TestLooseKeyDropsSpacesUnderscoresAndDashesAndLowersCase(t *testing.T) {
	if got, want := looseKey("Jumlah_ Bersih-RM"), "jumlahbersihrm"; got != want {
		t.Fatalf("looseKey = %q, want %q", got, want)
	}
}

func TestIsMonthNumberIsAWholeNumberFrom1To12(t *testing.T) {
	for n, want := range map[float64]bool{0: false, 1: true, 6: true, 12: true, 13: false, 1.5: false, -3: false} {
		if got := isMonthNumber(n); got != want {
			t.Errorf("isMonthNumber(%v) = %v, want %v", n, got, want)
		}
	}
}

func TestAddPartKeepsAPartOfExactlyTheRowLimit(t *testing.T) {
	cols := []Column{{"Kod", Text}}
	more := &Dataset{Columns: cols, Rows: [][]any{{"a"}}}
	if _, _, err := AddPart(cols, 1, more, 2, nil); err != nil {
		t.Fatalf("1 row had + 1 row added, limit 2: %v", err)
	}
	var p Problem
	if _, _, err := AddPart(cols, 2, more, 2, nil); !errors.As(err, &p) || p.Key != "lab.err.add-too-many-rows" {
		t.Fatalf("2 rows had + 1 row added, limit 2: got %v, want lab.err.add-too-many-rows", err)
	}
}

func TestAddPartReadsAPartsOtherTypedValuesAsTheDatasetsColumnType(t *testing.T) {
	// A number kept where the Dataset has text is written out as the number, with no decimals it never had.
	text := []Column{{"Kod", Text}}
	d, _, err := AddPart(text, 0, &Dataset{Columns: []Column{{"Kod", Number}}, Rows: [][]any{{7.0}, {2.5}}}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"7"}, {"2.5"}}; !reflect.DeepEqual(d.Rows, want) {
		t.Fatalf("rows = %v, want %v", d.Rows, want)
	}

	// A date written another way is read as a date only where the Dataset's column holds dates.
	d, _, err = AddPart([]Column{{"Tarikh", Date}}, 0, &Dataset{Columns: []Column{{"Tarikh", Text}}, Rows: [][]any{{"05.11.26"}}}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"2026-11-05"}}; !reflect.DeepEqual(d.Rows, want) {
		t.Fatalf("into a Date column: rows = %v, want %v", d.Rows, want)
	}
	d, changes, err := AddPart([]Column{{"Jumlah", Number}}, 0, &Dataset{Columns: []Column{{"Jumlah", Text}}, Rows: [][]any{{"05.11.26"}}}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{nil}}; !reflect.DeepEqual(d.Rows, want) {
		t.Fatalf("into a Number column: rows = %v, want %v", d.Rows, want)
	}
	if want := []Change{{Key: "lab.clean.not-numbers", Args: []int{1}, Column: "Jumlah"}}; !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
}

func TestLayoutDateReadsAColumnRangeFromItsFirstColumnToItsLast(t *testing.T) {
	l := layout{cols: []styledCols{{min: 2, max: 3, date: true}}} // 1-based columns B and C
	for col, want := range []bool{false, true, true, false} {
		if got := l.date(0, col); got != want {
			t.Errorf("column %d (0-based): date = %v, want %v", col, got, want)
		}
	}
	// A cell's own style comes before its row's, and its row's before its column's.
	l = layout{
		cells: []int{(1*excelize.MaxColumns + 1) << 1}, // row 1, column 1: a style that shows no date
		rows:  map[int]bool{1: true},
		cols:  []styledCols{{min: 1, max: 5, date: true}},
	}
	if l.date(1, 1) || !l.date(1, 2) || !l.date(0, 3) || l.date(0, 5) {
		t.Errorf("date = %v %v %v %v, want false true true false", l.date(1, 1), l.date(1, 2), l.date(0, 3), l.date(0, 5))
	}
}

// styledSheet is a workbook of one sheet, Sheet1, with a styles part where style 1 shows dates and style 2 does not.
func styledSheet(t *testing.T, rows string) *excelize.File {
	t.Helper()
	cols := ""
	if strings.HasPrefix(rows, "<cols>") { // a leading <cols> goes before the sheet data
		end := strings.Index(rows, "</cols>") + len("</cols>")
		cols, rows = rows[:end], rows[end:]
	}
	const ns = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	parts := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8"?><workbook ` + ns + ` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/styles.xml":              `<?xml version="1.0" encoding="UTF-8"?><styleSheet ` + ns + `><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="3"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="14" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="2" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml":   `<?xml version="1.0" encoding="UTF-8"?><worksheet ` + ns + `>` + cols + `<sheetData>` + rows + `</sheetData></worksheet>`,
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestReadLayoutKeepsEachStyledRowAndCellAtItsOwnPlace(t *testing.T) {
	// Rows and cells with no r attribute are counted from the first, as excelize counts them.
	l := readLayout(styledSheet(t, `<row s="1"><c s="1"><v>5</v></c><c><v>6</v></c><c s="2"><v>7</v></c></row><row s="2"><c s="1"><v>8</v></c></row>`), "Sheet1")
	if want := map[int]bool{0: true, 1: false}; !reflect.DeepEqual(l.rows, want) {
		t.Errorf("rows = %v, want %v", l.rows, want)
	}
	if want := []int{0<<1 | 1, 2 << 1, 1*excelize.MaxColumns<<1 | 1}; !reflect.DeepEqual(l.cells, want) {
		t.Errorf("cells = %v, want %v", l.cells, want)
	}

	// A row with no style, or style 0, is left out, and a row's r of 0 is not a row number.
	l = readLayout(styledSheet(t, `<row r="1"><c r="A1"><v>1</v></c></row><row r="2" s="0"><c r="A2" s="0"><v>1</v></c></row><row r="0" s="1"><c r="B1"><v>2</v></c></row>`), "Sheet1")
	if len(l.rows) != 1 || !l.rows[2] {
		t.Errorf("rows = %v, want only row index 2 (the third row, whose r of 0 is not a row number)", l.rows)
	}
	if len(l.cells) != 0 {
		t.Errorf("cells = %v, want none", l.cells)
	}
}

func TestReadLayoutKeepsTheLastRowAndColumnOfASheet(t *testing.T) {
	last := strconv.Itoa(excelize.TotalRows)
	l := readLayout(styledSheet(t, `<row r="`+last+`" s="1"><c r="XFD`+last+`" s="1"><v>1</v></c></row>`), "Sheet1")
	if want := map[int]bool{excelize.TotalRows - 1: true}; !reflect.DeepEqual(l.rows, want) {
		t.Errorf("rows = %v, want %v", l.rows, want)
	}
	if want := []int{((excelize.TotalRows-1)*excelize.MaxColumns+excelize.MaxColumns-1)<<1 | 1}; !reflect.DeepEqual(l.cells, want) {
		t.Errorf("cells = %v, want %v", l.cells, want)
	}
	if !l.date(excelize.TotalRows-1, excelize.MaxColumns-1) {
		t.Errorf("the date style on the last cell was not found")
	}
}

func TestSetWidensARowToTheCellItPuts(t *testing.T) {
	cells := [][]string{{"a"}, {"b", "c"}}
	set(cells, 0, 3, "x")
	set(cells, 1, 1, "y")
	if want := [][]string{{"a", "", "", "x"}, {"b", "y"}}; !reflect.DeepEqual(cells, want) {
		t.Fatalf("cells = %q, want %q", cells, want)
	}
}

func TestFillDownCountsBothTheCellsItWidensAndFillsAgainstTheCellLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("fills 2.5 million cells")
	}
	// A merge 14 columns wide from the first row to row bottom, on rows that start empty but the first: each row
	// after the first is widened by 14 and filled with 14, and the first by 13 and 13, so the count when it reaches
	// the row r is 14*(2r+1)-2: exactly maxSheetCells at r = 178571.
	run := func(bottom int) (*CleanUp, error) {
		cells := make([][]string, bottom+1)
		cells[0] = []string{"x"}
		res := &CleanUp{}
		_, err := fillDown([][4]int{{0, 0, bottom, 13}}, cells, 0, res)
		return res, err
	}
	res, err := run(178571)
	if err != nil {
		t.Fatalf("a merge reaching the limit exactly: %v", err)
	}
	if want := []Change{{Key: "lab.clean.merged-down", Args: []int{14*178572 - 1}}}; !reflect.DeepEqual(res.Changes, want) {
		t.Fatalf("changes = %v, want %v", res.Changes, want)
	}
	var p Problem
	if _, err := run(178572); !errors.As(err, &p) || p.Key != "lab.err.too-many-cells" {
		t.Fatalf("a merge one row past the limit: got %v, want lab.err.too-many-cells", err)
	}
}

func TestJoinHeaderFillsAMergedCellAcrossInEitherHeaderRow(t *testing.T) {
	cells := [][]string{
		{"Item", "Jualan", "", ""},
		{"", "Tunai", "Jumlah", ""},
	}
	// Jualan is merged over B and C of the top row, Jumlah over C and D of the bottom row.
	got := joinHeader([][4]int{{0, 1, 0, 2}, {1, 2, 1, 3}}, cells, 0, &CleanUp{})
	if want := []string{"Item", "Jualan Tunai", "Jualan Jumlah", "Jumlah"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
	// The cells it was given are left as they were.
	if cells[0][2] != "" || cells[1][3] != "" {
		t.Fatalf("cells = %q, want them untouched", cells)
	}
}

func TestCutOffKeepsDataBelowBlankLinesUnderTheHeader(t *testing.T) {
	cells := [][]string{{"Perkara", "Jumlah"}, {"", " "}, {""}, {"Beras", "5"}, {"Gula", "6"}, {}, {"Nota"}}
	res := &CleanUp{}
	got, err := cutOff(cells, 0, 1, res)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"Perkara", "Jumlah"}, {"", " "}, {"", ""}, {"Beras", "5"}, {"Gula", "6"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cells = %q, want %q", got, want)
	}
	if want := []Change{{Key: "lab.clean.cut-rows", Args: []int{6}}}; !reflect.DeepEqual(res.Changes, want) {
		t.Fatalf("changes = %v, want %v", res.Changes, want)
	}
}

func TestCutOffCountsTheCellsOfTheTableNotOfTheTitleRowsAboveIt(t *testing.T) {
	if testing.Short() {
		t.Skip("cuts a table of 5 million cells")
	}
	const width = 500
	// Rows share their cells: only the count matters to cutOff. 2000 title rows, a header, then data rows.
	table := func(data int) [][]string {
		title, wide := []string{"Laporan"}, make([]string, width)
		for i := range wide {
			wide[i] = "x"
		}
		cells := slices.Repeat([][]string{title}, 2000)
		cells = append(cells, wide)
		return append(cells, slices.Repeat([][]string{wide}, data)...)
	}
	// The header and 9999 data rows are 10000 rows of 500 cells: exactly maxSheetCells.
	if got, err := cutOff(table(9999), 2000, 2001, &CleanUp{}); err != nil || len(got) != 2000+1+9999 {
		t.Fatalf("a table of exactly the cell limit: %d rows, %v; want all %d and no error", len(got), err, 2000+1+9999)
	}
	var p Problem
	if _, err := cutOff(table(10000), 2000, 2001, &CleanUp{}); !errors.As(err, &p) || p.Key != "lab.err.too-many-cells" {
		t.Fatalf("a table one row over the limit: got %v, want lab.err.too-many-cells", err)
	}
}

func TestWorkTypeReadsAColumnByWhatMostOfItsFilledCellsAre(t *testing.T) {
	col := func(vs ...string) [][]cell {
		var raw [][]cell
		for _, v := range vs {
			raw = append(raw, []cell{{v: v}})
		}
		return raw
	}
	cases := []struct {
		name string
		raw  [][]cell
		want Type
	}{
		{"nothing filled", col("", "", ""), Text},
		{"only dashes", col("-", "-", "-"), Text},
		{"numbers and one dash", col("5", "6", "7", "-"), Number},
		{"nine dashes and one number", col("-", "-", "-", "-", "-", "-", "-", "-", "-", "5"), Number},
		{"nine numbers and one dash", col("1", "2", "3", "4", "5", "6", "7", "8", "9", "-"), Number},
		{"seven numbers, one dash, two words", col("1", "2", "3", "4", "5", "6", "7", "-", "x", "y"), Text},
		{"half numbers", col("1", "2", "x", "y"), Text},
		{"dates", col("05/11/2026", "06/11/2026", "07/11/2026"), Date},
	}
	for _, c := range cases {
		if got := workType(c.raw, 0); got != c.want {
			t.Errorf("%s: workType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMonthLikeIsFalseForAColumnOfNothingButDashes(t *testing.T) {
	raw := [][]cell{{{v: "-"}}, {{v: ""}}, {{v: "-"}}}
	if names, ok := monthLike(raw, 0); ok || names != 0 {
		t.Fatalf("monthLike = %d, %v; want 0, false", names, ok)
	}
	raw = [][]cell{{{v: "-"}}, {{v: "Mac"}}, {{v: "4"}}}
	if names, ok := monthLike(raw, 0); !ok || names != 1 {
		t.Fatalf("monthLike = %d, %v; want 1, true", names, ok)
	}
}

func TestSerialsCountsNumbersOnlyWhenEveryOneIsADateSerialFrom1950To2100(t *testing.T) {
	col := func(vs ...string) [][]cell {
		var raw [][]cell
		for _, v := range vs {
			raw = append(raw, []cell{{v: v}})
		}
		return raw
	}
	cases := []struct {
		name string
		raw  [][]cell
		want int
	}{
		{"the first and the last serial", col("18264", "73415"), 2},
		{"one before the first", col("18263", "45000"), 0},
		{"one past the last", col("73416", "45000"), 0},
		{"blanks and words are not looked at", col("45000", "", "Nota", "45001"), 2},
		{"a dash ends it", col("45000", "-"), 0},
		{"no numbers", col("", "x"), 0},
	}
	for _, c := range cases {
		if got := serials(c.raw, 0); got != c.want {
			t.Errorf("%s: serials = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestHeaderRowIsTheFirstRowFilledAcrossHalfTheWidest(t *testing.T) {
	cases := []struct {
		name  string
		cells [][]string
		want  int
	}{
		{"a two-cell title over a five-wide table", [][]string{{"Laporan", "Oktober"}, {"A", "B", "C", "D", "E"}}, 1},
		{"a three-cell row over a five-wide table", [][]string{{"A", "B", "C"}, {"A", "B", "C", "D", "E"}}, 0},
		{"a one-cell title over a two-wide table", [][]string{{"Laporan"}, {"A", "B"}}, 1},
		{"blank cells do not count", [][]string{{"A", " ", " ", " "}, {"A", "B", "C", "D"}}, 1},
		{"a table of one column", [][]string{{"A"}, {"1"}}, 0},
		{"nothing filled", [][]string{{}, {" "}}, -1},
		{"no rows", nil, -1},
	}
	for _, c := range cases {
		if got := headerRow(c.cells); got != c.want {
			t.Errorf("%s: headerRow = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTitleDateReadsAnExcelSerialOnlyInADateStyledCellFrom1ToTheLastDay(t *testing.T) {
	dated := layout{cells: []int{0<<1 | 1}} // the cell at A1 shows a date
	for serial, want := range map[string]string{"1": "1899-12-31", "45000": "2023-03-15", "2958465": "9999-12-31", "2958466": "", "0": ""} {
		day, at := titleDate([][]string{{serial}}, dated)
		if day != want || (want != "") != (at == "A1") {
			t.Errorf("serial %s: titleDate = %q at %q, want %q", serial, day, at, want)
		}
	}
	if day, _ := titleDate([][]string{{"45000"}}, layout{}); day != "" {
		t.Errorf("a number in a cell that shows no date was read as the date %q", day)
	}
}

func TestMonthYearReadsMonthsJanuaryToDecemberAndYears1900To2099(t *testing.T) {
	for name, want := range map[string]string{
		"01-1900": "1900-01-01", "12-2099": "2099-12-01", "01.2026": "2026-01-01", "122026": "2026-12-01", "2026-12": "2026-12-01",
		"Jan 2026": "2026-01-01", "Disember-2026": "2026-12-01",
		"00-2026": "", "13-2026": "", "12-1899": "", "12-2100": "",
	} {
		if got, ok := monthYear(name); got != want || ok != (want != "") {
			t.Errorf("monthYear(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestGridDaysTakesTwentyEightDaysAsAGridButNotTwentySeven(t *testing.T) {
	header := func(days int) []string {
		h := []string{"Staff"}
		for d := 1; d <= days; d++ {
			h = append(h, strconv.Itoa(d))
		}
		return h
	}
	if first, days := gridDays(header(28)); first != 1 || days != 28 {
		t.Errorf("28 days: gridDays = %d, %d; want 1, 28", first, days)
	}
	if first, days := gridDays(header(27)); first != 0 || days != 0 {
		t.Errorf("27 days: gridDays = %d, %d; want 0, 0", first, days)
	}
	if first, days := gridDays(header(31)[1:]); first != 0 || days != 0 {
		t.Errorf("no column before day 1: gridDays = %d, %d; want 0, 0", first, days)
	}
}
