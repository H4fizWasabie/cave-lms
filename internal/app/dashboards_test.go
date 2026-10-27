package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// labLearner returns the Admin and Ada, who has an active Plan and no Enrollment.
func labLearner(t *testing.T, opts ...func(*config.Config)) (admin, ada *site) {
	admin, ada, planID, _ := labSetup(t, opts...)
	admin.assignPlan("ada@example.com", planID)
	return admin, ada
}

var dashboardPath = regexp.MustCompile(`^/lab/dashboards/(\d+)$`)

// postFile posts a multipart form with one file field named "file".
func (s *site) postFile(path, filename string, content []byte, fields ...url.Values) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range fields {
		for k, vs := range f {
			for _, v := range vs {
				mw.WriteField(k, v)
			}
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		s.t.Fatal(err)
	}
	fw.Write(content)
	mw.Close()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req)
}

// financeSample is a tidy Finance workbook: 65 rows of Tarikh, Perkara, Kategori, Jenis and Jumlah.
func (s *site) financeSample() []byte {
	s.t.Helper()
	return labFile(s.t, "finance.xlsx")
}

// labFile is one of the Lab's test workbooks.
func labFile(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "lab", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLearnerWithAccessCreatesADashboard(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="/lab/new"`)
	expectStatus(t, res, body, http.StatusOK, "No Dashboards yet.")

	res, body = ada.get("/lab/new")
	expectStatus(t, res, body, http.StatusOK, `name="name"`)

	path := ada.createWork("Kewangan <Kedai>")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Kewangan &lt;Kedai&gt;")
	expectStatus(t, res, body, http.StatusOK, "Upload your Dataset")

	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `<a href="`+path+`">Kewangan &lt;Kedai&gt;</a>`)
	if strings.Contains(body, "No Dashboards yet.") {
		t.Fatalf("the Lab list is still empty: %s", body)
	}
}

func TestCreatingADashboardNeedsAName(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.post("/lab/dashboards", url.Values{"name": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give the Dashboard a name")
}

func TestLearnerWithoutAccessCannotCreateADashboard(t *testing.T) {
	_, ada, _, _ := labSetup(t) // no Plan
	res, body := ada.get("/lab")
	if strings.Contains(body, `href="/lab/new"`) {
		t.Fatalf("New Dashboard offered without access: %s", body)
	}
	res, body = ada.get("/lab/new")
	expectRedirect(t, res, body, "/pricing")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Mine"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")

	visitor := newSite(t)
	res, _ = visitor.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Mine"}})
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("a Visitor creating a Dashboard got %d %s, want a redirect to /login", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestPreviewLoadsItsDataAndChartLibraryFromCave(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")

	res, body := ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, `<iframe class="pv-frame" src="`+path+`/view/"`)

	res, page := ada.get(path + "/view/")
	expectStatus(t, res, page, http.StatusOK, `<script src="charts.js"></script>`)
	for _, want := range []string{`fetch("data/dataset-`, `"filters": [`, `"Kategori",`, `"Tarikh"`} {
		expectStatus(t, res, page, http.StatusOK, want)
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Fatalf("the Dashboard page loads something from another site: %s", page)
	}

	res, data := ada.get(path + "/view/" + ada.dataOf(path+"/view/"))
	expectStatus(t, res, data, http.StatusOK, "")
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("data.json Content-Type = %q", ct)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(data), &rows); err != nil || len(rows) != 65 {
		t.Fatalf("data.json: %d rows, %v", len(rows), err)
	}

	res, script := ada.get(path + "/view/charts.js")
	expectStatus(t, res, script, http.StatusOK, "function drawDashboard")
	if ct := res.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Fatalf("charts.js Content-Type = %q", ct)
	}
}

func TestCellTextAndColumnNamesAreEscaped(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "<b>Kategori</b>", "Jumlah"}, []any{day, "<script>alert(1)</script>", 12.5}, []any{day, "Teh", 3})
	res, body := ada.postFile(path+"/dataset", "mine.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "&lt;b&gt;Kategori&lt;/b&gt;")
	if strings.Contains(body, "<script>alert") || strings.Contains(body, "<b>Kategori") {
		t.Fatalf("the upload preview has hostile text unescaped: %s", body)
	}
	ada.confirmWork(path, url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"<b>Kategori</b>"}})
	for _, p := range []string{path + "/preview", path + "/view/", path + "/view/" + ada.dataOf(path+"/view/")} {
		_, body := ada.get(p)
		if strings.Contains(body, "<script>alert") || strings.Contains(body, "<b>Kategori") {
			t.Fatalf("%s has hostile text unescaped: %s", p, body)
		}
	}
}

