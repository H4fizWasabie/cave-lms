package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var workbookDeleteLink = regexp.MustCompile(`href="(/workbooks/\d+/delete)"`)

// deletePath is the delete link of the Learner's only Workbook, read from their Folder.
func (s *site) deletePath() string {
	s.t.Helper()
	res, body := s.getFolder()
	expectStatus(s.t, res, body, http.StatusOK, "/workbooks/")
	m := workbookDeleteLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("the Folder offers no way to delete a Workbook: %s", body)
	}
	return m[1]
}

func TestTheDeleteConfirmationNamesHowManyDashboardsUsedTheWorkbook(t *testing.T) {
	_, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	ada.readyDashboard("Stok") // the same file, so the same Workbook
	res, body := ada.get(ada.deletePath())
	expectStatus(t, res, body, http.StatusOK, "Dashboards that used it: 2.")

	ada.postFile(ada.createWork("Baru")+"/dataset", "rosak.xlsx", []byte("not a workbook")) // kept, never cleaned
	_, body = ada.getFolder()
	var counts []string
	for _, m := range workbookDeleteLink.FindAllStringSubmatch(body, -1) {
		_, page := ada.get(m[1])
		counts = append(counts, regexp.MustCompile(`Dashboards that used it: \d+`).FindString(page)+" "+regexp.MustCompile(`[a-z-]+\.xlsx`).FindString(page))
	}
	if got := strings.Join(counts, "|"); got != "Dashboards that used it: 0 rosak.xlsx|Dashboards that used it: 2 finance-sample.xlsx" {
		t.Fatalf("confirmations name %q", got)
	}
}

func TestDeletingAWorkbookRemovesItAndFreesItsRoomButNotWhatWasCleanedFromIt(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	visitor := ada.second()
	before := visitor.get2(link + visitor.dataOf(link))
	download := ada.workbookPath()
	del := ada.deletePath()
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want the Dataset and the Workbook", n)
	}
	res, body := ada.get("/lab")
	full := ada.storedKB()
	expectStatus(t, res, body, http.StatusOK, full)

	res, body = ada.post(del, nil) // not from the confirmation page: nothing happens yet
	expectRedirect(t, res, body, del)
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files after an unconfirmed post", n)
	}
	res, body = ada.post(del, url.Values{"confirm": {"yes"}})
	expectRedirect(t, res, body, "/my-folder")

	res, body = ada.getFolder()
	if strings.Contains(body, "finance-sample.xlsx") || strings.Contains(body, "/workbooks/") {
		t.Fatalf("the Folder still lists the Workbook: %s", body)
	}
	res, body = ada.get(download)
	expectStatus(t, res, body, http.StatusNotFound, "")
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want only the Dataset", n)
	}
	res, body = ada.get("/lab")
	if freed := ada.storedKB(); freed == full {
		t.Fatalf("storage used is still %s after the delete", full)
	} else {
		expectStatus(t, res, body, http.StatusOK, freed)
	}
	if after := visitor.get2(link + visitor.dataOf(link)); after != before {
		t.Fatalf("the Deployment's data.json changed:\n%s\nbecame\n%s", before, after)
	}
	res, body = ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, "")
	res, body = ada.post(del, url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusNotFound, "")
}

// get2 is the body of a GET that must be a 200.
func (s *site) get2(path string) string {
	s.t.Helper()
	res, body := s.get(path)
	expectStatus(s.t, res, body, http.StatusOK, "")
	return body
}

func TestAnotherLearnersWorkbookCannotBeDeleted(t *testing.T) {
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	del := ada.deletePath()
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	admin.assignPlan("bea@example.com", admin.onlyPlanID())

	for _, c := range []struct {
		who  *site
		name string
	}{{bea, "another Learner"}, {admin, "the Admin"}} {
		res, body := c.who.get(del)
		expectStatus(t, res, body, http.StatusNotFound, "")
		res, body = c.who.post(del, url.Values{"confirm": {"yes"}})
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "finance-sample.xlsx")
	res, body = ada.get("/workbooks/999999/delete")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestWithoutAnActivePlanAWorkbookCannotBeDeleted(t *testing.T) {
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	del := ada.deletePath()

	admin.ageAllPeriods(31)
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "finance-sample.xlsx")
	if strings.Contains(body, "/delete") {
		t.Fatalf("the Folder offers deleting after the Plan ended: %s", body)
	}
	res, body = ada.get(del)
	expectRedirect(t, res, body, "/pricing")
	res, body = ada.post(del, url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	res, body = ada.get(ada.workbookPath())
	expectStatus(t, res, body, http.StatusOK, "")
}

func TestTheDeleteWorkbookScreenAndFolderLinkAreInMalay(t *testing.T) {
	_, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	res, body := ada.getFolderAsking("ms")
	expectStatus(t, res, body, http.StatusOK, "Padam")
	res, body = ada.getAsking(ada.deletePath(), "ms")
	expectStatus(t, res, body, http.StatusOK, "Dashboard yang menggunakannya: 1.")
}

func TestTheStorageFullMessageAdvisesDeletingWorkbooks(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	if _, err := ada.db.Exec(`INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, 'workbook-filler', 'filler.xlsx', 900000, '\x00')`, ada.adaID()); err != nil {
		t.Fatal(err)
	}
	path := ada.createWork("Kedai")
	res, body := ada.postFile(path+"/dataset", "besar.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "delete Workbooks in your Folder")
	ada.switchLanguage("ms", "/")
	res, body = ada.postFile(path+"/dataset", "besar.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "padam Buku kerja")
}
