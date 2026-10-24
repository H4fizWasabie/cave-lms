package testutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Merge merges cell ranges like "A2:A4" on the first sheet of an .xlsx file, as Excel's Merge Cells does:
// the top-left cell's value shows across the range.
func Merge(t testing.TB, file []byte, ranges ...string) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	for _, r := range ranges {
		from, to, _ := strings.Cut(r, ":")
		if err := f.MergeCell(sheet, from, to); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
