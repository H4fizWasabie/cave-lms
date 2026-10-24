package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// createWork creates a Work Dashboard from a Template and returns its path.
func (s *site) createWork(template, name string) string {
	s.t.Helper()
	res, body := s.post("/lab/dashboards", url.Values{"kind": {"work"}, "template": {template}, "name": {name}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	loc := res.Header.Get("Location")
	if !dashboardPath.MatchString(loc) {
		s.t.Fatalf("created Work Dashboard redirects to %q", loc)
	}
	return loc
}

// shopReport is a Finance-like report as kept at work: two title rows and a blank row above the header.
func shopReport(t *testing.T, item string, amount float64) []byte {
	t.Helper()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, []string{"Kedai Runcit Ali"},
		[]any{"Laporan Oktober 2026"},
		[]any{},
		[]any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, item, "Dapur", "Keluar", amount},
		[]any{day, "Jualan", "Kedai", "Masuk", 900.0},
	)
}

// confirmWork confirms the pending Dataset of the Work Dashboard at path with Cave's guesses.
func (s *site) confirmWork(path string, choices url.Values) {
	s.t.Helper()
	res, body := s.post(path+"/dataset/confirm", choices)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// rowsOf fetches the data the Dashboard page at page (ending in /) draws.
func (s *site) rowsOf(page string) string {
	s.t.Helper()
	res, body := s.get(page + s.dataOf(page))
	expectStatus(s.t, res, body, http.StatusOK, "")
	return body
}

var financeGuess = url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Kategori"}, "type": {"Jenis"}, "description": {"Perkara"}}

func TestTheLabOffersBothKindsAndLabelsEachDashboard(t *testing.T) {
	_, ada := labLearner(t)
	practice := ada.readyDashboard("Latihan")
	res, body := ada.get("/lab")
	for _, want := range []string{`href="/lab/new"`, `href="/lab/new?kind=work"`, "Practice Dashboard", "Work Dashboard"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	work := ada.createWork("hr", "Staf")
	_, body = ada.get("/lab")
	for path, kind := range map[string]string{practice: "Practice", work: "Work"} {
		row := body[strings.Index(body, `<a href="`+path+`">`):]
		row = row[:strings.Index(row, "</li></ul>")]
		if !strings.Contains(row, `<li>`+kind+`</li>`) {
			t.Fatalf("%s is not labelled %s: %s", path, kind, row)
		}
	}

	res, body = ada.get("/lab/new?kind=work")
	for _, want := range []string{`name="kind" value="work"`, "Finance", "Inventory", "HR", "General"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

func TestAWorkDashboardUploadIsCleanedPreviewedAndConfirmed(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")

	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	for _, want := range []string{
		"Skipped rows 1 to 3 above the header in row 4.",
		"laporan.xlsx", "Beras", "Jualan",
		`<option value="Tarikh" selected>Tarikh</option>`, `<option value="Jumlah" selected>Jumlah</option>`,
		`action="` + path + `/dataset/confirm"`,
	} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	// Pending: nothing to preview or deploy yet.
	res, body = ada.get(path + "/view/")
	expectStatus(t, res, body, http.StatusNotFound, "")

	ada.switchLanguage("ms", path)
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Baris 1 hingga 3 di atas tajuk lajur di baris 4 dilangkau.")
	ada.switchLanguage("en", path)

	res, body = ada.post(path+"/dataset/confirm", url.Values{"date": {""}, "amount": {"Jumlah"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose a column for Date.")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `value="general"`)

	ada.confirmWork(path, financeGuess)
	page := ada.rowsOf(path + "/view/")
	_, index := ada.get(path + "/view/")
	for _, want := range []string{`"work": true`, `"group": "Kategori"`, `"vals": [`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the Work Dashboard page lacks %s:\n%s", want, index)
		}
	}
	if !strings.Contains(page, `"Perkara": "Beras"`) {
		t.Fatalf("data = %s", page)
	}
	res, body = ada.get(path + "/hood")
	expectStatus(t, res, body, http.StatusOK, "data.json")
}

func TestUntilConfirmTheDashboardAndDeploymentKeepTheOldDataset(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, financeGuess)
	link := ada.deploy(path)
	viewer := ada.second()

	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", shopReport(t, "Gula", 80))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for _, page := range []string{ada.rowsOf(path + "/view/"), viewer.rowsOf(link)} {
		if !strings.Contains(page, "Beras") || strings.Contains(page, "Gula") {
			t.Fatalf("before confirming, data = %s", page)
		}
	}
	ada.confirmWork(path, nil) // the blocks are already the Dashboard's
	for _, page := range []string{ada.rowsOf(path + "/view/"), viewer.rowsOf(link)} {
		if !strings.Contains(page, "Gula") || strings.Contains(page, "Beras") {
			t.Fatalf("after confirming, data = %s", page)
		}
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want only the current Dataset", n)
	}
}

func TestAWorkDashboardOnGeneralNeedsNoChoices(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "senarai.xlsx", testutil.XLSX(t, []string{"Hari", "Nota"}, []any{day, "Buka"}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", url.Values{"template": {"general"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "General Template")
	_, index := ada.get(path + "/view/")
	if !strings.Contains(index, `"cols": [`) {
		t.Fatalf("General built no table:\n%s", index)
	}
}

func TestAnUnreadableWorkUploadGivesOneMessage(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", []byte("Tarikh,Jumlah\n05/11/2026,RM5\n"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "We could not read laporan.xlsx")
	if n := strings.Count(body, "CSV file stores everything"); n != 1 {
		t.Fatalf("the message shows %d times: %s", n, body)
	}
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after an unreadable upload", n)
	}
}

func TestTheStorageAllowanceCountsConfirmedWorkDatasetsBySavedSize(t *testing.T) {
	_, ada, _ := storageSite(t, "1024")
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 GB") // pending does not count

	ada.confirmWork(path, financeGuess)
	entries, err := os.ReadDir(ada.storageDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stored files %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, fmt.Sprintf("%d KB of 1 GB", (info.Size()+1023)>>10))
}

func TestAConfirmOverTheStorageAllowanceIsRefused(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	practice := ada.createDashboard("finance", "Latihan")
	res, body := ada.postFile(practice+"/dataset", "one.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	path := ada.createWork("finance", "Kedai")
	res, body = ada.postFile(path+"/dataset", "two.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", financeGuess)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
}

func TestUnconfirmedWorkDatasetsArePruned(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.prune()
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("a fresh pending Dataset was pruned: %d stored files", n)
	}
	if _, err := admin.db.ExecContext(context.Background(), `UPDATE datasets SET uploaded_at = uploaded_at - interval '2 days'`); err != nil {
		t.Fatal(err)
	}
	admin.prune()
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after pruning an old pending Dataset", n)
	}
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Upload your Dataset")
}

func TestDeletingAWorkDashboardDeletesItsDatasets(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "a.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, financeGuess)
	res, body = ada.postFile(path+"/dataset", "b.xlsx", shopReport(t, "Gula", 80))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want current and pending", n)
	}
	res, body = ada.post(path+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Dashboard", n)
	}
}

func TestSalesAndRosterAreOfferedForWorkDashboardsOnly(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab/new?kind=work&t=roster")
	for _, want := range []string{`href="/lab/new?kind=work&amp;t=sales"`, "<code>Kod</code>", `href="/lab/samples/roster.xlsx"`, `src="/lab/demo/roster/"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	_, body = ada.get("/lab/new")
	if strings.Contains(body, "t=sales") || strings.Contains(body, "t=roster") {
		t.Fatalf("New Practice Dashboard offers Sales or Roster:\n%s", body)
	}
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"sales"}, "name": {"Jualan"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")

	_, demo := ada.get("/lab/demo/roster/")
	if !strings.Contains(demo, `"work": true`) || !strings.Contains(demo, `"by": "day"`) {
		t.Fatalf("the Roster demo is not drawn as a Work Dashboard:\n%s", demo)
	}

	path := ada.createWork("sales", "Kafe")
	_, sample := ada.get("/lab/samples/sales.xlsx")
	res, body = ada.postFile(path+"/dataset", "jualan.xlsx", []byte(sample))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "item": {"Produk"}, "category": {"Kategori"},
		"staff": {"Staf"}, "payment": {"Kaedah bayaran"}, "quantity": {"Kuantiti"}})
	_, index := ada.get(path + "/view/")
	for _, want := range []string{"What sells, and when?", `"label": "Revenue by payment method"`, `"group": "Kaedah bayaran"`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the Sales Dashboard lacks %s:\n%s", want, index)
		}
	}
}
