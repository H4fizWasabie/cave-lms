package app_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// A Part with a 13 in BULAN is added and confirmed: BULAN is no longer a month column, so the Dataset's values still
// decide. A later Part that writes OGOS gets it left empty and listed, as it would on a Dataset that never had a month
// column.
func TestAConfirmedPartWithAnotherNumberInAMonthColumnDropsItFromTheRecipe(t *testing.T) {
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

// Replace one Part leaves out the Part being replaced when it looks for the month columns: the only Part has a 13 in
// BULAN, which is going away, so the new Part's own values decide and its OGOS is read as 8.
func TestReplacingTheOnlyPartOfADatasetWithoutRecordedMonthColumnsIsDecidedByTheNewPart(t *testing.T) {
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
	expectAll(t, mustGet(t, ada, path+"/data"), "Month names in BULAN read as numbers: 1.")
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
