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
	first := make([]any, len(header))
	for i, h := range header {
		first[i] = h
	}
	return Workbook(t, Sheet{"Sheet1", append([][]any{first}, rows...)})
}

// Sheet is one sheet for Workbook: its name and its rows from row 1.
type Sheet struct {
	Name string
	Rows [][]any
}

// Workbook builds an .xlsx file in memory with the sheets in order, cells written as XLSX writes them.
func Workbook(t testing.TB, sheets ...Sheet) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	format := "dd/mm/yyyy"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range sheets {
		if i == 0 {
			err = f.SetSheetName(f.GetSheetName(0), s.Name)
		} else {
			_, err = f.NewSheet(s.Name)
		}
		if err != nil {
			t.Fatal(err)
		}
		for r, row := range s.Rows {
			for c, v := range row {
				if v == nil {
					continue
				}
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
				if err := f.SetCellValue(s.Name, cell, v); err != nil {
					t.Fatal(err)
				}
				if _, ok := v.(time.Time); ok {
					if err := f.SetCellStyle(s.Name, cell, cell, dateStyle); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
