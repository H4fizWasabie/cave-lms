package lab

import (
	"reflect"
	"testing"
)

// A merge that starts past the data, or past the end of its row, or runs past the last row, is left alone.
func TestFillDownIgnoresMergesPastTheEdgesOfTheCells(t *testing.T) {
	cells := [][]string{{"h"}, {"x"}}
	res := &CleanUp{}
	got, err := fillDown([][4]int{{2, 0, 3, 0}, {1, 1, 2, 1}}, cells, 0, res)
	if err != nil || len(res.Changes) != 0 || !reflect.DeepEqual(got, [][]string{{"h"}, {"x"}}) {
		t.Fatalf("got %q, %v, %v", got, res.Changes, err)
	}
	// A merge running past the last row stops at it.
	res = &CleanUp{}
	got, err = fillDown([][4]int{{0, 0, 5, 0}}, [][]string{{"a"}, {""}}, 0, res)
	if err != nil || !reflect.DeepEqual(got, [][]string{{"a"}, {"a"}}) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestJoinHeaderStopsAMergeAtTheEndOfItsRow(t *testing.T) {
	// A merge starting at the row's width, or reaching beyond it, adds no columns.
	cells := [][]string{{"A", "B"}, {"", ""}}
	got := joinHeader([][4]int{{0, 2, 0, 4}, {0, 1, 0, 5}}, cells, 0, &CleanUp{})
	if want := []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
}

func TestCutOffKeepsAColumnThatStartsAfterABlankHeaderCell(t *testing.T) {
	cells := [][]string{{"", "A"}, {"", "1"}}
	got, err := cutOff(cells, 0, 1, &CleanUp{})
	if err != nil || len(got[0]) != 2 {
		t.Fatalf("got %q, %v; want both columns kept", got, err)
	}
}

func TestCutOffNotesNothingWhenOnlyBlankRowsFollowTheTable(t *testing.T) {
	res := &CleanUp{}
	if _, err := cutOff([][]string{{"A", "B"}, {"1", "2"}, {}}, 0, 1, res); err != nil || len(res.Changes) != 0 {
		t.Fatalf("changes = %v, %v; want none", res.Changes, err)
	}
}

func TestDropEmptyListsOnlyTheNamedColumns(t *testing.T) {
	res := &CleanUp{}
	header, raw := dropEmpty([]string{"A", " "}, [][]cell{{{v: "x"}, {}}}, res)
	if len(res.Changes) != 0 || !reflect.DeepEqual(header, []string{"A"}) || len(raw[0]) != 1 {
		t.Fatalf("header %q, raw %v, changes %v", header, raw, res.Changes)
	}
}

func TestDropBottomKeepsTheOnlyRowEvenIfItIsATotal(t *testing.T) {
	res := &CleanUp{}
	raw, rowNo := dropBottom([][]cell{{{v: "Total"}, {v: "5"}}}, []int{2}, res)
	if len(raw) != 1 || len(rowNo) != 1 || len(res.Changes) != 0 {
		t.Fatalf("raw %v, changes %v", raw, res.Changes)
	}
}

func TestTableColsStopsAtSixColumns(t *testing.T) {
	var ps []profile
	cols := make([]Column, 8)
	for i := range cols {
		n := string(rune('a' + i))
		cols[i] = Column{n, Text}
		ps = append(ps, profile{Column: cols[i], Role: Dimension})
	}
	if got := tableCols(&Dataset{Columns: cols}, ps); len(got) != 6 {
		t.Fatalf("got %v, want 6 columns", got)
	}
}

func TestReadLayoutKeepsOnlyStyledColumns(t *testing.T) {
	l := readLayout(styledSheet(t, `<cols><col min="1" max="2" style="1"/><col min="3" max="3" style="0"/><col min="4" max="4"/></cols><row r="1"><c r="A1"><v>1</v></c></row>`), "Sheet1")
	if want := []styledCols{{1, 2, true}}; !reflect.DeepEqual(l.cols, want) {
		t.Fatalf("cols = %v, want %v", l.cols, want)
	}
}
