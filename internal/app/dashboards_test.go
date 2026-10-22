package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
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
	"github.com/xuri/excelize/v2"
)

// labLearner returns the Admin and Ada, who has an active Plan and no Enrollment.
func labLearner(t *testing.T) (admin, ada *site) {
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
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
	_, ada, _, _ := labSetup(t) // no Plan
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
	for _, want := range []string{`fetch("data/dataset-`, `"filters": [`, `"category",`, `"date"`} {
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
	path := ada.createDashboard("finance", "Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "<b>Kategori</b>", "Jumlah"}, []any{day, "<script>alert(1)</script>", 12.5})
	res, body := ada.postFile(path+"/dataset", "mine.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get(path + "/columns")
	expectStatus(t, res, body, http.StatusOK, "&lt;b&gt;Kategori&lt;/b&gt;")
	ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"<b>Kategori</b>"}})
	for _, p := range []string{path + "/columns", path + "/preview", path + "/view/", path + "/view/" + ada.dataOf(path+"/view/")} {
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
	for _, want := range []string{"data.csv is not an .xlsx file.", "A CSV file stores everything as plain text", "Fix this in Excel, save, and upload again. Nothing changed."} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	res, body = ada.postFile(path+"/dataset", "renamed.xlsx", []byte("Tarikh,Jumlah\n2026-01-01,5\n"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This is not an .xlsx file.")
	res, body = ada.postFile(path+"/dataset", "under.xlsx", bytes.Repeat([]byte("x"), 10<<20))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "This is not an .xlsx file.") // read, not refused for its size
	res, body = ada.postFile(path+"/dataset", "big.xlsx", bytes.Repeat([]byte("x"), 10<<20+1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The Lab reads files up to 10 MB.")
	res, body = ada.postFile(path+"/dataset", "huge.xlsx", bytes.Repeat([]byte("x"), 11<<20))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The Lab reads files up to 10 MB.")
	res, body = ada.get(path + "/columns")
	expectStatus(t, res, body, http.StatusOK, "Upload a Dataset first")
}

func TestOnlyItsLearnerReachesADashboard(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	data := ada.dataOf(path + "/view/")

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	for _, p := range []string{path, path + "/columns", path + "/preview", path + "/view/", path + "/view/" + data} {
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

// unzip reads a zip download into file name -> contents.
func unzip(t *testing.T, body string) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(strings.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(b)
	}
	return files
}

func TestDownloadZipHasThePageItsDataAndTheChartLibrary(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan Kedai")
	if res, _ := ada.get(path + "/download"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("a Dashboard with no Dataset downloads: %d", res.StatusCode)
	}
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())

	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/download"`)
	res, body = ada.get(path + "/download")
	expectStatus(t, res, body, http.StatusOK, "")
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("zip Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="kewangan-kedai.zip"` {
		t.Fatalf("zip Content-Disposition = %q", cd)
	}
	files := unzip(t, body)
	address := ada.dataOf(path + "/view/")
	_, index := ada.get(path + "/view/")
	_, data := ada.get(path + "/view/" + address)
	_, charts := ada.get(path + "/view/charts.js")
	// The zip's page reads data.json beside it; the preview's reads the stored Dataset's address.
	index = strings.Replace(index, `fetch("`+address+`")`, `fetch("data.json")`, 1)
	if files["index.html"] != index || files["data.json"] != data || files["charts.js"] != charts {
		t.Fatalf("zip files differ from the preview's: %v", keys(files))
	}
	// Browsers block fetch from a file opened on the computer, so the page falls back to data.js.
	for _, want := range []string{`fetch("data.json")`, `"data.js"`} {
		if !strings.Contains(index, want) {
			t.Fatalf("index.html lacks %s: %s", want, index)
		}
	}
	js, ok := strings.CutPrefix(files["data.js"], "var dashboardRows = ")
	js, ok2 := strings.CutSuffix(js, ";\n")
	var rows []map[string]any
	if !ok || !ok2 || json.Unmarshal([]byte(js), &rows) != nil || len(rows) != 65 {
		t.Fatalf("data.js does not hold the 65 rows: %.200s", files["data.js"])
	}
	if len(files) != 4 {
		t.Fatalf("zip holds %v, want index.html, data.json, data.js and charts.js", keys(files))
	}
}

func keys(m map[string]string) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}

var hoodCode = regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`)

func TestUnderTheHoodShowsTheGeneratedPageAndDataExplained(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())

	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/hood"`)
	res, body = ada.get(path + "/hood")
	for _, want := range []string{"The page&#39;s details", "The layout", "The chart library", "Your column choices", "Load the data, then draw", "Your spreadsheet, as data"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	parts := hoodCode.FindAllStringSubmatch(body, -1)
	if len(parts) != 6 {
		t.Fatalf("want 5 parts of index.html and data.json, got %d: %s", len(parts), body)
	}
	var page string
	for _, p := range parts[:5] {
		page += html.UnescapeString(p[1])
	}
	if _, index := ada.get(path + "/view/"); page != index {
		t.Fatalf("the hood's index.html is not the preview's:\n%s\n---\n%s", page, index)
	}
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/view/`+ada.dataOf(path+"/view/")+`"`)
	data := html.UnescapeString(parts[5][1])
	if !strings.Contains(data, `{"Tarikh": "2026-`) || !strings.Contains(data, "62 more rows") {
		t.Fatalf("data.json excerpt: %s", data)
	}
}

func TestAfterAccessEndsTheLabIsReadOnly(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	empty := ada.createDashboard("finance", "Kosong")
	admin.ageAllPeriods(31)

	res, body := ada.get("/lab")
	for _, want := range []string{"Your Dashboards are safe", `src="/lab/demo/finance/"`, `href="` + path + `"`, `href="` + path + `/download"`, `href="` + empty + `"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, `href="/lab/new"`) || strings.Contains(body, `href="`+empty+`/download"`) {
		t.Fatalf("read-only Lab offers New Dashboard, or a zip of a Dashboard with no Dataset: %s", body)
	}

	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "this Dashboard is read-only")
	for _, p := range []string{path, path + "/columns"} {
		_, body := ada.get(p)
		if strings.Contains(body, `action="`+path+`/dataset"`) || strings.Contains(body, `action="`+path+`/columns"`) {
			t.Fatalf("%s still offers a form: %s", p, body)
		}
	}
	for _, p := range []string{path + "/preview", path + "/hood", path + "/view/", path + "/view/" + ada.dataOf(path+"/view/"), path + "/download"} {
		if res, body := ada.get(p); res.StatusCode != http.StatusOK {
			t.Errorf("%s = %d without access, want 200: %.300s", p, res.StatusCode, body)
		}
	}

	res, body = ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
	res, body = ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Baru"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")
}

func TestTheErrorPageListsEveryProblemAndChangesNothing(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())

	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	bad := testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, []any{day, "dua ratus"}, []any{"31/02/2026", 5}, []any{day, nil})
	res, body := ada.postFile(path+"/dataset", "bad.xlsx", bad)
	for _, want := range []string{"We could not read bad.xlsx", "Row 2, Jumlah: not a number (&#34;dua ratus&#34;).", "Row 3, Tarikh: not a date",
		"Row 4, Jumlah: empty, but this column is required.", "Fix these in Excel, save, and upload again. Nothing changed: your Dashboard still uses finance-sample.xlsx."} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}

	res, body = ada.postFile(path+"/dataset", "renamed.xlsx", testutil.XLSX(t, []string{"Tarikh", "Jumlah Baru"}, []any{day, "x"}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Column Jumlah is missing. The Finance Template needs it for Amount, a column of numbers. Did you rename it?")

	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "finance-sample.xlsx")
	expectStatus(t, res, body, http.StatusOK, "65 rows")
}

func TestReuploadKeepsTheColumnChoicesWhileTheyStillFit(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Perkara"}, "description": {"Kategori"}})

	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	same := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jumlah"}, []any{day, "Kopi", "Makan", "RM 12.50"}, []any{"06/03/2026", "Teh", "Makan", "RM3"})
	res, body := ada.postFile(path+"/dataset", "march.xlsx", same)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		t.Fatalf("a matching re-upload redirects to %q, want the preview", loc)
	}
	_, data := ada.get(path + "/view/" + ada.dataOf(path+"/view/"))
	if !strings.Contains(data, `"Jumlah": 12.5`) || strings.Count(data, "Tarikh") != 2 {
		t.Fatalf("data.json not updated: %s", data)
	}
	res, body = ada.get(path + "/columns")
	if !regexp.MustCompile(`(?s)name="category".*?<option value="Perkara" selected>`).MatchString(body) {
		t.Fatalf("the Learner's Category choice was lost: %s", body)
	}

	moved := testutil.XLSX(t, []string{"Tarikh", "Kumpulan", "Jumlah"}, []any{day, "Makan", 12.5})
	res, body = ada.postFile(path+"/dataset", "april.xlsx", moved)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/columns?again" {
		t.Fatalf("a re-upload without the chosen columns redirects to %q, want the columns page", loc)
	}
	res, body = ada.get(path + "/columns?again")
	expectStatus(t, res, body, http.StatusOK, "Your new file does not have every column you chose")
	expectStatus(t, res, body, http.StatusOK, `<option value="" selected>Skip</option>`)
}

func TestARenamedRequiredColumnIsFlaggedOnTheColumnsPage(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "renamed.xlsx", testutil.XLSX(t, []string{"Tarikh", "Nilai"}, []any{day, 5}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path + "/columns")
	expectStatus(t, res, body, http.StatusOK, "Cave could not find Jumlah, so it picked Nilai for Amount. Did you rename it?")
	if strings.Contains(body, "could not find Tarikh") {
		t.Fatalf("Tarikh was found by name: %s", body)
	}
}

func TestBlankCellsAreCheckedInTheChosenRequiredColumn(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	ada.postFile(path+"/dataset", "first.xlsx", testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Lain"}, []any{day, 1, 2}, []any{day, 3, nil}))

	res, body := ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Lain"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Row 3, Lain: empty, but this column is required.")

	full := testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Lain"}, []any{day, 1, 2}, []any{day, 3, 4})
	ada.postFile(path+"/dataset", "full.xlsx", full)
	res, body = ada.post(path+"/columns", url.Values{"date": {"Tarikh"}, "amount": {"Lain"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.postFile(path+"/dataset", "gap.xlsx", testutil.XLSX(t, []string{"Tarikh", "Jumlah", "Lain"}, []any{day, 1, 2}, []any{day, 3, nil}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Row 3, Lain: empty, but this column is required.")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "your Dashboard still uses full.xlsx")
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
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
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
	res, body := ada.postFile(path+"/dataset", "new.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	second := ada.dataOf(path + "/view/")
	if second == first {
		t.Fatalf("a re-upload kept the data address %s", first)
	}
	if res, _ := ada.get(path + "/view/" + first); res.StatusCode != http.StatusNotFound {
		t.Fatalf("the old data address answers %d, want 404", res.StatusCode)
	}
	res, data = ada.get(path + "/view/" + second)
	expectStatus(t, res, data, http.StatusOK, "4321.5")
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files after a re-upload, want 1", n)
	}

	res, body = ada.post(path+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Dashboard, want 0", n)
	}
}

// restart starts a new App on the same database and files, as a deploy does, and serves it in place of the old one.
func (s *site) restart() {
	s.t.Helper()
	a, err := app.New(context.Background(), s.cfg, s.mail)
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = a.Close() })
	s.server.Config.Handler = a
}

func TestDatasetsStillInTheDatabaseMoveToTheStoreAtStartup(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
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
		t.Fatalf("%d stored files after the move, want 1", n)
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
	s.createPlan("Lab Monthly", "30", "3", "")
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.assignPlan("ada@example.com", s.onlyPlanID())
	path := ada.createDashboard("finance", "Kewangan")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
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
