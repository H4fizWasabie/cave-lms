package app_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var workbookLink = regexp.MustCompile(`href="(/workbooks/\d+)"`)

func TestAnUploadOnADashboardIsKeptAsAWorkbookInTheFolder(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	file := shopReport(t, "Beras", 120)
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", file)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get("/my-folder")
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
	links := workbookLink.FindStringSubmatch(body)
	if links == nil {
		t.Fatalf("the Folder lists no Workbook download: %s", body)
	}
	res, got := ada.get(links[1])
	expectStatus(t, res, got, http.StatusOK, "")
	if got != string(file) {
		t.Fatalf("the download is %d bytes, want the %d uploaded", len(got), len(file))
	}
	if !strings.Contains(res.Header.Get("Content-Disposition"), "laporan.xlsx") {
		t.Errorf("Content-Disposition = %q", res.Header.Get("Content-Disposition"))
	}
}

// forgetWorkbooks deletes every Workbook, for tests of the room Datasets alone take: deleting a Workbook is a later step.
func (s *site) forgetWorkbooks() {
	s.t.Helper()
	if _, err := s.db.Exec(`DELETE FROM workbooks`); err != nil {
		s.t.Fatal(err)
	}
}

// workbookPath is the download link of the Learner's only Workbook, read from their Folder.
func (s *site) workbookPath() string {
	s.t.Helper()
	res, body := s.get("/my-folder")
	expectStatus(s.t, res, body, http.StatusOK, "/workbooks/")
	return workbookLink.FindStringSubmatch(body)[1]
}

func TestAWorkbookStaysWhenCleanUpFailsOrIsNeverConfirmed(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	res, body := ada.postFile(path+"/dataset", "rosak.xlsx", []byte("not a workbook"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "We could not read rosak.xlsx")
	res, body = ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120)) // never confirmed
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get("/my-folder")
	expectStatus(t, res, body, http.StatusOK, "rosak.xlsx")
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
	if n := len(workbookLink.FindAllString(body, -1)); n != 2 {
		t.Fatalf("%d Workbooks listed, want 2", n)
	}
}

