package app_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// forgetMonthColumns takes the month columns out of the recipe, as a Dataset confirmed before they were recorded has it.
func forgetMonthColumns(t *testing.T, ada *site) {
	t.Helper()
	if _, err := ada.db.Exec(`UPDATE dashboards SET upload_settings = upload_settings #- '{recipe,monthColumns}'`); err != nil {
		t.Fatal(err)
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
	var logged int
	if err := ada.db.QueryRow(`SELECT count(*) FROM system_log WHERE level = 'error' AND path = $1`, path+"/dataset").Scan(&logged); err != nil || logged == 0 {
		t.Fatalf("%d error lines logged for the store's refusal (%v), want one", logged, err)
	}
}
