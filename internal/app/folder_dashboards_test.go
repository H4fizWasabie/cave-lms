package app_test

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var (
	folderRestoreForm = regexp.MustCompile(`<form[^>]*action="(/lab/dashboards/\d+/pages/\d+/restore)"[^>]*>\s*<input type="hidden" name="next" value="([^"]*)"`)
	folderAddress     = "/my-folder/ada-example-com"
)

// adaWithDeployedAndUndeployed has a deployed page Dashboard with one earlier page, and one never deployed.
func adaWithDeployedAndUndeployed(t *testing.T) (admin, ada *site, link string) {
	t.Helper()
	admin, ada = labLearner(t)
	deployed := ada.createPage("Kewangan", versionOf(1))
	link = ada.deployAs(deployed, "kewangan-ada")
	ada.uploadPage(deployed, versionOf(2))
	ada.createPage("Draf", versionOf(1))
	return admin, ada, link
}

// cardOf is the part of the Folder's body for the Dashboard card called name, up to the next card.
func cardOf(t *testing.T, body, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<h3 class="folder-dash-name">(?:<a [^>]*>)?` + name + `(?:</a>)?</h3>(.*?)</article>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no card for %q: %s", name, body)
	}
	return m[1]
}

func TestADashboardCardOpensAnAwakeDeploymentLikeTheDeployTabAndSaysNotDeployedOtherwise(t *testing.T) {
	t.Parallel()
	_, ada, link := adaWithDeployedAndUndeployed(t)
	_, body := ada.getFolder()
	expectAll(t, cardOf(t, body, "Kewangan"), `<a class="folder-live" href="`+link+`" target="_blank" rel="noopener"`, ">Open<")
	draft := cardOf(t, body, "Draf")
	expectAll(t, draft, "Not deployed")
	if strings.Contains(draft, "folder-live") {
		t.Errorf("an undeployed Dashboard has an Open link: %s", draft)
	}
	if strings.Contains(body, ">Live<") {
		t.Errorf("the Folder uses Live, which the glossary keeps for a Live Course: %s", body)
	}
	_, body = ada.getFolderAsking("ms")
	expectAll(t, cardOf(t, body, "Kewangan"), `href="`+link+`"`, ">Buka<", "Halaman semasa", "Halaman terdahulu", "Pulihkan")
	expectAll(t, cardOf(t, body, "Draf"), "Belum di-deploy")
}

func TestADashboardCardSaysTakenDownNotNotDeployedWhenCaveTookItsDeploymentOffline(t *testing.T) {
	t.Parallel()
	admin, ada, link := adaWithDeployedAndUndeployed(t)
	_, body := admin.get("/admin/deployments")
	admin.post(takedownAction.FindStringSubmatch(body)[1], url.Values{"reason": {"Aduan"}})

	_, body = ada.getFolder()
	card := cardOf(t, body, "Kewangan")
	expectAll(t, card, "Taken down")
	for _, not := range []string{"folder-live", link, "Not deployed"} {
		if strings.Contains(card, not) {
			t.Errorf("a taken down Deployment's card shows %q: %s", not, card)
		}
	}
	expectAll(t, cardOf(t, body, "Draf"), "Not deployed")
	_, body = ada.getFolderAsking("ms")
	expectAll(t, cardOf(t, body, "Kewangan"), "Diturunkan")
}

func TestTheCardShowsTheCurrentPageAndGivesEachEarlierPageARestoreButtonThatReturnsToTheFolderWithARestoredMessage(t *testing.T) {
	t.Parallel()
	_, ada, _ := adaWithDeployedAndUndeployed(t)
	_, body := ada.getFolder()
	card := cardOf(t, body, "Kewangan")
	expectAll(t, card, "Current page", "Earlier page")
	forms := folderRestoreForm.FindAllStringSubmatch(card, -1)
	if len(forms) != 1 {
		t.Fatalf("%d Restore forms on the card, want 1: %s", len(forms), card)
	}
	if strings.Contains(body, "Restored") {
		t.Errorf("the Folder says Restored before any restore: %s", body)
	}
	res, got := ada.post(forms[0][1], url.Values{"next": {strings.ReplaceAll(forms[0][2], "&amp;", "&")}})
	expectRedirect(t, res, got, folderAddress+"?restored#dashboard-pages")

	res, body = ada.get(folderAddress + "?restored")
	expectStatus(t, res, body, 200, `role="status">Restored<`)
	card = cardOf(t, body, "Kewangan")
	if n := len(folderRestoreForm.FindAllString(card, -1)); n != 1 { // the page that was current is earlier now
		t.Errorf("%d Restore forms after restoring, want 1: %s", n, card)
	}
	_, body = ada.getFolderAsking("ms")
	if strings.Contains(body, "Dipulihkan") {
		t.Errorf("the Folder says Restored without ?restored: %s", body)
	}
	_, body = ada.getAsking(folderAddress+"?restored", "ms")
	expectAll(t, body, ">Dipulihkan<")
}

