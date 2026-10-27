package app_test

import (
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// monthNamedReport is augustReport with BULAN written as a month name, as the real August export does.
func monthNamedReport(t *testing.T, month any, schools ...[2]string) []byte {
	t.Helper()
	var rows [][]any
	for _, s := range schools {
		rows = append(rows, []any{"JOHOR", "Batu Pahat", s[0], s[1], "RENDAH", "SK", "T1", "1 Amanah", 2026.0, month, "31/08/2026", 120.0, 108.0, 12.0, 90.0})
	}
	return testutil.XLSX(t, augustColumns, rows...)
}

// June's BULAN is 6, a month column. August's, mapped to the Dataset's columns, says OGOS on every row: the Part is
// added with BULAN = 8, the preview lists the names read as numbers on their own, apart from the cells left empty, and
// the recipe keeps the month column, so September's SEP is read the same way after the August Part is confirmed.
func TestAMapPartOfMonthNamesAddsAsTheMonthNumberInTheDatasetsMonthColumn(t *testing.T) {
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA2002", "SK Jaya"}, [2]string{"JBA2003", "SK Baru"}), adding)
	res, body := postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Month names in BULAN read as numbers: 2.")
	if strings.Contains(body, "that were not numbers") {
		t.Fatalf("the month names are counted among the cells left empty: %s", body)
	}
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")

	res, body = ada.postFile(path+"/dataset", "kehadiran_092026.xlsx", monthNamedReport(t, "Sep", [2]string{"JBA3003", "SK Lagi"}), adding)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Month names in BULAN read as numbers: 1.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")

	_, body = ada.second().get(link + "data.json")
	rows := dataRows(t, body)
	if len(rows) != 4 {
		t.Fatalf("data.json has %d rows, want 4: %s", len(rows), body)
	}
	for i, want := range []float64{6, 8, 8, 9} {
		if rows[i]["BULAN"] != want {
			t.Fatalf("data.json row %d has BULAN = %v, want %v: %s", i+1, rows[i]["BULAN"], want, body)
		}
	}
}
