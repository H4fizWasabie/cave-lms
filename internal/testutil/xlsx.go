package testutil

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// XLSX builds an .xlsx file in memory: header in row 1, then rows. A time.Time cell is written as a
// real Excel date (formatted dd/mm/yyyy, as Excel in Malaysia shows it); nil leaves the cell empty.
func XLSX(t testing.TB, header []string, rows ...[]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	format := "dd/mm/yyyy"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
	if err != nil {
		t.Fatal(err)
	}
	put := func(col, row int, v any) {
		cell, _ := excelize.CoordinatesToCellName(col+1, row)
		if v == nil {
			return
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			t.Fatal(err)
		}
		if _, ok := v.(time.Time); ok {
			if err := f.SetCellStyle(sheet, cell, cell, dateStyle); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, h := range header {
		put(i, 1, h)
	}
	for r, row := range rows {
		for c, v := range row {
			put(c, r+2, v)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