func TestRestoreSendsTheLearnerToAnyLocalNextAndIgnoresOneOffSiteOrMissing(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	for _, c := range []struct{ next, want string }{
		{"/my-folder", "/my-folder"},
		{"", path},
		{"https://evil.example/", path},
		{"//evil.example/", path},
		{"/\\evil.example", path},
		{"javascript:alert(1)", path},
	} {
		ada.uploadPage(path, versionOf(2))
		restore := ada.earlierPages(path)[0]
		form := url.Values{}
		if c.next != "" {
			form.Set("next", c.next)
		}
		res, body := ada.post(restore, form)
		expectRedirect(t, res, body, c.want)
	}
}

func TestACardListsTheWorkbooksItsDashboardUsedAsLinks(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	dashboard := ada.createPage("Kewangan", myPage)
	ada.createPage("Draf", myPage)
	ada.postFile("/workbooks", "rak.xlsx", shopReport(t, "Gula", 80))
	ada.postFile("/workbooks", "lain.xlsx", shopReport(t, "Beras", 120))
	if _, err := ada.db.Exec(`INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date, workbook_id)
		SELECT $1, 'k1', 'rak.xlsx', 1, 1, 'current', 1, '2026-10-01', id FROM workbooks WHERE name = 'rak.xlsx'`,
		strings.TrimPrefix(dashboard, "/lab/dashboards/")); err != nil {
		t.Fatal(err)
	}
	_, body := ada.getFolder()
	used := cardOf(t, body, "Kewangan")
	expectAll(t, used, "Workbooks used", `>rak.xlsx</a>`)
	if strings.Contains(used, "lain.xlsx") {
		t.Errorf("the card lists a Workbook its Dashboard did not use: %s", used)
	}
	expectAll(t, cardOf(t, body, "Draf"), "none")
}

func TestTheAdminReadsTheCardsOfALearnersFolderWithTheLiveLinkButNoRestore(t *testing.T) {
	t.Parallel()
	admin, _, link := adaWithDeployedAndUndeployed(t)
	_, body := admin.get(folderAddress)
	card := cardOf(t, body, "Kewangan")
	expectAll(t, card, `href="`+link+`"`, "Earlier page")
	for _, not := range []string{"Restore", "/restore"} {
		if strings.Contains(body, not) {
			t.Errorf("the Admin's read-only Folder shows %q: %s", not, body)
		}
	}
}

func TestTheAdminSeesTheWorkbooksUsedAsPlainNamesAndTheLearnerSeesLinks(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	dashboard := ada.createPage("Kewangan", myPage)
	ada.postFile("/workbooks", "rak.xlsx", shopReport(t, "Gula", 80))
	if _, err := ada.db.Exec(`INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date, workbook_id)
		SELECT $1, 'k1', 'rak.xlsx', 1, 1, 'current', 1, '2026-10-01', id FROM workbooks WHERE name = 'rak.xlsx'`,
		strings.TrimPrefix(dashboard, "/lab/dashboards/")); err != nil {
		t.Fatal(err)
	}
	_, body := ada.getFolder()
	expectAll(t, cardOf(t, body, "Kewangan"), `<a href="/workbooks/`, `>rak.xlsx</a>`)

	_, body = admin.get(folderAddress)
	card := cardOf(t, body, "Kewangan")
	expectAll(t, card, "Workbooks used", "rak.xlsx")
	if strings.Contains(card, `href="/workbooks/`) {
		t.Errorf("the Admin's read-only card links a Workbook, as the table's Used on does not: %s", card)
	}
}
