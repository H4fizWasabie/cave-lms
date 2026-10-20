package app_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/xuri/excelize/v2"
)

// labLearner returns the Admin and Ada, who has an active Plan and has joined a Course that includes the Lab.
func labLearner(t *testing.T) (admin, ada *site) {
	admin, ada, planID, labCourse, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+labCourse+"/join", nil)
	return admin, ada
}

var dashboardPath = regexp.MustCompile(`^/lab/dashboards/(\d+)$`)

// createDashboard creates a Dashboard from a Template and returns its path, /lab/dashboards/{id}.
func (s *site) createDashboard(template, name string) string {
	s.t.Helper()
	res, body := s.post("/lab/dashboards", url.Values{"template": {template}, "name": {name}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	loc := res.Header.Get("Location")
	if !dashboardPath.MatchString(loc) {
		s.t.Fatalf("created Dashboard redirects to %q", loc)
	}
	return loc
}

// postFile posts a multipart form with one file field named "file".
func (s *site) postFile(path, filename string, content []byte) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
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

// financeSample downloads the Finance Sample.
func (s *site) financeSample() []byte {
	s.t.Helper()
	res, body := s.get("/lab/samples/finance.xlsx")
	expectStatus(s.t, res, body, http.StatusOK, "")
	return []byte(body)
}

func TestLearnerWithAccessCreatesAFinanceDashboard(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="/lab/new"`)
	expectStatus(t, res, body, http.StatusOK, "No Dashboards yet.")

	res, body = ada.get("/lab/new")
	for _, want := range []string{"Finance", `href="/lab/samples/finance.xlsx"`, "Date", "Amount", "Category", "Type", `name="name"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}

	path := ada.createDashboard("finance", "Kewangan <Kedai>")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Kewangan &lt;Kedai&gt;")
	expectStatus(t, res, body, http.StatusOK, "Upload your Dataset")

	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `<a href="`+path+`">Kewangan &lt;Kedai&gt;</a>`)
	if strings.Contains(body, "No Dashboards yet.") {
		t.Fatalf("the Lab list is still empty: %s", body)
	}
}

func TestCreatingADashboardNeedsANameAndAKnownTemplate(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give the Dashboard a name")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"astrology"}, "name": {"Stars"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose a Template")
}

func TestLearnerWithoutAccessCannotCreateADashboard(t *testing.T) {
	_, ada, _, _, _ := labSetup(t) // no Plan
	res, body := ada.get("/lab")
	if strings.Contains(body, `href="/lab/new"`) {
		t.Fatalf("New Dashboard offered without access: %s", body)
	}
	res, body = ada.get("/lab/new")
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Mine"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")

	visitor := newSite(t)
	res, _ = visitor.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Mine"}})
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("a Visitor creating a Dashboard got %d %s, want a redirect to /login", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestFinanceSampleDownloadsAsAnXLSXWithMalayColumns(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/lab/samples/finance.xlsx")
	expectStatus(t, res, body, http.StatusOK, "")
	if ct := res.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, `attachment; filename="finance-sample.xlsx"`) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	f, err := excelize.OpenReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rows[0], ","); got != "Tarikh,Perkara,Kategori,Jenis,Jumlah" {
		t.Fatalf("header = %s", got)
	}
	res, _ = s.get("/lab/samples/astrology.xlsx")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown Sample: %d, want 404", res.StatusCode)
	}
}

func TestUploadingTheSamplePrefillsEveryBlankAndTheLearnerCanChangeThem(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	res, body := ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/columns" {
		t.Fatalf("upload redirects to %q, want the columns page", loc)
	}

	res, body = ada.get(path + "/columns")
	for blank, col := range map[string]string{"date": "Tarikh", "amount": "Jumlah", "category": "Kategori", "type": "Jenis", "description": "Perkara"} {
		chosen := regexp.MustCompile(`(?s)<select [^>]*name="` + blank + `".*?<option value="` + col + `" selected>`)
		if !chosen.MatchString(body) {
			t.Errorf("%s is not pre-filled with %s", blank, col)
		}
	}
	expectStatus(t, res, body, http.StatusOK, "65 rows")

	res, body = ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Perkara"}, "type": {""}, "description": {"Kategori"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		t.Fatalf("saving columns redirects to %q, want the preview", loc)
	}
	res, body = ada.get(path + "/columns")
	for _, want := range []string{`name="type"`, `<option value="" selected>Skip</option>`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if !regexp.MustCompile(`(?s)name="category".*?<option value="Perkara" selected>`).MatchString(body) {
		t.Fatalf("Category did not change to Perkara: %s", body)
	}
	_, page := ada.get(path + "/view/")
	if strings.Contains(page, `"Money in"`) || !strings.Contains(page, `"Total"`) {
		t.Fatalf("skipping Type should drop the money-in cards and show Total: %s", page)
	}

	res, body = ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Kategori"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Amount")
}

func TestPreviewLoadsItsDataAndChartLibraryFromCave(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())

	res, body := ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, `<iframe class="pv-frame" src="`+path+`/view/"`)

	res, page := ada.get(path + "/view/")
	expectStatus(t, res, page, http.StatusOK, `<script src="charts.js"></script>`)
	for _, want := range []string{`fetch("data.json")`, `"filters": [`, `"category",`, `"date"`} {
		expectStatus(t, res, page, http.StatusOK, want)
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Fatalf("the Dashboard page loads something from another site: %s", page)
	}

	res, data := ada.get(path + "/view/data.json")
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
	path := ada.createDashboard("finance", "Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "<b>Kategori</b>", "Jumlah"}, []any{day, "<script>alert(1)</script>", 12.5})
	res, body := ada.postFile(path+"/dataset", "mine.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get(path + "/columns")
	expectStatus(t, res, body, http.StatusOK, "&lt;b&gt;Kategori&lt;/b&gt;")
	ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"<b>Kategori</b>"}})
	for _, p := range []string{path + "/columns", path + "/preview", path + "/view/", path + "/view/data.json"} {
		_, body := ada.get(p)
		if strings.Contains(body, "<script>alert") || strings.Contains(body, "<b>Kategori") {
			t.Fatalf("%s has hostile text unescaped: %s", p, body)
		}
	}
}

func TestUploadingSomethingThatIsNotAnXLSXIsRefused(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	res, body := ada.postFile(path+"/dataset", "data.csv", []byte("Tarikh,Jumlah\n2026-01-01,5\n"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The Lab reads .xlsx files only")
	res, body = ada.get(path + "/columns")
	expectStatus(t, res, body, http.StatusOK, "Upload a Dataset first")
}

func TestOnlyTheOwnerReachesADashboard(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	for _, p := range []string{path, path + "/columns", path + "/preview", path + "/view/", path + "/view/data.json"} {
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
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	// There is no delete-account page yet; deleting the account row must take the Dashboards with it.
	if _, err := ada.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatalf("deleting a Learner with Dashboards failed: %v", err)
	}
}
