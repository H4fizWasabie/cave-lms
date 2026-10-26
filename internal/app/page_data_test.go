package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

var adding = url.Values{"mode": {"add"}}

// shopColumns is shopReport's month with its columns in another order, amount for item, and the same Jualan row.
func shopColumns(t *testing.T, item string, amount float64) []byte {
	t.Helper()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, []string{"Jumlah", "Jenis", "Kategori", "Perkara", "Tarikh"},
		[]any{amount, "Keluar", "Dapur", item, day},
		[]any{900.0, "Masuk", "Kedai", "Jualan", day},
	)
}

// pageWithData is a deployed page Dashboard of Ada's with shopReport's Beras confirmed as september.xlsx: its path
// and link.
func pageWithData(t *testing.T, ada *site) (path, link string) {
	t.Helper()
	path = ada.createPage("Kedai", myPage)
	link = ada.deployAs(path, "kedai-ada")
	res, body := ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectRedirect(t, res, body, path+"/data")
	if _, body = ada.get(path + "/data"); strings.Contains(body, `name="mode"`) {
		t.Fatalf("the first upload offers Replace or Add: %s", body)
	}
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	return path, link
}

func TestAddingToAPageDashboardsDataCombinesTheRowsAndRestoringUndoesIt(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	visitor := ada.second()
	_, body := ada.get(path + "/data")
	expectAll(t, body, `name="mode" value="replace" checked`, `name="mode" value="add"`, "Replace data", "Add to data")

	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Adding 2 rows to your 2: 4 rows in all.", "1 new row exactly matches a row already in your data.",
		`name="mode" value="add" checked`, `action="`+path+`/dataset/confirm"`)
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = visitor.get(link + "data.json")
	expectAll(t, body, `"Perkara": "Beras"`, `"Perkara": "Gula"`)
	if n := strings.Count(body, `"Tarikh": "2026-10-05", "Perkara"`); n != 4 {
		t.Fatalf("data.json has %d rows in the current column order, want 4: %s", n, body)
	}

	september := ada.earlierOn(path+"/data", path)["september.xlsx"]
	res, body = ada.post(september+"/restore", nil)
	expectRedirect(t, res, body, path+"/data")
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Gula") || !strings.Contains(body, "Beras") {
		t.Fatalf("after restoring data.json = %s", body)
	}
}

// Added rows keep the current Dataset's column types: a cell that does not fit its column is left empty and the
// preview says so, as Clean-up does.
func TestAddingDataKeepsTheColumnTypesAndSaysWhichCellsDidNotFit(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	texty := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, "Gula", "Dapur", "Keluar", "banyak"},
		[]any{day, "Garam", "Dapur", "Keluar", 70.0})
	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", texty, adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Cells in Jumlah that were not numbers, left empty: 1.")
	ada.switchLanguage("ms", path)
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Sel dalam Jumlah yang bukan nombor, dibiarkan kosong: 1.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	expectAll(t, body, `"Perkara": "Gula", "Kategori": "Dapur", "Jenis": "Keluar", "Jumlah": null`,
		`"Perkara": "Garam", "Kategori": "Dapur", "Jenis": "Keluar", "Jumlah": 70`)
}

// An Add is put together with the data current at upload: once that changed, confirming it would undo the change,
// so it is refused and the Learner is asked to upload again.
func TestAnAddIsRefusedWhenTheDataChangedSinceItsUpload(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	res, body := ada.postFile(path+"/dataset", "ogos.xlsx", shopReport(t, "Tepung", 50))
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectRedirect(t, res, body, path+"/data")
	september := ada.earlierOn(path+"/data", path)["september.xlsx"]
	res, body = ada.post(september+"/restore", nil)
	expectRedirect(t, res, body, path+"/data")

	res, body = ada.post(path+"/dataset/confirm", nil)
	expectStatus(t, res, body, http.StatusConflict, "Your data changed after you uploaded oktober.xlsx")
	if strings.Contains(body, `action="`+path+`/dataset/confirm"`) {
		t.Fatalf("the refused Add is still offered: %s", body)
	}
	if _, body = ada.second().get(link + "data.json"); strings.Contains(body, "Gula") || strings.Contains(body, "Tepung") || !strings.Contains(body, "Beras") {
		t.Fatalf("after the refused Add data.json = %s", body)
	}

	ada.switchLanguage("ms", path)
	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(ada.earlierOn(path+"/data", path)["ogos.xlsx"]+"/restore", nil)
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectStatus(t, res, body, http.StatusConflict, "Data anda berubah selepas anda memuat naik oktober.xlsx")
}

func TestAddingDataWithOtherColumnsIsRefusedNamingThem(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	other := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Amaun", "Nota"},
		[]any{time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), "Gula", "Dapur", "Keluar", 80.0, "baru"})
	res, body := ada.postFile(path+"/dataset", "lain.xlsx", other, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing: Jumlah.")
	expectAll(t, body, "Not in your data: Amaun, Nota.", "We did not upload lain.xlsx")
	if _, body = ada.get(path + "/data"); strings.Contains(body, "lain.xlsx") {
		t.Fatalf("a refused add was kept: %s", body)
	}
	if _, body = ada.second().get(link + "data.json"); strings.Contains(body, "Gula") {
		t.Fatalf("a refused add changed data.json: %s", body)
	}
}

