package app_test

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// storedFiles lists what the Disk store holds for CaveHost.
func storedFiles(s *site) []string {
	files, _ := filepath.Glob(filepath.Join(s.storageDir, "cavehost-*"))
	return files
}

func TestALearnerDeletesACaveHostDeploymentAfterConfirmingAndItsNameIsNeverGivenAgain(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")
	onlineSales(t, ada, "jualan-bob")
	if files := storedFiles(ada); len(files) != 4 {
		t.Fatalf("the store holds %v before the delete, want two files for each of two Deployments", files)
	}
	_, body := ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, `href="/cavehost/jualan-ada/delete"`) {
		t.Fatalf("the Deployment's page offers no delete: %s", body)
	}

	// The post only asks first, and deletes nothing.
	res, body := ada.post("/cavehost/jualan-ada/delete", url.Values{})
	expectRedirect(t, res, body, "/cavehost/jualan-ada/delete")
	res, body = ada.get("/cavehost/jualan-ada/delete")
	expectStatus(t, res, body, http.StatusOK, "Delete jualan-ada")
	expectStatus(t, res, body, http.StatusOK, `action="/cavehost/jualan-ada/delete"`)
	if res, _ := ada.second().get(link); res.StatusCode != http.StatusOK {
		t.Fatalf("the link answers %d before the delete is confirmed, want 200", res.StatusCode)
	}

	// Another Learner cannot delete it.
	other := ada.second()
	other.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	if res, _ := other.get("/cavehost/jualan-ada/delete"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner opening the delete got %d, want 404", res.StatusCode)
	}

	res, body = ada.post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}})
	expectRedirect(t, res, body, "/cavehost")
	res, body = ada.second().get(link)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusNotFound, "")
	if _, body = ada.get("/cavehost"); strings.Contains(body, "jualan-ada") || !strings.Contains(body, "jualan-bob") {
		t.Errorf("the CaveHost page after the delete lists the wrong Deployments: %s", body)
	}
	if files := storedFiles(ada); len(files) != 2 {
		t.Errorf("the store holds %v after the delete, want only the other Deployment's two files", files)
	}
	// Its name is never given to another Deployment, not by CaveHost and not by the Lab.
	res, body = ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "already taken")
}

func TestALearnerWithoutAnActiveCaveHostPlanCanStillDeleteTheirSleepingDeployment(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin := newSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	admin.assignCaveHost("ada@example.com", 30)
	link := onlineSales(t, ada, "jualan-ada")
	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	admin.ageAllCaveHostPeriods(31) // asleep, within its 90 days

	res, body := ada.get("/cavehost/jualan-ada/delete")
	expectStatus(t, res, body, http.StatusOK, "Delete jualan-ada")
	// Another Learner, who has no CaveHost either, still finds nothing to delete.
	if res, _ := bob.post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}}); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner deleting got %d, want 404", res.StatusCode)
	}
	res, body = ada.post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}})
	expectRedirect(t, res, body, "/cavehost")
	res, body = ada.second().get(link)
	expectStatus(t, res, body, http.StatusNotFound, "")
	if files := storedFiles(ada); len(files) != 0 {
		t.Errorf("the store holds %v after the delete, want nothing", files)
	}
}

func TestOnlyTheOwnerOfACaveHostDeploymentCanDeleteIt(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin := newSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	admin.assignCaveHost("ada@example.com", 30)
	link := onlineSales(t, ada, "jualan-ada")

	// The Admin has CaveHost, but the Deployment is not theirs; a Visitor is sent to log in.
	res, body := admin.get("/cavehost/jualan-ada/delete")
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = admin.post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusNotFound, "")
	if res, _ := ada.second().post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a Visitor deleting got %d, want a redirect to log in", res.StatusCode)
	}
	if res, _ := ada.second().get(link); res.StatusCode != http.StatusOK {
		t.Fatalf("the link answers %d after the others' deletes, want 200", res.StatusCode)
	}
}

// takedownOf is the address the Admin's Deployments list posts Takedown of the Deployment called name to.
func takedownOf(t *testing.T, list, name string) string {
	t.Helper()
	m := regexp.MustCompile(`action="(/admin/deployments/\d+/takedown)">\s*<label[^>]*>Reason for taking down ` + name + `<`).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("the list has no Takedown form for %s: %s", name, list)
	}
	return m[1]
}