func TestBuildingADashboardFromExcelKeepsTheWorkbookEvenWhenCleanUpFails(t *testing.T) {
	_, ada := labLearner(t)
	file := shopReport(t, "Beras", 120)
	res, body := ada.postFile("/lab/dashboards/build", "laporan.xlsx", file, url.Values{"name": {"Kedai"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.postFile("/lab/dashboards/build", "rosak.xlsx", []byte("not a workbook"), url.Values{"name": {"Rosak"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "rosak.xlsx")
	res, body = ada.postFile("/lab/dashboards/build", "tanpa-nama.xlsx", file, url.Values{"name": {""}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give the Dashboard a name.")

	_, body = ada.get("/my-folder")
	if n := len(workbookLink.FindAllString(body, -1)); n != 2 || strings.Contains(body, "tanpa-nama.xlsx") {
		t.Fatalf("%d Workbooks listed, want laporan.xlsx and rosak.xlsx only: %s", n, body)
	}
	_, got := ada.get(workbookLink.FindAllStringSubmatch(body, -1)[1][1]) // the oldest
	if got != string(file) {
		t.Fatalf("the built Dashboard's Workbook is %d bytes, want the %d uploaded", len(got), len(file))
	}
}

func TestTheDatasetConfirmedFromAnUploadRecordsItsWorkbook(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "laporan.xlsx", shopReport(t, "Beras", 120))
	var name sql.NullString
	if err := ada.db.QueryRow(`SELECT w.name FROM datasets d JOIN workbooks w ON w.id = d.workbook_id WHERE d.state = 'current'`).Scan(&name); err != nil || name.String != "laporan.xlsx" {
		t.Fatalf("the current Dataset's Workbook is %q, %v", name.String, err)
	}
	// A Dataset from before has no Workbook and keeps working.
	if _, err := ada.db.Exec(`UPDATE datasets SET workbook_id = NULL`); err != nil {
		t.Fatal(err)
	}
	res, body := ada.get(path + "/view/")
	expectStatus(t, res, body, http.StatusOK, "")
	if got := ada.dataOf(path + "/view/"); got == "" {
		t.Fatal("the Dashboard has no data")
	}
}

func TestStorageUsedCountsWorkbooksButNotSubmissions(t *testing.T) {
	admin, ada, planID, courseID := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	admin.addAssignment(courseID, "Write a prompt", "Write one prompt.")
	admin.post("/admin/assignments/"+admin.assignmentIDByTitle("Write a prompt")+"/publish", nil)
	ada.post("/courses/"+courseID+"/join", nil)
	ada.postFiles("/courses/"+courseID+"/assignments/"+admin.assignmentIDByTitle("Write a prompt")+"/files", upload{"prompt.pdf", pdfHandIn})
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 GB")

	file := bigFinance(t, 1)
	path := ada.createWork("Kedai")
	res, body = ada.postFile(path+"/dataset", "laporan.xlsx", file) // pending: only the Workbook counts
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, fmt.Sprintf("%d KB of 1 GB", (len(file)+1023)>>10))
}

func TestAWorkbookThatDoesNotFitIsRefusedAndNothingIsKept(t *testing.T) {
	_, ada, _ := storageSite(t, "1")
	if _, err := ada.db.Exec(`INSERT INTO workbooks (learner_id, storage_key, name, size) VALUES ($1, 'workbook-filler', 'filler.xlsx', 900000)`, ada.adaID()); err != nil {
		t.Fatal(err)
	}
	path := ada.createWork("Kedai")
	res, body := ada.postFile(path+"/dataset", "besar.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after a refused upload", n)
	}
	_, body = ada.get("/my-folder")
	if strings.Contains(body, "besar.xlsx") {
		t.Fatalf("the refused Workbook is listed: %s", body)
	}

	res, body = ada.postFile("/lab/dashboards/build", "besar.xlsx", bigFinance(t, 1), url.Values{"name": {"Baru"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after a refused build", n)
	}
}

func TestOnlyTheOwnerAndTheAdminDownloadAWorkbookAndAnEndedPlanStillCan(t *testing.T) {
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	file := shopReport(t, "Beras", 120)
	ada.postFile(ada.createWork("Kedai")+"/dataset", "laporan.xlsx", file)
	link := ada.workbookPath()

	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	res, body := bea.get(link)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, got := admin.get(link)
	expectStatus(t, res, got, http.StatusOK, "")
	if got != string(file) {
		t.Fatalf("the Admin's download is %d bytes, want %d", len(got), len(file))
	}
	res, body = admin.second().get(link)
	expectRedirect(t, res, body, "/login?next="+url.QueryEscape(link))
	res, body = ada.get("/workbooks/999999")
	expectStatus(t, res, body, http.StatusNotFound, "")

	admin.ageAllPeriods(31)
	res, got = ada.get("/lab")
	expectRedirect(t, res, got, "/pricing")
	res, got = ada.get(link)
	expectStatus(t, res, got, http.StatusOK, "")
	if got != string(file) {
		t.Fatalf("the download after the Plan ended is %d bytes, want %d", len(got), len(file))
	}
	res, body = ada.get("/my-folder")
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
}

func TestAFreeLearnersFolderHasNoWorkbooksPart(t *testing.T) {
	_, ada, _, _ := labSetup(t)
	res, body := ada.get("/my-folder")
	expectStatus(t, res, body, http.StatusOK, "You have not handed in anything yet.")
	if strings.Contains(body, "Workbooks") || strings.Contains(body, "/workbooks/") {
		t.Fatalf("a free Learner's Folder has a Workbooks part: %s", body)
	}
}

func TestTheFoldersWorkbooksPartIsInMalay(t *testing.T) {
	_, ada := labLearner(t)
	ada.postFile(ada.createWork("Kedai")+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	res, body := ada.getAsking("/my-folder", "ms")
	expectStatus(t, res, body, http.StatusOK, "Buku kerja")
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
}
