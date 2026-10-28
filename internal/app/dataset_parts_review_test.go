package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// A new Part is read with the first Part's sheet, but its header row is found afresh, as its title rows may differ.
func TestAPartIsReadWithItsOwnHeaderRowWhenTheFirstPartsWasFoundByCave(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := pageWithData(t, ada) // shopReport: the header is on row 4
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	short := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, "Gula", "Dapur", "Keluar", 10.0}, []any{day, "Garam", "Dapur", "Keluar", 11.0}, []any{day, "Lada", "Dapur", "Keluar", 12.0})
	res, body := ada.postFile(path+"/dataset", "november.xlsx", short, adding)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Adding 3 rows to your 2: 5 rows in all.")
}

// A header row the Learner typed for the first Part is read again for every Part.
func TestAPartIsReadWithTheFirstPartsHeaderRowWhenTheLearnerTypedIt(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	// Row 1 looks like a header too; the Learner says the header is row 2.
	layout := func(item string) []byte {
		return testutil.XLSX(t, []string{"Perkara", "Jumlah"}, []any{"Perkara", "Jumlah"}, []any{item, 5.0})
	}
	res, body := ada.postFile(path+"/dataset", "september.xlsx", layout("Beras"), url.Values{"header": {"2"}})
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)

	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", layout("Gula"), adding)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Adding 1 row to your 1: 2 rows in all.")
}

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

// date_source says where each Part's date came from: the sources Clean-up finds, the upload day and the Learner's own.
func TestEachPartRecordsWhereItsDateCameFrom(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	sources := func() string {
		rows, err := ada.db.Query(`SELECT date_source FROM dataset_parts WHERE state = 'current' ORDER BY position`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return strings.Join(out, ",")
	}
	noDate := func(item string) []byte {
		return testutil.XLSX(t, []string{"Stock Balance Report"}, []any{}, []any{"Perkara", "Baki", "Unit"}, []any{item, 5, "kg"})
	}
	ada.postFile(path+"/dataset", "stok_20260702.xlsx", stockReport(t, "Gula", "01.09.2026"))
	ada.confirmWork(path, nil)
	ada.postFile(path+"/dataset", "stok_20260703.xlsx", noDate("Beras"), adding)
	ada.confirmWork(path, nil)
	ada.postFile(path+"/dataset", "stok.xlsx", noDate("Garam"), adding)
	ada.confirmWork(path, nil)
	ada.postFile(path+"/dataset", "stok2.xlsx", noDate("Lada"), adding)
	ada.confirmWork(path, url.Values{"part-date": {"2026-07-09"}})
	if got, want := sources(), "cell,file,upload,chosen"; got != want {
		t.Fatalf("date sources = %s, want %s", got, want)
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
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files, want the Part's alone", n)
	}
	var queued int
	if err := ada.db.QueryRow(`SELECT count(*) FROM stored_file_deletions`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("%d files still queued for deletion (%v), want the old object deleted", queued, err)
	}
}
