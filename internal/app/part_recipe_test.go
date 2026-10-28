package app_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// shopSheet is a sheet of shopReport's kind named name: title rows, the header on row 4, then one row per item.
func shopSheet(name string, items ...string) testutil.Sheet {
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	s := testutil.Sheet{Name: name, Rows: [][]any{{"Kedai Runcit Ali"}, {"Laporan November 2026"}, {},
		{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}}}
	for _, item := range items {
		s.Rows = append(s.Rows, []any{day, item, "Dapur", "Keluar", 10.0})
	}
	return s
}

// perkara is data.json's Perkara of every row, in order.
func perkara(t *testing.T, body string) []string {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("data.json is not one array: %v\n%s", err, body)
	}
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprint(r["Perkara"]))
	}
	return out
}

// With no sheet by the recipe's name the preview offers only the sheets with the Dataset's columns, and the one the
// Learner picks is added.
func TestAnAddWithoutTheRecipeSheetOffersTheSheetsWithTheDatasetsColumnsAndAddsThePickedOne(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)

	notes := testutil.Sheet{Name: "Nota", Rows: [][]any{{"Kumpulan", "Catatan"}, {"A", "B"}}}
	file := testutil.Workbook(t, notes, shopSheet("Okt", "Gula"), shopSheet("Nov", "Garam", "Lada"))
	res, body := ada.postFile(path+"/dataset", "november.xlsx", file, adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, `name="sheet" value="Okt" checked`, `name="sheet" value="Nov"`)
	if strings.Contains(body, `value="Nota"`) {
		t.Fatalf("the picker offers a sheet with other columns: %s", body)
	}
	if strings.Contains(body, `name="combine"`) {
		t.Fatalf("the picker offers to combine sheets: %s", body)
	}

	picked := url.Values{"mode": {"add"}, "sheet": {"Nov"}, "shown": {"Okt"}}
	res, body = ada.postFile(path+"/dataset", "november.xlsx", file, picked)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Adding 2 rows to your 2: 4 rows in all.", `name="sheet" value="Nov" checked`)
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	if got, want := perkara(t, body), []string{"Beras", "Jualan", "Garam", "Lada"}; !slices.Equal(got, want) {
		t.Fatalf("data.json Perkara = %v, want %v", got, want)
	}
}

// Replace data is cleaned as the first Part is, with sheets combined, and sets the recipe the next Add follows.
func TestReplacingDataCombinesSheetsAndSetsANewRecipe(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)

	both := testutil.Workbook(t, shopSheet("Okt", "Gula"), shopSheet("Nov", "Garam"))
	res, body := ada.postFile(path+"/dataset", "semua.xlsx", both)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Combined 2 sheets")

	res, body = ada.postFile(path+"/dataset", "okt.xlsx", testutil.Workbook(t, shopSheet("Okt", "Gula")))
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")

	// The recipe's sheet is now Okt, not Sheet1.
	more := testutil.Workbook(t, shopSheet("Sheet1", "Salah"), shopSheet("OKT", "Lada"))
	res, body = ada.postFile(path+"/dataset", "bulan.xlsx", more, adding)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	if got, want := perkara(t, body), []string{"Gula", "Lada"}; !slices.Equal(got, want) {
		t.Fatalf("data.json Perkara = %v, want %v", got, want)
	}
}

// A Dataset Cave built from several sheets has a Sheet column; a later Add reads one sheet, so the column is filled
// from that sheet's name and the Add is not refused as missing it.
func TestAnAddToADatasetOfCombinedSheetsFillsTheSheetColumnFromItsSheetName(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	both := testutil.Workbook(t, shopSheet("Okt", "Gula"), shopSheet("Nov", "Garam"))
	res, body := ada.postFile(path+"/dataset", "semua.xlsx", both)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")

	res, body = ada.postFile(path+"/dataset", "lagi.xlsx", testutil.Workbook(t, shopSheet("okt", "Lada")), adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Filled the Sheet column of your data from the sheet name: okt.")
	ada.switchLanguage("ms", path)
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Lajur Sheet dalam data anda diisi daripada nama helaian: okt.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	expectAll(t, body, `"Sheet": "okt"`, `"Perkara": "Lada"`)
}

// A renamed sheet that also has an extra column is still offered: it has every column the Dataset has.
func TestAnAddWithoutTheRecipeSheetOffersASheetWithExtraColumns(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := pageWithData(t, ada)
	wide := shopSheet("Okt", "Gula")
	wide.Rows[3] = append(wide.Rows[3], "Nota")
	wide.Rows[4] = append(wide.Rows[4], "baru")
	res, body := ada.postFile(path+"/dataset", "november.xlsx", testutil.Workbook(t, testutil.Sheet{Name: "Nota", Rows: [][]any{{"Kumpulan", "Catatan"}, {"A", "B"}}}, wide), adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, `name="sheet" value="Okt" checked`, "Columns not in your data, dropped (1): Nota.")
}