func TestAnAddPastTheRowLimitIsRefusedWithTheNumbers(t *testing.T) {
	_, ada, _ := storageSite(t, "1024")
	path := ada.createPage("Besar", myPage)
	rows := make([][]any, 49999)
	for i := range rows {
		rows[i] = []any{"x", float64(i)}
	}
	res, body := ada.postFile(path+"/dataset", "banyak.xlsx", testutil.XLSX(t, []string{"Perkara", "Jumlah"}, rows...))
	expectRedirect(t, res, body, path+"/data")
	ada.post(path+"/dataset/confirm", nil)
	more := testutil.XLSX(t, []string{"Jumlah", "Perkara"}, []any{1.0, "y"}, []any{2.0, "z"})
	res, body = ada.postFile(path+"/dataset", "lagi.xlsx", more, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "49,999")
	expectAll(t, body, "50,001", "50,000")
}

func TestAnAddOverTheStorageAllowanceIsRefused(t *testing.T) {
	_, ada, _ := storageSite(t, "1") // 1 MB, and each bigFinance saves as about 600 KB
	path := ada.createPage("Kewangan", myPage)
	res, body := ada.postFile(path+"/dataset", "one.xlsx", bigFinance(t, 1))
	expectRedirect(t, res, body, path+"/data")
	ada.post(path+"/dataset/confirm", nil)
	res, body = ada.postFile(path+"/dataset", "two.xlsx", bigFinance(t, 2))
	expectRedirect(t, res, body, path+"/data") // replacing fits
	res, body = ada.postFile(path+"/dataset", "two.xlsx", bigFinance(t, 2), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
}

func TestReplacingDataWithOtherColumnsWarnsNamingThem(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	lower := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "jumlah"},
		[]any{time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), "Gula", "Dapur", "Keluar", 80.0})
	res, body := ada.postFile(path+"/dataset", "kecil.xlsx", lower)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Gone or renamed: Jumlah.", "New: jumlah.") // data.json's keys are case-sensitive

	other := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Amaun"},
		[]any{time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), "Gula", "Dapur", "Keluar", 80.0})
	res, body = ada.postFile(path+"/dataset", "baru.xlsx", other)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Your page may stop working", "Gone or renamed: Jumlah.", "New: Amaun.")
	ada.switchLanguage("ms", path)
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Hilang atau ditukar nama: Jumlah.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if _, body = ada.second().get(link + "data.json"); !strings.Contains(body, `"Amaun": 80`) {
		t.Fatalf("after confirming the new columns data.json = %s", body)
	}
}

func TestAPageDashboardsDataIsUploadedConfirmedAndServedAtDataJSON(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	link := ada.deployAs(path, "kedai-ada")
	visitor := ada.second()

	res, body := visitor.get(link + "data.json")
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.get(path + "/data")
	expectStatus(t, res, body, http.StatusOK, "september.xlsx")
	expectAll(t, body, "Columns: Tarikh, Perkara, Kategori, Jenis, Jumlah",
		"Upload all your rows", `action="`+path+`/dataset/confirm"`)
	if strings.Contains(body, `name="template"`) || strings.Contains(body, "/lab/samples/") {
		t.Fatalf("a page Dashboard's preview offers Templates or Samples: %s", body)
	}

	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	res, body = visitor.get(link + "data.json")
	expectStatus(t, res, body, http.StatusOK, `"Perkara": "Beras"`)
	for h, want := range map[string]string{
		"Access-Control-Allow-Origin": "*",
		"Cache-Control":               "public, max-age=300",
		"Content-Type":                "application/json",
	} {
		if got := res.Header.Get(h); got != want {
			t.Fatalf("data.json %s = %q, want %q", h, got, want)
		}
	}

	// A second upload with the same columns still shows the preview, without the first-upload note or a Template.
	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", shopReport(t, "Gula", 80))
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "oktober.xlsx", "Columns: Tarikh")
	if strings.Contains(body, "Upload all your rows") || strings.Contains(body, `name="template"`) {
		t.Fatalf("a later upload shows the first-upload note or a Template: %s", body)
	}
	ada.post(path+"/dataset/confirm", nil)
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, `"Perkara": "Gula"`) {
		t.Fatalf("after the second confirm data.json = %s", body)
	}

	// Sleeping, the link serves no rows.
	admin.ageAllPeriods(31)
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Perkara") {
		t.Fatalf("an asleep Deployment's data.json = %s", body)
	}
	admin.assignPlan("ada@example.com", admin.onlyPlanID())

	september := ada.earlierOn(path+"/data", path)["september.xlsx"]
	res, body = ada.post(september+"/restore", nil)
	expectRedirect(t, res, body, path+"/data")
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, `"Perkara": "Beras"`) {
		t.Fatalf("after restoring data.json = %s", body)
	}
}

func TestThePageAndDataScreensGiveTheStandardSentenceToCopyInBMAndEN(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	for _, c := range []struct{ lang, sentence, copy, readsOnly string }{
		{"en", "Load the rows from ./data.json when it exists, using the Excel header names", ">Copy<", "Your page changes only if it reads data.json."},
		{"ms", "Muatkan baris daripada ./data.json jika ia wujud, menggunakan nama tajuk lajur Excel", ">Salin<", "Halaman anda hanya berubah jika ia membaca data.json."},
	} {
		ada.switchLanguage(c.lang, path)
		for _, screen := range []string{path, path + "/data"} {
			res, body := ada.get(screen)
			expectStatus(t, res, body, http.StatusOK, c.sentence)
			expectAll(t, body, `data-copy="data-sentence"`, c.copy)
		}
		_, body := ada.get(path + "/data")
		expectAll(t, body, c.readsOnly)
	}
	res, body := ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Muat naik semua baris anda", "Lajur: Tarikh")
}

func TestAPageDashboardsDataOverTheStorageAllowanceIsRefused(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	path := ada.createPage("Besar", strings.Repeat("z", 900<<10))
	res, body := ada.postFile(path+"/dataset", "data.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
	expectAll(t, body, `action="`+path+`/dataset"`, "Upload your data")
}
