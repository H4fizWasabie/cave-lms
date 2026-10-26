package app_test

import (
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"time"

	"context"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readyDashboard creates a Finance Dashboard with the Sample confirmed and returns its path.
func (s *site) readyDashboard(name string) string {
	s.t.Helper()
	path := s.createWork("finance", name)
	s.confirmFile(path, "finance-sample.xlsx", s.financeSample())
	return path
}

var deploymentLink = regexp.MustCompile(`https://cave\.example\.invalid(/d/[A-Za-z0-9_-]+/)`)

// deploy confirms the Deployment of the Dashboard at path, named dash-<its id>, and returns its link's path, /d/<name>/.
func (s *site) deploy(path string) string {
	s.t.Helper()
	return s.deployAs(path, "dash-"+path[strings.LastIndex(path, "/")+1:])
}

// deployAs deploys the Dashboard at path under name, ticking that they have the right to put its data online, and returns its link's path.
func (s *site) deployAs(path, name string) string {
	s.t.Helper()
	res, body := s.post(path+"/deploy", url.Values{"confirm": {"yes"}, "name": {name}, "right": {"yes"}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	_, body = s.get(path + "/deploy")
	m := deploymentLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("the Deploy tab shows no link after deploying: %s", body)
	}
	return m[1]
}

// ageAllDeployments moves every Deployment's deploy time into the past.
func (s *site) ageAllDeployments(days int) {
	s.t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE deployments SET deployed_at = deployed_at - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

// A PUBLIC_BASE_URL with a path names only Cave's origin in the CSP, since a source with a path matches that exact path.
func TestADeploymentsCSPNamesOnlyCavesOrigin(t *testing.T) {
	_, ada := labLearner(t, func(c *config.Config) { c.PublicBaseURL = "https://cave.example.invalid:8443/lms/" })
	path := ada.readyDashboard("Kewangan")
	res, body := ada.post(path+"/deploy", url.Values{"confirm": {"yes"}, "name": {"kedai-ada"}, "right": {"yes"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, _ = ada.second().get("/d/kedai-ada/")
	const wantCSP = "sandbox allow-scripts; default-src 'none'; script-src 'self' https://cave.example.invalid:8443 'unsafe-inline'; " +
		"style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'self' https://cave.example.invalid:8443; frame-ancestors 'none'"
	if csp := res.Header.Get("Content-Security-Policy"); csp != wantCSP {
		t.Fatalf("Dashboard page CSP = %q", csp)
	}
}

func TestLearnerDeploysAfterTheWarningAndAVisitorSeesTheDashboard(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")

	res, body := ada.get(path + "/deploy")
	for _, want := range []string{"Anyone with the link can see it", "Online while your Plan is active", "Once only", `href="` + path + `/deploy?confirm"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body = ada.get(path + "/deploy?confirm")
	expectStatus(t, res, body, http.StatusOK, `<input type="hidden" name="confirm" value="yes">`)
	expectStatus(t, res, body, http.StatusOK, "You can deploy it only once")
	for _, want := range []string{`name="name"`, `pattern="[a-z0-9]+(-[a-z0-9]+)*"`, `type="checkbox" name="right" value="yes" required`,
		"I have the right to put this data online", "Anyone with the link can see this Dashboard and its data", "https://cave.example.invalid/d/"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}

	// Without the confirmation nothing is deployed.
	res, _ = ada.post(path+"/deploy", nil)
	if loc := res.Header.Get("Location"); loc != path+"/deploy?confirm" {
		t.Fatalf("deploying without confirming went to %q, want the confirm step", loc)
	}
	if _, body = ada.get(path + "/deploy"); deploymentLink.MatchString(body) {
		t.Fatalf("deployed without confirming: %s", body)
	}

	link := ada.deployAs(path, "kedai-ada-2026")
	if link != "/d/kedai-ada-2026/" {
		t.Fatalf("the link is %s, want the name Ada picked", link)
	}
	res, body = ada.get(path + "/deploy")
	for _, want := range []string{"Online", "when your Plan ends, it sleeps", "readonly", `href="` + link + `"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}

	visitor := ada.second() // no session
	res, page := visitor.get(link)
	expectStatus(t, res, page, http.StatusOK, `fetch("data/dataset-`)
	// Sandboxed to an opaque origin, where some browsers match nothing to 'self', so Cave's address is named too.
	const wantCSP = "sandbox allow-scripts; default-src 'none'; script-src 'self' https://cave.example.invalid 'unsafe-inline'; " +
		"style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'self' https://cave.example.invalid; frame-ancestors 'none'"
	if csp := res.Header.Get("Content-Security-Policy"); csp != wantCSP {
		t.Fatalf("Dashboard page CSP = %q", csp)
	}
	res, data := visitor.get(link + visitor.dataOf(link))
	expectStatus(t, res, data, http.StatusOK, `"Tarikh"`)
	if acao := res.Header.Get("Access-Control-Allow-Origin"); acao != "*" {
		t.Fatalf("the data's Access-Control-Allow-Origin = %q", acao)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=300" {
		t.Fatalf("the data's Cache-Control = %q, want public for 5 minutes, so sleep and Takedown take effect", cc)
	}
	res, script := visitor.get(link + "charts.js")
	expectStatus(t, res, script, http.StatusOK, "function drawDashboard")

	if res, _ := visitor.get(strings.TrimSuffix(link, "/")); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != link {
		t.Fatalf("the link without its slash: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestADeploymentIsAwakeWhileThePlanIsActiveSleepsWhenItEndsAndWakesOnRenewal(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	other := ada.readyDashboard("Stok")
	visitor := ada.second()
	data := visitor.dataOf(link)

	ada.ageAllDeployments(20) // well past the old 7 days, inside the 30-day period
	if res, page := visitor.get(link); res.StatusCode != http.StatusOK || !strings.Contains(page, `fetch("data/`) {
		t.Fatalf("20 days in, with the Plan active, the Deployment is not awake: %d", res.StatusCode)
	}

	admin.ageAllPeriods(31) // the only period ends, none follows
	for _, p := range []string{link, link + data} {
		res, body := visitor.get(p)
		expectStatus(t, res, body, http.StatusOK, "This Dashboard is asleep")
		if strings.Contains(body, "Tarikh") {
			t.Fatalf("an asleep link still shows the data: %s", body)
		}
	}
	res, body := ada.get(path + "/deploy") // the Lab is closed to a lapsed Plan holder, their Deploy tabs too
	expectRedirect(t, res, body, "/pricing")
	res, body = ada.get(other + "/deploy")
	expectRedirect(t, res, body, "/pricing")
	res, body = ada.post(other+"/deploy", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusForbidden, "need a Plan")

	admin.assignPlan("ada@example.com", admin.onlyPlanID()) // the next period starts now
	res, body = visitor.get(link + data)
	expectStatus(t, res, body, http.StatusOK, `"Tarikh"`)
	res, body = ada.get(path + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "Online")
	expectStatus(t, res, body, http.StatusOK, `href="`+link+`"`)

	// It is still one Deployment per Dashboard: deploying again changes nothing.
	ada.post(path+"/deploy", url.Values{"confirm": {"yes"}})
	if _, again := ada.get(path + "/deploy"); !strings.Contains(again, `href="`+link+`"`) {
		t.Fatalf("deploying again changed the link: %s", again)
	}
}

func TestReuploadingTheDatasetChangesWhatTheDeploymentShows(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, []any{day, "Kopi", "Dapur", "Keluar", 4321.5})
	res, body := ada.postFile(path+"/dataset", "new.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, data := ada.second().get(link + ada.second().dataOf(link))
	if !strings.Contains(data, "4321.5") || strings.Count(data, "Tarikh") != 1 {
		t.Fatalf("the Deployment does not show the new Dataset: %s", data)
	}
}

func TestDeployingBeyondTheAllowanceIsRefusedAndDeletingFreesASlot(t *testing.T) {
	_, ada := labLearner(t) // the Plan allows 3 Deployments awake at once
	first := ada.readyDashboard("One")
	res, body := ada.get(first + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "It uses 1 of your 3 free Deployments.")
	firstLink := ada.deploy(first)
	ada.deploy(ada.readyDashboard("Two"))
	ada.deploy(ada.readyDashboard("Three"))
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "3 of 3")

	fourth := ada.readyDashboard("Four")
	res, body = ada.get(fourth + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "Awake now: 3 of the 3 your Plan allows.")
	res, body = ada.post(fourth+"/deploy", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Awake now: 3 of the 3 your Plan allows.")
	if deploymentLink.MatchString(body) {
		t.Fatalf("deployed beyond the allowance: %s", body)
	}

	res, body = ada.post(first+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if res, _ := ada.get(first); res.StatusCode != http.StatusNotFound {
		t.Fatalf("the deleted Dashboard still opens: %d", res.StatusCode)
	}
	res, body = ada.second().get(firstLink)
	expectStatus(t, res, body, http.StatusNotFound, "Not found")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "2 of 3")
	ada.deploy(fourth)
}

func TestRenewingOntoASmallerAllowanceWakesEveryDeploymentButRefusesNewOnes(t *testing.T) {
	admin, ada := labLearner(t) // 3 awake at once
	var links, paths []string
	for _, name := range []string{"One", "Two", "Three"} {
		path := ada.readyDashboard(name)
		paths, links = append(paths, path), append(links, ada.deploy(path))
	}
	plan := admin.onlyPlanID()
	res, body := admin.post("/admin/plans/"+plan, url.Values{"name": {"Lab Monthly"}, "days": {"30"}, "deployments": {"1"}, "storage": {"1024"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.ageAllPeriods(31)
	admin.assignPlan("ada@example.com", plan) // renews with 1 awake at once

	for _, link := range links {
		res, data := ada.second().get(link + ada.second().dataOf(link))
		expectStatus(t, res, data, http.StatusOK, `"Tarikh"`)
	}
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "3 of 1")
	fourth := ada.readyDashboard("Four")
	res, body = ada.get(fourth + "/deploy?confirm")
	if strings.Contains(body, "-2") {
		t.Fatalf("the Deploy tab shows a negative number of free Deployments: %s", body)
	}
	res, body = ada.post(fourth+"/deploy", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Awake now: 3 of the 1 your Plan allows.")

	ada.post(paths[0]+"/delete", confirming)
	ada.post(paths[1]+"/delete", confirming)
	res, body = ada.post(fourth+"/deploy", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Awake now: 1 of the 1 your Plan allows.")
	ada.post(paths[2]+"/delete", confirming)
	ada.deploy(fourth)
}

func TestLearnerRenamesAndDeletesADashboard(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")

	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/rename"`)
	expectStatus(t, res, body, http.StatusOK, `href="`+path+`/delete"`)
	res, body = ada.get(path + "/rename")
	expectStatus(t, res, body, http.StatusOK, `value="Kewangan"`)
	res, body = ada.post(path+"/rename", url.Values{"name": {" "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give the Dashboard a name")
	res, body = ada.post(path+"/rename", url.Values{"name": {"Kedai 2026"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `<a href="`+path+`">Kedai 2026</a>`)

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	admin.assignPlan("bob@example.com", admin.onlyPlanID()) // without a Plan the Lab sends him to /pricing first
	for _, p := range []string{"/rename", "/delete"} {
		if res, _ := bob.post(path+p, url.Values{"name": {"Mine"}}); res.StatusCode != http.StatusNotFound {
			t.Errorf("Bob got %d for POST %s, want 404", res.StatusCode, p)
		}
	}

	if res, _ := bob.get(path + "/delete"); res.StatusCode != http.StatusNotFound {
		t.Errorf("Bob got %d for GET /delete, want 404", res.StatusCode)
	}

	res, body = ada.post(path+"/delete", nil)
	expectRedirect(t, res, body, path+"/delete")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Kedai 2026")
	res, body = ada.get(path + "/delete")
	expectStatus(t, res, body, http.StatusOK, "Delete “Kedai 2026”? Its Dataset goes too.")
	expectAll(t, body, "This cannot be undone.", `href="/lab"`)
	res, body = ada.post(path+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "No Dashboards yet.")
}

func TestAnAdminUsesTheWholeLabWithoutAPlan(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := admin.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="/lab/new"`)
	admin.createPlan("Lab Monthly", "30", "3")
	admin.assignPlan("admin@example.com", admin.onlyPlanID()) // a Plan of their own does not limit them
	res, body = admin.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="/lab/new"`)

	res, body = admin.get(admin.readyDashboard("Kosong") + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "A Dashboard can be deployed one time.")
	if strings.Contains(body, "free Deployments") {
		t.Fatalf("the Admin is shown a Plan allowance: %s", body)
	}
	_, confirm := admin.get(admin.readyDashboard("Sahkan") + "/deploy?confirm")
	for _, page := range []string{body, confirm} {
		if strings.Contains(page, "while your Plan is active") {
			t.Fatalf("the Admin is told their Deployment depends on a Plan: %s", page)
		}
	}
	expectStatus(t, res, confirm, http.StatusOK, "Anyone with the link can see this Dashboard and its data")

	var links []string
	for _, name := range []string{"Satu", "Dua", "Tiga", "Empat"} { // more than the Plan allows
		links = append(links, admin.deploy(admin.readyDashboard(name)))
	}
	admin.ageAllPeriods(31) // the Admin's own Plan ends; their Deployments never sleep
	for _, link := range links {
		res, body := admin.second().get(link)
		expectStatus(t, res, body, http.StatusOK, `fetch("data/`)
	}
	res, body = admin.get("/admin/deployments")
	for _, want := range []string{"admin@example.com", "Empat", `href="` + links[0] + `"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

func TestADeploymentNameIsRefusedWithTheReason(t *testing.T) {
	admin, ada := labLearner(t)
	taken := ada.readyDashboard("Lama")
	ada.deployAs(taken, "kedai-lama")
	ada.post(taken+"/delete", confirming) // the name stays taken after its Dashboard is deleted
	path := ada.readyDashboard("Kewangan")
	for _, c := range []struct{ name, want string }{
		{"ab", "3 to 40 characters"},
		{strings.Repeat("a", 41), "3 to 40 characters"},
		{"Kedai", "lowercase letters, numbers and single dashes"},
		{"kedai ada", "lowercase letters, numbers and single dashes"},
		{"kedai--ada", "lowercase letters, numbers and single dashes"},
		{"-kedai", "lowercase letters, numbers and single dashes"},
		{"kedai-", "lowercase letters, numbers and single dashes"},
		{"kedai_ada", "lowercase letters, numbers and single dashes"},
		{"admin", "is reserved"},
		{"login", "is reserved"},
		{"courses", "is reserved"},
		{"cavelms", "is reserved"},
		{"kedai-lama", "is already taken"},
	} {
		res, body := ada.post(path+"/deploy", url.Values{"confirm": {"yes"}, "name": {c.name}, "right": {"yes"}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, `name="name"`) // back on the confirm step
	}
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	admin.assignPlan("bob@example.com", admin.onlyPlanID())
	res, body := bob.post(bob.readyDashboard("Bob")+"/deploy", url.Values{"confirm": {"yes"}, "name": {"kedai-lama"}, "right": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "is already taken")

	res, body = ada.post(path+"/deploy", url.Values{"confirm": {"yes"}, "name": {"kedai-baru"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tick that you have the right to put this data online")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `value="kedai-baru"`)
	if _, body = ada.get(path + "/deploy"); deploymentLink.MatchString(body) {
		t.Fatalf("deployed after a refusal: %s", body)
	}
	if link := ada.deployAs(path, "kedai-baru"); link != "/d/kedai-baru/" {
		t.Fatalf("link %s", link)
	}
}

func TestADeploymentMadeBeforeNamesKeepsItsOldLink(t *testing.T) {
	_, ada := labLearner(t)
	ada.deploy(ada.readyDashboard("Kewangan"))
	old := "AbC_xyz-0123456789abcd" // a random slug from before names, kept as the name
	if _, err := ada.db.ExecContext(context.Background(), `UPDATE deployments SET name = $1`, old); err != nil {
		t.Fatal(err)
	}
	visitor := ada.second()
	res, body := visitor.get("/d/" + old + "/")
	expectStatus(t, res, body, http.StatusOK, `fetch("data/`)
	res, body = visitor.get("/d/" + old + "/" + visitor.dataOf("/d/"+old+"/"))
	expectStatus(t, res, body, http.StatusOK, `"Tarikh"`)
}

// A Dataset never changes under its key, so a Deployment reads it once, not on every visit:
// with the stored Dataset gone, its page and data still answer the same.
func TestADeploymentReadsItsDatasetOnceNotOnEveryVisit(t *testing.T) {
	_, ada := labLearner(t)
	link := ada.deploy(ada.readyDashboard("Kewangan"))
	visitor := ada.second()
	dataURL := link + visitor.dataOf(link)
	_, page := visitor.get(link)
	_, data := visitor.get(dataURL)

	key := dataURL[strings.LastIndex(dataURL, "/")+1:]
	if err := os.Remove(filepath.Join(ada.storageDir, key)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		res, again := visitor.get(link)
		expectStatus(t, res, again, http.StatusOK, "")
		if again != page {
			t.Fatalf("visit %d: the page changed:\n%s\nwant\n%s", i, again, page)
		}
		res, again = visitor.get(dataURL)
		expectStatus(t, res, again, http.StatusOK, "")
		if again != data {
			t.Fatalf("visit %d: the data changed", i)
		}
	}
}
