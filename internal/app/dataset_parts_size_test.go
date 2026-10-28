package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// bigReport is a shopReport-shaped workbook of n rows (no title rows) for item.
func bigReport(t *testing.T, item string, n int) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, bigRows(item, n)...)
}

// bigRows is n rows of shopReport's columns for item.
func bigRows(item string, n int) [][]any {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	rows := make([][]any, n)
	for i := range rows {
		rows[i] = []any{day, fmt.Sprintf("%s %d", item, i), "Dapur", "Keluar", float64(i + 1)}
	}
	return rows
}

// bigReportTitled is bigReport under shopReport's title rows, the header on row 4 as pageWithData's first Part has it.
func bigReportTitled(t *testing.T, item string, n int) []byte {
	t.Helper()
	return underTitleRows(t, []any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, bigRows(item, n)...)
}

func TestAnOwnPageDashboardAddsPartsPastFiftyThousandRowsAndDataJSONReturnsEveryRow(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	for _, name := range []string{"oktober.xlsx", "november.xlsx"} {
		res, body := ada.postFile(path+"/dataset", name, bigReportTitled(t, name, 25000), adding)
		expectRedirect(t, res, body, path+"/data")
		res, body = ada.post(path+"/dataset/confirm", nil)
		expectRedirect(t, res, body, path+"/data")
	}
	_, body := ada.get(path + "/data")
	expectAll(t, body, "3 Parts", "50002 rows")
	_, body = ada.second().get(link + "data.json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil || len(rows) != 50002 {
		t.Fatalf("data.json has %d rows (%v), want 50002", len(rows), err)
	}
}

func TestAThemeBuiltDashboardRefusesAnAddPastFiftyThousandRowsInTotalAndChangesNothing(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", bigReport(t, "Beras", 25001))
	before := ada.rowsOf(path + "/view/")

	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", bigReport(t, "Gula", 25000), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity,
		"Adding these 25,000 rows to your 25,001 makes 50,001. A Dashboard built in a Theme holds up to 50,000 rows. Replace or delete a Part to make room.")
	ada.switchLanguage("ms", path)
	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", bigReport(t, "Gula", 25000), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity,
		"Menambah 25,000 baris ini pada 25,001 baris anda menjadikannya 50,001. Dashboard yang dibina dalam Tema memuatkan sehingga 50,000 baris. Gantikan atau padam satu Bahagian untuk memberi ruang.")
	if after := ada.rowsOf(path + "/view/"); after != before {
		t.Fatal("a refused add changed the Dashboard's data")
	}
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files, want the one Part", n)
	}
}

// The Dataset tab, Page tab and Deploy tab know a Dashboard's Parts from their records and columns; with every Part's
// file gone from the store they still render.
func TestTheDashboardTabsRenderWithoutReadingAnyPartFromStorage(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	page, _ := pageWithData(t, ada)
	ada.addFile(page, "oktober.xlsx", shopColumns(t, "Gula", 80))
	built := ada.createWork("Kedai")
	ada.confirmFile(built, "september.xlsx", shopReport(t, "Beras", 120))
	ada.addFile(built, "oktober.xlsx", shopReport(t, "Gula", 80))

	files, err := filepath.Glob(filepath.Join(ada.storageDir, "dataset-*"))
	if err != nil || len(files) != 4 {
		t.Fatalf("%d stored Parts (%v), want 4", len(files), err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}
	for _, tab := range []string{page, page + "/data", page + "/deploy", built, built + "/deploy"} {
		res, body := ada.get(tab)
		expectStatus(t, res, body, http.StatusOK, "")
		if strings.Contains(body, "went wrong") {
			t.Fatalf("%s: %s", tab, body)
		}
	}
}
