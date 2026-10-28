package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Changing a Part's columns is what Replace data is for: replacing the only Part is read and checked like replacing one
// of several.
func TestReplacingTheOnlyPartNeedsTheDatasetsColumnsAndShowsNoReplaceAllWarning(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	visitor := ada.second()
	links := replaceLinks(t, ada, path+"/data")

	res, body := ada.postFile(links[0], "markah.xlsx", underTitleRows(t, []any{"Nama", "Markah"}, []any{"Ali", 80.0}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing:")

	res, body = ada.postFile(links[0], "september-betul.xlsx", shopColumns(t, "Tepung", 60))
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Replacing september.xlsx with these 2 rows: 2 rows in all")
	if strings.Contains(body, "Confirming removes") {
		t.Fatalf("a Replace of the only Part warns that the whole Dataset goes: %s", body)
	}
	ada.confirmWork(path, nil)
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, "Tepung") || strings.Contains(body, "Beras") {
		t.Fatalf("data.json = %s, want the new Part in the old one's place", body)
	}

	// Replace data, on the Dataset itself, may change the columns.
	res, body = ada.postFile(path+"/dataset", "markah.xlsx", markahReport(t))
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Confirming removes the 1 Part")
	ada.confirmWork(path, nil)
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, `"Nama": "Ali"`) || strings.Contains(body, "Tepung") {
		t.Fatalf("data.json = %s, want the Replace data's rows", body)
	}
}

// A Learner's own column called Part date would clash with the key Cave adds to every row.
func TestAnUploadWithAColumnCalledPartDateIsRefusedInBothLanguages(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	file := testutil.XLSX(t, []string{"Perkara", "part  DATE"}, []any{"Beras", "2026-01-01"})
	res, body := ada.postFile(path+"/dataset", "september.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Rename the column “part  DATE” in Excel")
	ada.switchLanguage("ms", path+"/data")
	res, body = ada.postFile(path+"/dataset", "september.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Namakan semula lajur “part  DATE” dalam Excel")
	if n := ada.datasetFiles(); n != 0 {
		t.Fatalf("%d stored files after a refused upload, want 0", n)
	}
}

// A Part date the Learner changes in the preview is written under a new key, so the Part's object is never rewritten in
// place; the old object goes.
func TestChangingThePartDateInThePreviewStoresTheRowsUnderANewKeyAndDeletesTheOldObject(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	ada.postFile(path+"/dataset", "Report_20260702091058.xlsx", shopReport(t, "Beras", 120))
	var pending string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dataset_parts`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	ada.post(path+"/dataset/confirm", url.Values{"part-date": {"2026-07-09"}})
	var key, day string
	if err := ada.db.QueryRow(`SELECT storage_key, part_date::text FROM dataset_parts`).Scan(&key, &day); err != nil {
		t.Fatal(err)
	}
	if key == pending || day != "2026-07-09" {
		t.Fatalf("Part stored under %s on %s, want a new key (not %s) and 2026-07-09", key, day, pending)
	}
	var source string
	if err := ada.db.QueryRow(`SELECT date_source FROM dataset_parts`).Scan(&source); err != nil || source != "chosen" {
		t.Fatalf("date source = %q (%v), want chosen", source, err)
	}
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files, want the Part's alone", n)
	}
	var queued int
	if err := ada.db.QueryRow(`SELECT count(*) FROM stored_file_deletions`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("%d files still queued for deletion (%v), want the old object deleted", queued, err)
	}
}
