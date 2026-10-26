package app_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const myPage = `<!doctype html><title>Jualan</title><h1>Jualan bulan ini</h1>`

// createPage creates a Dashboard called name from pasted page HTML and returns its path.
func (s *site) createPage(name, page string) string {
	s.t.Helper()
	res, body := s.post("/lab/dashboards", url.Values{"name": {name}, "html": {page}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	loc := res.Header.Get("Location")
	if dashboardPath.FindStringSubmatch(loc) == nil {
		s.t.Fatalf("creating a page Dashboard went to %q", loc)
	}
	return loc
}

// dropPage deletes the page of the Dashboard with id, record and stored file, leaving an engine Dashboard.
func (s *site) dropPage(id string) {
	s.t.Helper()
	var key string
	if err := s.db.QueryRow(`DELETE FROM dashboard_pages WHERE dashboard_id = $1 RETURNING storage_key`, id).Scan(&key); err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM stored_file_deletions WHERE storage_key = $1`, key); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.storageDir, key)); err != nil {
		s.t.Fatal(err)
	}
}

// pageCSP is the policy a Deployment's own page is served under (ADR 0017).
const pageCSP = "sandbox allow-scripts; default-src 'none'; " +
	"script-src 'unsafe-inline' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://unpkg.com; " +
	"style-src 'unsafe-inline' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://unpkg.com https://fonts.googleapis.com; " +
	"font-src data: https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://unpkg.com https://fonts.gstatic.com; " +
	"img-src data: https:; connect-src 'self' https://cave.example.invalid; form-action 'none'; frame-ancestors 'none'"

func TestAPlanHolderPastesHTMLDeploysAndAVisitorGetsItSandboxed(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", myPage)
	link := ada.deployAs(path, "jualan-ada")

	res, body := ada.second().get(link)
	expectStatus(t, res, body, http.StatusOK, myPage)
	if body != myPage {
		t.Fatalf("the Visitor got %q, want the page as pasted", body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp != pageCSP {
		t.Fatalf("page CSP = %q", csp)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=300" {
		t.Fatalf("page Cache-Control = %q, want 5 minutes", cc)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("page Content-Type = %q", ct)
	}

	// A browser posts the form with an empty file part when no file is picked: the paste is used.
	res, body = ada.postFile("/lab/dashboards", "", nil, url.Values{"name": {"Tampal"}, "html": {myPage}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestAPlanHolderPicksAnHTMLFileForTheirPage(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab/new")
	expectStatus(t, res, body, http.StatusOK, "")
	expectAll(t, body, `enctype="multipart/form-data"`, `name="html"`, `type="file" name="file"`, "Paste the HTML")

	res, body = ada.postFile("/lab/dashboards", "jualan.html", []byte(myPage), url.Values{"name": {"Jualan"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	path := res.Header.Get("Location")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Jualan")
	expectAll(t, body, "Your page", "Uploaded ", `href="`+path+`/deploy"`)
	link := ada.deployAs(path, "jualan-fail")
	if _, page := ada.second().get(link); page != myPage {
		t.Fatalf("the Visitor got %q, want the file as picked", page)
	}
}

func TestAPageThatIsEmptyTooBigNotHTMLOrOverTheAllowanceIsRefusedAndNothingIsSaved(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	big := strings.Repeat("x", 5<<20+1)
	for _, c := range []struct {
		name string
		post func() (*http.Response, string)
		want string
	}{
		{"empty paste", func() (*http.Response, string) {
			return ada.post("/lab/dashboards", url.Values{"name": {"Jualan"}, "html": {"  \n"}})
		}, "Paste your page&#39;s HTML or pick a .html file."},
		{"no name", func() (*http.Response, string) {
			return ada.post("/lab/dashboards", url.Values{"name": {" "}, "html": {myPage}})
		}, "Give the Dashboard a name."},
		{"wrong type", func() (*http.Response, string) {
			return ada.postFile("/lab/dashboards", "jualan.xlsx", []byte(myPage), url.Values{"name": {"Jualan"}})
		}, "jualan.xlsx is not a .html file."},
		{"big file", func() (*http.Response, string) {
			return ada.postFile("/lab/dashboards", "jualan.html", []byte(big), url.Values{"name": {"Jualan"}})
		}, "The page is over 5 MB."},
		{"file over the request cap", func() (*http.Response, string) {
			return ada.postFile("/lab/dashboards", "jualan.html", []byte(strings.Repeat("x", 6<<20)), url.Values{"name": {"Jualan"}})
		}, "The page is over 5 MB."},
		{"big paste", func() (*http.Response, string) {
			return ada.post("/lab/dashboards", url.Values{"name": {"Jualan"}, "html": {big + big}})
		}, "The page is over 5 MB."},
		{"over the allowance", func() (*http.Response, string) {
			return ada.post("/lab/dashboards", url.Values{"name": {"Jualan"}, "html": {strings.Repeat("y", 1100<<10)}})
		}, "would take you over your Storage allowance"},
	} {
		res, body := c.post()
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		var n int
		if err := ada.db.QueryRow(`SELECT count(*) FROM dashboards`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 || ada.storedFiles() != 0 {
			t.Fatalf("%s: refused, but %d Dashboards and %d stored files were saved", c.name, n, ada.storedFiles())
		}
	}
	res, body := ada.postAsking("/lab/dashboards", "ms", url.Values{"name": {"Kosong"}, "html": {""}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tampal HTML halaman anda atau pilih fail .html.")
	expectAll(t, body, `value="Kosong"`)
}

func TestTheLabListsPageDashboardsWithTheirLinkAwakeStateAndLastChange(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createPage("Jualan", myPage)
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "")
	expectAll(t, body, `href="`+path+`"`, "Jualan", "Not deployed", "Changed ")

	link := ada.deployAs(path, "jualan-ada")
	_, body = ada.get("/lab")
	expectAll(t, body, `href="`+link+`"`, "https://cave.example.invalid"+link, "Online")
	if strings.Contains(body, "Not deployed") || strings.Contains(body, "Template") {
		t.Fatalf("a deployed page Dashboard is listed as not deployed or with a Template: %s", body)
	}

	_, body = admin.get("/admin/deployments")
	admin.post(takedownAction.FindStringSubmatch(body)[1], url.Values{"reason": {"Borang palsu"}})
	_, body = ada.get("/lab")
	expectAll(t, body, "Taken down ")
}

func TestPagesCountInTheStorageTotalAndAgainstDatasetUploads(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	ada.createPage("Besar", strings.Repeat("z", 900<<10))
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, strings.TrimSuffix(ada.storedKB(), " of 1 GB")+" of 1 MB")

	path := ada.createWork("finance", "Kewangan")
	res, body = ada.postFile(path+"/dataset", "data.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
}

func TestAPageDeploymentSleepsWakesIsTakenDownRenamedAndDeleted(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createPage("Jualan", myPage)
	link := ada.deployAs(path, "jualan-ada")
	visitor := ada.second()

	admin.ageAllPeriods(31)
	res, body := visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, "This Dashboard is asleep")
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	if _, page := visitor.get(link); page != myPage {
		t.Fatalf("after renewing, the link shows %q", page)
	}

	res, body = ada.post(path+"/rename", url.Values{"name": {"Jualan 2026"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/lab")
	expectAll(t, body, "Jualan 2026")
	if _, page := visitor.get(link); page != myPage {
		t.Fatalf("after renaming, the link shows %q", page)
	}

	_, body = admin.get("/admin/deployments")
	expectAll(t, body, "Jualan 2026")
	res, body = admin.post(takedownAction.FindStringSubmatch(body)[1], url.Values{"reason": {"Borang palsu"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, "Cave took it offline early")
	if body == myPage {
		t.Fatalf("a taken-down link still shows the page")
	}

	res, body = ada.post(path+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if res, _ = visitor.get(link); res.StatusCode != http.StatusNotFound {
		t.Fatalf("a deleted Dashboard's link answered %d", res.StatusCode)
	}
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("deleting the Dashboard left %d stored files", n)
	}
}

func TestWithoutAPlanTheLabAndTheDemoGoToPricingButTheAdminKeepsTheLab(t *testing.T) {
	admin, ada, _, _ := labSetup(t)
	for _, who := range []*site{ada, ada.second()} {
		for _, p := range []string{"/lab", "/lab/demo/finance/", "/lab/demo/finance/data.json", "/lab/demo/finance/charts.js"} {
			res, body := who.get(p)
			expectRedirect(t, res, body, "/pricing")
		}
	}
	res, body := admin.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "My Dashboards")
	admin.deployAs(admin.createPage("Admin", myPage), "admin-jualan")
}