func TestUploadingSomethingThatIsNotAnXLSXIsRefused(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kewangan")
	res, body := ada.postFile(path+"/dataset", "data.csv", []byte("Tarikh,Jumlah\n2026-01-01,5\n"))
	for _, want := range []string{"data.csv is not an .xlsx file.", "A CSV file stores everything as plain text", "Fix this in Excel, save, and upload again. Nothing changed."} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	res, body = ada.postFile(path+"/dataset", "old.XLS", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This is an old Excel file (Excel 97-2003, .xls).")
	res, body = ada.postFile(path+"/dataset", "renamed.xlsx", []byte("Tarikh,Jumlah\n2026-01-01,5\n"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This is not an .xlsx file.")
	res, body = ada.postFile(path+"/dataset", "under.xlsx", bytes.Repeat([]byte("x"), 10<<20))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This is not an .xlsx file.") // read, not refused for its size
	res, body = ada.postFile(path+"/dataset", "big.xlsx", bytes.Repeat([]byte("x"), 10<<20+1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The Lab reads files up to 10 MB.")
	res, body = ada.postFile(path+"/dataset", "huge.xlsx", bytes.Repeat([]byte("x"), 11<<20))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The Lab reads files up to 10 MB.")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Upload your Dataset")
}

func TestOnlyItsLearnerReachesADashboard(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	data := ada.dataOf(path + "/view/")

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	admin.assignPlan("bob@example.com", admin.onlyPlanID()) // without a Plan the Lab sends him to /pricing first
	for _, p := range []string{path, path + "/preview", path + "/view/", path + "/view/" + data} {
		if res, _ := bob.get(p); res.StatusCode != http.StatusNotFound {
			t.Errorf("Bob got %d for %s, want 404", res.StatusCode, p)
		}
	}
	if res, _ := bob.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample()); res.StatusCode != http.StatusNotFound {
		t.Errorf("Bob uploaded to Ada's Dashboard: %d", res.StatusCode)
	}
}

func TestDeletingALearnerDeletesTheirDashboards(t *testing.T) {
	_, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	// There is no delete-account page yet; deleting the account row must take the Dashboards with it.
	if _, err := ada.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatalf("deleting a Learner with Dashboards failed: %v", err)
	}
}

// Once the Plan ends every Lab screen, the Learner's own Dashboards too, goes to /pricing; changes stay refused.
func TestAfterAccessEndsEveryLabScreenGoesToPricing(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	empty := ada.createWork("Kosong")
	view := path + "/view/" + ada.dataOf(path+"/view/")
	admin.ageAllPeriods(31)

	for _, p := range []string{"/lab", "/lab/new", empty, path, path + "/data", path + "/preview",
		path + "/view/", view, path + "/deploy", path + "/rename", path + "/delete"} {
		res, body := ada.get(p)
		expectRedirect(t, res, body, "/pricing")
	}

	res, body := ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Baru"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
}

var dataAddress = regexp.MustCompile(`fetch\("(data/[A-Za-z0-9_-]+)"\)`)

// dataOf returns the data address the Dashboard page at page (ending in /) fetches, relative to it.
func (s *site) dataOf(page string) string {
	s.t.Helper()
	_, body := s.get(page)
	m := dataAddress.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("%s fetches no data address: %s", page, body)
	}
	return m[1]
}

func TestEachUploadStoresTheDatasetUnderANewAddressAndDeletingRemovesIt(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	first := ada.dataOf(path + "/view/")
	res, data := ada.get(path + "/view/" + first)
	expectStatus(t, res, data, http.StatusOK, `"Tarikh"`)
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=31536000") || !strings.Contains(cc, "immutable") || !strings.Contains(cc, "private") {
		t.Fatalf("the preview's data Cache-Control = %q, want private, a year and immutable", cc)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files after one upload, want 1", n)
	}

	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, []any{day, "Kopi", "Dapur", "Keluar", 4321.5})
	ada.confirmFile(path, "new.xlsx", file)
	second := ada.dataOf(path + "/view/")
	if second == first {
		t.Fatalf("a re-upload kept the data address %s", first)
	}
	if res, _ := ada.get(path + "/view/" + first); res.StatusCode != http.StatusNotFound {
		t.Fatalf("the old data address answers %d, want 404", res.StatusCode)
	}
	res, data = ada.get(path + "/view/" + second)
	expectStatus(t, res, data, http.StatusOK, "4321.5")
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files after a re-upload, want 2: the current Dataset and the earlier one", n)
	}

	res, body := ada.post(path+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Dashboard, want 0", n)
	}
}

