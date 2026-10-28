package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// adaWithWorkAndPlan is a Learner with a Plan who kept a Workbook, handed in a file the Admin commented on and has a
// Dashboard page. The Admin is logged in too.
func adaWithWorkAndPlan(t *testing.T) (admin, ada *site, workbook []byte, dashboard string) {
	t.Helper()
	admin, ada, planID, courseID := labSetup(t)
	ada.post("/courses/"+courseID+"/join", nil)
	admin.addAssignment(courseID, "Write a prompt", "Write one prompt.")
	aid := admin.assignmentIDByTitle("Write a prompt")
	admin.post("/admin/assignments/"+aid+"/publish", nil)
	ada.postFiles("/courses/"+courseID+"/assignments/"+aid+"/files", upload{"prompt.pdf", pdfHandIn})
	admin.post("/admin/assignments/"+aid+"/learners/"+admin.adaID()+"/comment", url.Values{"comment": {"Well done."}})
	admin.assignPlan("ada@example.com", planID)
	workbook = shopReport(t, "Beras", 120)
	res, body := ada.postFile("/workbooks", "laporan.xlsx", workbook)
	expectRedirect(t, res, body, "/my-folder")
	dashboard = ada.createPage("Jualan", myPage)
	return admin, ada, workbook, dashboard
}

func TestTheAdminOpensALearnersFolderAndDownloadsTheirFiles(t *testing.T) {
	t.Parallel()
	admin, _, workbook, dashboard := adaWithWorkAndPlan(t)

	res, body := admin.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, "<h1>Folder of Ada</h1>")
	for _, want := range []string{"laporan.xlsx", "Jualan", "Current", "prompt.pdf", "Well done.", "Go Basics"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, `href="`+dashboard+`"`) {
		t.Errorf("the Admin is linked to a Dashboard that is not theirs to open: %s", body)
	}
	for _, not := range []string{folderUploadForm, ">Delete<", "/delete"} {
		if strings.Contains(body, not) {
			t.Errorf("the Admin can act on a Learner's files from their Folder (%q): %s", not, body)
		}
	}

	res, got := admin.get(workbookLink.FindStringSubmatch(body)[1])
	expectStatus(t, res, got, http.StatusOK, "")
	if got != string(workbook) {
		t.Fatalf("the Admin's Workbook download is %d bytes, want the %d uploaded", len(got), len(workbook))
	}
	res, got = admin.get(fileLink.FindStringSubmatch(body)[1])
	expectStatus(t, res, got, http.StatusOK, string(pdfHandIn))
}

func TestTheLearnersPlanNotTheAdminsDecidesTheLabPartOfTheirFolder(t *testing.T) {
	t.Parallel()
	admin, _, _, _ := labSetup(t) // Ada has no Plan, the Admin always has the Lab
	res, body := admin.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, "<h1>Folder of Ada</h1>")
	for _, not := range []string{"<h2>Workbooks</h2>", folderUploadForm, labIntro} {
		if strings.Contains(body, not) {
			t.Errorf("a free Learner's Folder shows a Lab part to the Admin (%q): %s", not, body)
		}
	}
}

func TestTheAdminSeesTheLearnersFolderInMalay(t *testing.T) {
	t.Parallel()
	admin, _, _, _ := adaWithWorkAndPlan(t)
	res, body := admin.getAsking("/my-folder/ada-example-com", "ms")
	expectStatus(t, res, body, http.StatusOK, "<h1>Folder Ada</h1>")
}

func TestTheAdminGetsNotFoundForAnUnknownUsernameAndALearnerIsStillSentToTheirOwnFolder(t *testing.T) {
	t.Parallel()
	admin, ada, _, _ := labSetup(t)
	res, body := admin.get("/my-folder/nobody-here")
	expectStatus(t, res, body, http.StatusNotFound, "")

	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	for _, c := range []struct {
		who        *site
		path, want string
	}{{bea, "/my-folder/ada-example-com", "/my-folder/bea-example-com"}, {ada, "/my-folder/bea-example-com", "/my-folder/ada-example-com"}} {
		res, body = c.who.get(c.path)
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != c.want {
			t.Fatalf("GET %s = %d to %q, want a redirect to %s; body:\n%s", c.path, res.StatusCode, res.Header.Get("Location"), c.want, body)
		}
	}
}

func TestTheAdminLearnersPageLinksEachLearnerToTheirFolder(t *testing.T) {
	t.Parallel()
	admin, _, _, _ := labSetup(t)
	res, body := admin.get("/admin/learners")
	expectStatus(t, res, body, http.StatusOK, `href="/my-folder/ada-example-com"`)
}