func TestTheAdminSeesCaveHostDeploymentsBesideLabOnesAndTakesOneDownWithAReason(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin, ada := labLearner(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.assignCaveHost("ada@example.com", 30)
	labLink := ada.deployAs(ada.readyDashboard("Kewangan"), "kewangan-ada")
	link := onlineSales(t, ada, "jualan-ada")
	visitor := ada.second()

	res, list := admin.get("/admin/deployments")
	for _, want := range []string{"<code>kewangan-ada</code>", "<code>jualan-ada</code>", `href="` + labLink + `"`, `href="` + link + `"`, "Ada", "ada@example.com", "CaveHost", "Awake"} {
		expectStatus(t, res, list, http.StatusOK, want)
	}
	action := takedownOf(t, list, "jualan-ada")
	res, body := ada.post(action, url.Values{"reason": {"mine"}})
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin")

	res, body = admin.post(action, url.Values{"reason": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give a reason")
	if res, body := visitor.get(link); res.StatusCode != http.StatusOK || strings.Contains(body, "offline early") {
		t.Fatalf("a Takedown without a reason took the Deployment down: %s", body)
	}

	res, body = admin.post(action, url.Values{"reason": {"Shows a real company's IC numbers."}})
	expectRedirect(t, res, body, "/admin/deployments")
	for _, p := range []string{link, link + "data.json"} {
		res, page := visitor.get(p)
		expectStatus(t, res, page, http.StatusOK, "Cave took it offline early")
		if strings.Contains(page, `"Item"`) || strings.Contains(page, "Jualan") {
			t.Fatalf("a taken-down link still shows its content: %s", page)
		}
	}
	if res, page := visitor.get(labLink); res.StatusCode != http.StatusOK || strings.Contains(page, "offline early") {
		t.Fatalf("the Lab Deployment went down with it: %s", page)
	}
	res, list = admin.get("/admin/deployments")
	expectStatus(t, res, list, http.StatusOK, "Taken down")
	expectStatus(t, res, list, http.StatusOK, "Shows a real company&#39;s IC numbers.")
	if strings.Contains(list, "Reason for taking down jualan-ada") {
		t.Fatalf("a taken-down Deployment still offers Takedown: %s", list)
	}

	// The Learner sees why, and no Sync reads it.
	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "Cave took this Deployment offline on")
	expectStatus(t, res, body, http.StatusOK, "Shows a real company&#39;s IC numbers.")
	asked := len(google.requests())
	ada.syncDue()
	if got := google.requests()[asked:]; len(got) != 0 {
		t.Errorf("the hourly Sync asked Google for %q for a taken-down Deployment, want nothing", got)
	}

	// A renewal does not wake it.
	admin.ageAllCaveHostPeriods(31)
	admin.assignCaveHost("ada@example.com", 30)
	res, page := visitor.get(link + "data.json")
	expectStatus(t, res, page, http.StatusOK, "Cave took it offline early")
}

func TestTheAdminsDeploymentsListShowsASleepingCaveHostDeploymentAsleep(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin, ada := labLearner(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.assignCaveHost("ada@example.com", 30)
	onlineSales(t, ada, "jualan-ada")
	admin.ageAllCaveHostPeriods(31)

	res, list := admin.get("/admin/deployments")
	expectStatus(t, res, list, http.StatusOK, "Asleep: the Learner has no active CaveHost Plan")
	res, list = admin.getAsking("/admin/deployments", "ms")
	expectStatus(t, res, list, http.StatusOK, "Tidur: Pelajar tiada Pelan CaveHost aktif")
}

func TestTheDeleteOfACaveHostDeploymentReadsInMalay(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	onlineSales(t, ada, "jualan-ada")
	res, body := ada.getAsking("/cavehost/jualan-ada", "ms")
	expectStatus(t, res, body, http.StatusOK, "Padam Deployment")
	res, body = ada.getAsking("/cavehost/jualan-ada/delete", "ms")
	expectStatus(t, res, body, http.StatusOK, "Padam jualan-ada? Datanya dan setiap halaman")
}

// deleteLapsed runs the step the ticker runs: delete the Deployments of Learners whose CaveHost Plan ended 90 days ago.
func (s *site) deleteLapsed() { s.application.DeleteLapsedCaveHost(context.Background()) }

func TestDeploymentsSleepWhenTheLastPeriodEndsWakeAtTheSameLinksAndAreDeletedAfter90DaysUnlessANewPeriodStarted(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin := newSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada, bob := admin.second(), admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	for _, email := range []string{"ada@example.com", "bob@example.com"} {
		admin.assignCaveHost(email, 30)
	}
	adaLink, bobLink := onlineSales(t, ada, "jualan-ada"), onlineSales(t, bob, "jualan-bob")
	visitor := ada.second()

	// 89 days after the end: asleep, and nothing is deleted.
	admin.ageAllCaveHostPeriods(30 + 89)
	ada.deleteLapsed()
	for _, link := range []string{adaLink, adaLink + "data.json", bobLink} {
		res, body := visitor.get(link)
		expectStatus(t, res, body, http.StatusOK, "This CaveHost page is asleep")
	}
	if files := storedFiles(ada); len(files) != 4 {
		t.Fatalf("the store holds %v 89 days after the end, want all four files", files)
	}

	// A new period wakes Ada's Deployment at the same link, with its data and page, and keeps it past the 90 days.
	admin.assignCaveHost("ada@example.com", 30)
	admin.ageAllCaveHostPeriods(2) // Bob's period ended 91 days ago; Ada's new one began 2 days ago
	ada.deleteLapsed()
	res, body := visitor.get(adaLink)
	expectStatus(t, res, body, http.StatusOK, "<title>Jualan</title>")
	res, body = visitor.get(adaLink + "data.json")
	expectStatus(t, res, body, http.StatusOK, `"Item"`)
	res, body = visitor.get(bobLink)
	expectStatus(t, res, body, http.StatusNotFound, "")
	if files := storedFiles(ada); len(files) != 2 {
		t.Fatalf("the store holds %v after Bob's Deployment was deleted, want Ada's two files", files)
	}

	// Bob's name is never given to another Deployment, even once the Deployment is gone.
	admin.assignCaveHost("bob@example.com", 30)
	res, body = bob.linkSheet(sheetLink, "jualan-bob")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "already taken")
}

// adminRowOf is the Admin's Deployments list row of the Deployment called name.
func adminRowOf(t *testing.T, list, name string) string {
	t.Helper()
	for _, row := range strings.Split(list, "</tr>") {
		if strings.Contains(row, "<code>"+name+"</code>") {
			return row
		}
	}
	t.Fatalf("the list has no row for %s: %s", name, list)
	return ""
}

func TestTheAdminsListSaysWhetherTheLearnerOrThe90DaysDeletedACaveHostDeployment(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin := newSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada, bob := admin.second(), admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	for _, email := range []string{"ada@example.com", "bob@example.com"} {
		admin.assignCaveHost(email, 30)
	}
	onlineSales(t, ada, "jualan-ada")
	onlineSales(t, bob, "jualan-bob")
	res, body := ada.post("/cavehost/jualan-ada/delete", url.Values{"confirm": {"yes"}})
	expectRedirect(t, res, body, "/cavehost")
	admin.ageAllCaveHostPeriods(30 + 91)
	admin.application.RunDailyPrune(context.Background())

	_, list := admin.get("/admin/deployments")
	if row := adminRowOf(t, list, "jualan-ada"); !strings.Contains(row, "Deleted by the Learner") {
		t.Errorf("a Deployment its Learner deleted does not say so: %s", row)
	}
	row := adminRowOf(t, list, "jualan-bob")
	if strings.Contains(row, "Deleted by the Learner") || !strings.Contains(row, "Deleted: the CaveHost Plan ended over 90 days ago") {
		t.Errorf("a Deployment the 90 days deleted does not say so: %s", row)
	}
	_, list = admin.getAsking("/admin/deployments", "ms")
	if row := adminRowOf(t, list, "jualan-bob"); !strings.Contains(row, "Dipadam: Pelan CaveHost telah tamat lebih 90 hari lalu") {
		t.Errorf("the Malay list does not say the 90 days deleted it: %s", row)
	}
}