// restart starts a new App on the same database and files, as a deploy does, and serves it in place of the old one.
func (s *site) restart() {
	s.t.Helper()
	a, err := app.New(context.Background(), s.cfg, s.mail, s.ai)
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = a.Close() })
	s.server.Config.Handler = a
	s.application = a
}

func TestDatasetsStillInTheDatabaseMoveToTheStoreAtStartup(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	address := ada.dataOf(path + "/view/")
	_, want := ada.get(path + "/view/" + address)

	// Put the Dashboard back the way it was before the store: its Dataset in the dataset column, nothing stored.
	stored := filepath.Join(ada.storageDir, strings.TrimPrefix(address, "data/"))
	dataset, err := os.ReadFile(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.MigrateDownTo(context.Background(), ada.db, 68); err != nil {
		t.Fatal(err)
	}
	if _, err := ada.db.Exec(`UPDATE dashboards SET dataset = $1, dataset_key = NULL`, dataset); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stored); err != nil {
		t.Fatal(err)
	}

	ada.restart()
	res, got := ada.get(path + "/view/" + ada.dataOf(path+"/view/"))
	expectStatus(t, res, got, http.StatusOK, "")
	if got != want {
		t.Fatalf("after the move the data is\n%.300s\nwant\n%.300s", got, want)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files after the move, want 1: the Dataset", n)
	}
	var column bool
	if err := ada.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'dashboards' AND column_name = 'dataset')`).Scan(&column); err != nil || column {
		t.Fatalf("the old dataset column is still there (%v)", err)
	}
}

func TestAFailedStoreWriteTellsTheLearnerAndKeepsThePreviousDataset(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" })
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3")
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.assignPlan("ada@example.com", s.onlyPlanID())
	path := ada.readyDashboard("Kewangan")
	before := ada.dataOf(path + "/view/")

	// A read-only storage directory stands in for R2 refusing the write.
	if err := os.Chmod(s.storageDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.storageDir, 0o700) })
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, []any{day, "Kopi", "Dapur", "Keluar", 4321.5})
	res, body := ada.postFile(path+"/dataset", "new.xlsx", file)
	expectStatus(t, res, body, http.StatusServiceUnavailable, "The upload failed")
	expectStatus(t, res, body, http.StatusServiceUnavailable, "your Dashboard still uses finance-sample.xlsx")

	if after := ada.dataOf(path + "/view/"); after != before {
		t.Fatalf("after a failed upload the Dashboard fetches %s, want the previous %s", after, before)
	}
	if _, log := s.get("/admin/log?level=error"); !strings.Contains(log, "put Dataset") {
		t.Fatalf("the System log does not record the failed write:\n%s", log)
	}
}

func TestALearnerPicksAThemeAndTheDashboardDrawsInIt(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")

	// Every Dashboard starts in Corporate Executive.
	res, body := ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, `<option value="corporate" selected>Corporate Executive</option>`)
	expectStatus(t, res, body, http.StatusOK, `<option value="storytelling">Data Storytelling</option>`)
	res, page := ada.get(path + "/view/")
	expectStatus(t, res, page, http.StatusOK, `"theme": "corporate"`)

	res, body = ada.post(path+"/theme", url.Values{"theme": {"midnight"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		t.Fatalf("picking a Theme redirects to %q, want the preview", loc)
	}
	res, body = ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, `<option value="midnight" selected>Midnight Intelligence</option>`)
	res, page = ada.get(path + "/view/")
	expectStatus(t, res, page, http.StatusOK, `"theme": "midnight"`)

	// Picked at phone width, the preview stays at phone width.
	res, body = ada.post(path+"/theme", url.Values{"theme": {"glass"}, "phone": {"1"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview?phone" {
		t.Fatalf("picking a Theme at phone width redirects to %q, want the phone preview", loc)
	}
	ada.post(path+"/theme", url.Values{"theme": {"midnight"}})

	// A Theme Cave does not offer changes nothing.
	res, body = ada.post(path+"/theme", url.Values{"theme": {"neon"}})
	expectStatus(t, res, body, http.StatusBadRequest, "")
	res, page = ada.get(path + "/view/")
	expectStatus(t, res, page, http.StatusOK, `"theme": "midnight"`)
}
