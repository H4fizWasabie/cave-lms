package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

var adding = url.Values{"mode": {"add"}}

// underTitleRows is a workbook of shopReport's layout: two title rows and a blank one, then the header on row 4.
func underTitleRows(t *testing.T, header []any, rows ...[]any) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Kedai Runcit Ali"}, append([][]any{{"Laporan Oktober 2026"}, {}, header}, rows...)...)
}

// shopColumns is shopReport's month with its columns in another order, amount for item, and the same Jualan row. Its
// header is on row 4, as shopReport's is: a new Part is read with the first Part's header row.
func shopColumns(t *testing.T, item string, amount float64) []byte {
	t.Helper()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, []string{"Kedai Runcit Ali"},
		[]any{"Laporan Oktober 2026"},
		[]any{},
		[]any{"Jumlah", "Jenis", "Kategori", "Perkara", "Tarikh"},
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

// partDate is the day a Part uploaded now is dated.
func partDate() string { return time.Now().UTC().Format(time.DateOnly) }

func TestConfirmingAnAddCreatesASecondPartAndDataJSONReturnsBothPartsInOrderEachRowWithItsPartDate(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	visitor := ada.second()
	_, body := ada.get(path + "/data")
	expectAll(t, body, `name="mode" value="add" checked`, `name="mode" value="replace"`, "Replace data", "Add to data")

	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Adding 2 rows to your 2: 4 rows in all.", `name="mode" value="add" checked`, `action="`+path+`/dataset/confirm"`)
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Gula") {
		t.Fatalf("before confirming, data.json = %s", body)
	}
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "2 Parts", "4 rows")

	_, body = visitor.get(link + "data.json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("data.json is not one array: %v\n%s", err, body)
	}
	var got []string
	for _, r := range rows {
		got = append(got, fmt.Sprint(r["Perkara"], " ", r["Part date"]))
	}
	today := partDate()
	want := []string{"Beras " + today, "Jualan " + today, "Gula " + today, "Jualan " + today}
	if !slices.Equal(got, want) {
		t.Fatalf("data.json rows = %v, want %v", got, want)
	}
	if n := strings.Count(body, `"Tarikh": "2026-10-05", "Perkara"`); n != 4 {
		t.Fatalf("data.json has %d rows in the current column order, want 4: %s", n, body)
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want one per Part", n)
	}
}

// Added rows keep the current Dataset's column types: a cell that does not fit its column is left empty and the
// preview says so, as Clean-up does.
func TestAddingDataKeepsTheColumnTypesAndSaysWhichCellsDidNotFit(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	texty := underTitleRows(t, []any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
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

// A Part may have columns the Dataset lacks, in any order: the preview lists them as dropped, and only the Dataset's
// columns reach data.json.
func TestAddingDataWithExtraColumnsDropsThemListsThemAndKeepsTheDatasetsColumns(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	extra := underTitleRows(t, []any{"Nota", "Jumlah", "Jenis", "Rak", "Kategori", "Perkara", "Warna", "Tarikh"},
		[]any{"baru", 80.0, "Keluar", "A1", "Dapur", "Gula", "putih", time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
	res, body := ada.postFile(path+"/dataset", "lebih.xlsx", extra, adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Columns not in your data, dropped (3): Nota, Rak, Warna.")
	ada.switchLanguage("ms", path)
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Lajur yang tiada dalam data anda, dibuang (3): Nota, Rak, Warna.")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	expectAll(t, body, `"Perkara": "Gula", "Kategori": "Dapur", "Jenis": "Keluar", "Jumlah": 80`)
	for _, dropped := range []string{"Nota", "Rak", "Warna"} {
		if strings.Contains(body, dropped) {
			t.Fatalf("data.json holds the dropped column %s: %s", dropped, body)
		}
	}
}

func TestAddingDataLackingAColumnIsRefusedNamingItAndChangesNothing(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	short := underTitleRows(t, []any{"Tarikh", "Perkara", "Kategori", "Jenis", "Nota"},
		[]any{time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), "Gula", "Dapur", "Keluar", "baru"})
	res, body := ada.postFile(path+"/dataset", "kurang.xlsx", short, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing: Jumlah.")
	expectAll(t, body, "We did not upload kurang.xlsx")
	ada.switchLanguage("ms", path)
	res, body = ada.postFile(path+"/dataset", "kurang.xlsx", short, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Hilang: Jumlah.")
	if strings.Contains(partItems(t, mustGet(t, ada, path+"/data")), "kurang.xlsx") { // its Workbook is kept, and picked from the Folder
		t.Fatalf("a refused add was kept: %s", body)
	}
	if _, body = ada.second().get(link + "data.json"); strings.Contains(body, "Gula") {
		t.Fatalf("a refused add changed data.json: %s", body)
	}
}

func TestReplacingDataWithOtherColumnsWarnsNamingThem(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
		`action="`+path+`/dataset/confirm"`)
	if strings.Contains(body, "Upload all your rows") {
		t.Fatalf("the preview shows the outdated all-rows hint: %s", body)
	}
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

	// Sleeping stops data.json however many Parts it has; so does a Takedown.
	res, body = ada.postFile(path+"/dataset", "november.xlsx", shopColumns(t, "Garam", 70), adding)
	expectRedirect(t, res, body, path+"/data")
	ada.post(path+"/dataset/confirm", nil)
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, "Gula") || !strings.Contains(body, "Garam") {
		t.Fatalf("after adding, data.json = %s", body)
	}
	_, body = admin.get("/admin/deployments")
	res, body = admin.post(takedownAction.FindStringSubmatch(body)[1], url.Values{"reason": {"Borang palsu"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Perkara") {
		t.Fatalf("a taken-down Deployment's data.json = %s", body)
	}
}

func TestThePageAndDataScreensGiveTheStandardSentenceToCopyInBMAndEN(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	for _, c := range []struct{ lang, sentence, copy, readsOnly string }{
		{"en", "Load the rows from ./data.json when it exists, using the Excel header names as the field names, and the Part date field to tell the uploads apart", ">Copy<", "Your page changes only if it reads data.json."},
		{"ms", "Muatkan baris daripada ./data.json jika ia wujud, menggunakan nama tajuk lajur Excel sebagai nama medan, dan medan Part date untuk membezakan setiap muat naik", ">Salin<", "Halaman anda hanya berubah jika ia membaca data.json."},
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
	expectAll(t, body, "Lajur: Tarikh")
	if strings.Contains(body, "Muat naik semua baris anda") {
		t.Fatalf("the Malay preview shows the outdated all-rows hint: %s", body)
	}
}
