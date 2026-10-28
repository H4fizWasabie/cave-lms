package app_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// recipeMonthColumns is the Dashboard's recipe month columns as stored: "null" when the recipe never recorded them.
func recipeMonthColumns(t *testing.T, ada *site) string {
	t.Helper()
	var raw string
	if err := ada.db.QueryRow(`SELECT COALESCE(upload_settings->'recipe'->'monthColumns', 'null'::jsonb)::text FROM dashboards`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// forgetMonthColumns takes the month columns out of the recipe, as a Dataset confirmed before they were recorded has it.
func forgetMonthColumns(t *testing.T, ada *site) {
	t.Helper()
	if _, err := ada.db.Exec(`UPDATE dashboards SET upload_settings = upload_settings #- '{recipe,monthColumns}'`); err != nil {
		t.Fatal(err)
	}
}

// A Part with a 13 in BULAN is added and confirmed: BULAN is no longer a month column, so the Dataset's values still
// decide. A later Part that writes OGOS gets it left empty and listed, as it would on a Dataset that never had a month
// column.
func TestAConfirmedPartWithAnotherNumberInAMonthColumnDropsItFromTheRecipe(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	if got := recipeMonthColumns(t, ada); !strings.Contains(got, `"BULAN"`) {
		t.Fatalf("the recipe's month columns after June = %s, want BULAN in it", got)
	}
	_, refused := ada.postFile(path+"/dataset", "kehadiran_072026.xlsx", monthNamedReport(t, 13.0, [2]string{"JBA2002", "SK Jaya"}), adding)
	res, body := postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if got := recipeMonthColumns(t, ada); strings.Contains(got, `"BULAN"`) {
		t.Fatalf("the recipe's month columns after a 13 in BULAN = %s, want BULAN dropped", got)
	}

	res, body = ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA3003", "SK Lagi"}), adding)
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Cells in BULAN that were not numbers, left empty: 1.")
	if strings.Contains(body, "Month names in") {
		t.Fatalf("the name is read as a month in a column that is no longer one: %s", body)
	}
}

// The Dataset module's Upload finds a Dataset's month columns once and reads month names with them; these check what a
// confirm then records in the recipe, which moves to the module with the confirm (#460).

// A Dataset confirmed before month columns were recorded has none in its recipe: the confirm of the first Add saves the
// ones the upload found, BULAN being 6 in every row of June.
func TestConfirmingTheFirstAddToADatasetWithoutRecordedMonthColumnsRecordsThem(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	forgetMonthColumns(t, ada)

	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA2002", "SK Jaya"}), adding)
	res, body := postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Month names in BULAN read as numbers: 1.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if got := recipeMonthColumns(t, ada); !strings.Contains(got, `"BULAN"`) { // JUMTHADIR, 10, is whole 1 to 12 too
		t.Fatalf("the recipe's month columns after the confirm = %s, want BULAN in it", got)
	}
	_, body = ada.second().get(link + "data.json")
	rows := dataRows(t, body)
	if len(rows) != 2 || rows[1]["BULAN"] != 8.0 {
		t.Fatalf("data.json = %s, want August's BULAN = 8 after June's row", body)
	}
}

// A Dataset whose Parts have no month column (BULAN holds a 13) gets "none" recorded by the confirm, not left as never
// found.
func TestConfirmingTheFirstAddToADatasetWithNoMonthColumnRecordsNone(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kehadiran", myPage)
	odd := testutil.XLSX(t, juneColumns, []any{"Batu Pahat", "JBA1001", "SK Maju", "T1", "1 Amanah", 2026.0, 13.0, 100.0, 90.0, 50.0, 90.0})
	res, body := ada.postFile(path+"/dataset", "kehadiran_062026.xlsx", odd)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	forgetMonthColumns(t, ada)

	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA2002", "SK Jaya"}), adding)
	res, body = postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if got := recipeMonthColumns(t, ada); got != `[]` {
		t.Fatalf("the recipe's month columns after the confirm = %s, want []", got)
	}
}

// Replacing the only Part, whose 13 in BULAN is going away, is decided by the new Part: the confirm records BULAN.
func TestConfirmingAReplaceOfTheOnlyPartRecordsTheNewPartsMonthColumns(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	replace := replaceAction.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	odd := testutil.XLSX(t, juneColumns, []any{"Batu Pahat", "JBA1001", "SK Maju", "T1", "1 Amanah", 2026.0, 13.0, 100.0, 90.0, 50.0, 90.0})
	res, body := ada.postFile(replace, "kehadiran_062026.xlsx", odd)
	expectRedirect(t, res, body, path+"/data")
	ada.post(path+"/dataset/confirm", nil)
	forgetMonthColumns(t, ada)

	replace = replaceAction.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	_, refused := ada.postFile(replace, "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA2002", "SK Jaya"}))
	res, body = postMapping(t, ada, replace, refused, augustPicks, nil)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if got := recipeMonthColumns(t, ada); !strings.Contains(got, `"BULAN"`) {
		t.Fatalf("the recipe's month columns after the Replace = %s, want BULAN in it", got)
	}
	_, body = ada.second().get(link + "data.json")
	if rows := dataRows(t, body); len(rows) != 1 || rows[0]["BULAN"] != 8.0 {
		t.Fatalf("data.json = %s, want the one Part with BULAN = 8", body)
	}
}

// A store that cannot give a Part to the one-time scan answers as every other upload does when the store fails: the
// 503 page, not a bare 500.
func TestAStoreThatFailsWhileFindingMonthColumnsGivesTheFailedPage(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	forgetMonthColumns(t, ada)
	files, err := filepath.Glob(filepath.Join(ada.storageDir, "dataset-*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("%d stored Parts (%v), want some", len(files), err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}
	res, body := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", monthNamedReport(t, "OGOS", [2]string{"JBA2002", "SK Jaya"}), adding)
	expectStatus(t, res, body, http.StatusServiceUnavailable, "The upload failed")
}
