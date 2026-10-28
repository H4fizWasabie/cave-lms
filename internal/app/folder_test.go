package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// getFolder opens the Folder the way a Learner does, at /my-folder, which leads to the Folder at their Username.
func (s *site) getFolder() (*http.Response, string) {
	s.t.Helper()
	return s.getFolderAsking("en")
}

// getFolderAsking is getFolder asking for lang.
func (s *site) getFolderAsking(lang string) (*http.Response, string) {
	s.t.Helper()
	res, body := s.getAsking("/my-folder", lang)
	if res.StatusCode == http.StatusFound {
		return s.getAsking(res.Header.Get("Location"), lang)
	}
	return res, body
}

func TestTheFolderListsALearnersSubmissionsByCourseWithComments(t *testing.T) {
	admin, ada, path := publishedAssignment(t)
	aid := strings.Split(path, "/")[4]
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	admin.post("/admin/assignments/"+aid+"/learners/"+admin.adaID()+"/comment", url.Values{"comment": {"Well done."}})

	admin.createCourse("Data Basics", "Charts.", "")
	var other string
	if err := admin.db.QueryRow(`SELECT id::text FROM courses WHERE title = 'Data Basics'`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	admin.addAssignment(other, "Draw a chart", "One chart.")
	chart := admin.assignmentIDByTitle("Draw a chart")
	admin.post("/admin/assignments/"+chart+"/publish", nil)
	ada.post("/courses/"+other+"/join", nil)
	ada.postFiles("/courses/"+other+"/assignments/"+chart+"/files", upload{"chart.png", pngHandIn})
	ada.leaveCourse(other)

	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	res, body := bea.get("/my-folder/bea-example-com")
	expectStatus(t, res, body, http.StatusOK, "You have not handed in anything yet.")
	if strings.Contains(body, "prompt.pdf") {
		t.Fatalf("Bea sees Ada's files: %s", body)
	}

	res, body = ada.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, `href="/my-folder"`)
	_, body, _ = strings.Cut(body, "<h1>My folder</h1>") // past the header, which lists Courses too
	goBasics, dataBasics := strings.Index(body, "Go Basics"), strings.Index(body, "Data Basics")
	if goBasics < 0 || dataBasics < 0 {
		t.Fatalf("the Folder lacks a Course: %s", body)
	}
	data := body[dataBasics:]
	if dataBasics < goBasics {
		data = body[dataBasics:goBasics]
	}
	for _, want := range []string{"Write a prompt", "prompt.pdf", "Well done.", "Draw a chart", "chart.png"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Folder lacks %q", want)
		}
	}
	if !strings.Contains(data, "Read only") {
		t.Errorf("the Course Ada left is not marked read only: %s", data)
	}
	if links := fileLink.FindAllString(body, -1); len(links) != 2 {
		t.Errorf("%d download links, want 2", len(links))
	}
	if strings.Contains(body, "/delete") {
		t.Errorf("the Folder offers a delete: %s", body)
	}

	res, _ = admin.second().get("/my-folder/ada-example-com")
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("a Visitor got %d to %q, want sent to log in", res.StatusCode, res.Header.Get("Location"))
	}
}

// The Folder is at the Learner's own Username (ADR 0023): /my-folder leads there, and so does another Learner's address.
func TestTheFolderIsAtTheLearnersUsernameAndNobodyElsesLeadsThere(t *testing.T) {
	admin, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")

	for _, c := range []struct {
		site       *site
		path, want string
	}{
		{ada, "/my-folder", "/my-folder/ada-example-com"},
		{bea, "/my-folder", "/my-folder/bea-example-com"},
		{bea, "/my-folder/ada-example-com", "/my-folder/bea-example-com"},
		{bea, "/my-folder/nobody", "/my-folder/bea-example-com"},
	} {
		res, _ := c.site.get(c.path)
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != c.want {
			t.Errorf("GET %s = %d to %q, want 302 to %q", c.path, res.StatusCode, res.Header.Get("Location"), c.want)
		}
	}
	res, body := ada.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, "prompt.pdf")

	// Changing the Username moves the Folder, and the old address leads to the new one.
	res, body = ada.post("/my-profile/username", url.Values{"username": {"ada-l"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, _ = ada.get("/my-folder/ada-example-com")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/my-folder/ada-l" {
		t.Errorf("the old Folder address went %d to %q, want 302 to /my-folder/ada-l", res.StatusCode, res.Header.Get("Location"))
	}
	res, body = ada.get("/my-folder/ada-l")
	expectStatus(t, res, body, http.StatusOK, "prompt.pdf")
}

func TestTheFolderIsInTheMyMenuInMalay(t *testing.T) {
	_, ada, _ := publishedAssignment(t)
	res, body := ada.getFolderAsking("ms")
	expectStatus(t, res, body, http.StatusOK, `<a href="/my-folder">Folder saya</a>`)
	expectStatus(t, res, body, http.StatusOK, "Anda belum menyerahkan apa-apa.")
}

// pastIntro is the Folder's page below its heading, past the header, which lists Courses too.
func pastIntro(t *testing.T, body string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, "<h1>My folder</h1>")
	if !ok {
		t.Fatalf("no Folder heading: %s", body)
	}
	return rest
}

const labIntro = "Your Workbooks, your Dashboards&#39; pages and your Submissions"

func TestALearnerWithAPlanAndNoCourseSeesTheLabPartAndNoJoinACourseEmptyState(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, labIntro)
	rest := pastIntro(t, body)
	for _, gone := range []string{"You have not handed in anything yet", "empty-state"} {
		if strings.Contains(rest, gone) {
			t.Errorf("a Learner with a Plan is told %q: %s", gone, rest)
		}
	}
	if !strings.Contains(rest, "<h2>Workbooks</h2>") {
		t.Errorf("the Folder has no Workbooks part: %s", rest)
	}
}

func TestALearnerWithAPlanAndSubmissionsSeesTheLabPartFirstAndSubmissionsBelow(t *testing.T) {
	admin, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	admin.createPlan("Lab Monthly", "30", "3")
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	ada.createPage("Jualan", myPage)
	ada.postFile("/workbooks", "laporan.xlsx", shopReport(t, "Beras", 120))

	_, body := ada.getFolder()
	rest := pastIntro(t, body)
	workbooks, pages, course := strings.Index(rest, "laporan.xlsx"), strings.Index(rest, "Jualan"), strings.Index(rest, "Go Basics")
	if workbooks < 0 || pages < workbooks || course < pages {
		t.Fatalf("want Workbooks, then pages, then the Course; got %d, %d, %d: %s", workbooks, pages, course, rest)
	}
	if !strings.Contains(rest, "prompt.pdf") || strings.Contains(rest, "You have not handed in anything yet") {
		t.Errorf("the Submission is missing or the empty state shows: %s", rest)
	}
}

func TestTheFolderListsEachDashboardsCurrentAndEarlierPagesLinkedToTheDashboard(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", myPage)
	ada.uploadPage(path, versionOf(2))
	built := ada.readyDashboard("Kewangan") // a built Dashboard has no page, only its Workbook's "used on" line

	_, body := ada.getFolder()
	rest := pastIntro(t, body)
	if n := strings.Count(rest, `href="`+path+`"`); n != 3 { // its name and each of its two pages
		t.Errorf("%d links to the page Dashboard, want 3: %s", n, rest)
	}
	for _, want := range []string{"Jualan", "Current", "Earlier"} {
		if !strings.Contains(rest, want) {
			t.Errorf("the Folder lacks %q: %s", want, rest)
		}
	}
	if strings.Contains(rest, "<h3><a href=\""+built+"\">") {
		t.Errorf("the Folder lists a Dashboard with no page: %s", rest)
	}
}

func TestAFreeLearnersFolderKeepsTodaysIntroAndEmptyState(t *testing.T) {
	_, ada, _, _ := labSetup(t)
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "Your Submissions to Assignments, by Course")
	expectStatus(t, res, body, http.StatusOK, "You have not handed in anything yet.")
	expectStatus(t, res, body, http.StatusOK, `href="/my-courses"`)
	if strings.Contains(body, labIntro) || strings.Contains(body, "<h2>Workbooks</h2>") || strings.Contains(body, `action="/workbooks"`) {
		t.Errorf("a free Learner sees the Lab part: %s", body)
	}
}

// Once the last Plan period ends the Lab part stays for as long as Workbooks are kept, to download; without any it goes.
func TestTheLabPartStaysForAnEndedPlanWithWorkbooksAndGoesWithoutThem(t *testing.T) {
	admin, ada := labLearner(t)
	ada.createPage("Jualan", myPage)
	ada.postFile("/workbooks", "laporan.xlsx", shopReport(t, "Beras", 120))
	admin.ageAllPeriods(31)
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, labIntro)
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
	if rest := pastIntro(t, body); strings.Contains(rest, "empty-state") {
		t.Errorf("the empty state shows beside kept Workbooks: %s", rest)
	}

	if _, err := ada.db.Exec(`DELETE FROM workbooks`); err != nil {
		t.Fatal(err)
	}
	res, body = ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "You have not handed in anything yet.")
	if strings.Contains(body, labIntro) {
		t.Errorf("an ended Plan with no Workbooks keeps the Lab intro: %s", body)
	}
}

func TestTheLabAndTheDataTabLinkToTheFolder(t *testing.T) {
	_, ada := labLearner(t)
	// The header links to the Folder too, so look for the sentence each page puts before its own link.
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `Your Workbooks and your Dashboards&#39; pages are in <a href="/my-folder">My folder</a>.`)
	path := ada.readyDashboard("Kewangan")
	res, body = ada.get(path + "/data")
	expectStatus(t, res, body, http.StatusOK, `Your Workbooks are in <a href="/my-folder">My folder</a>.`)
	// A Dashboard with no data yet links to the Folder too, where the Workbooks to pick from are.
	res, body = ada.get(ada.createPage("Jualan", myPage) + "/data")
	expectStatus(t, res, body, http.StatusOK, `Your Workbooks and your Dashboards&#39; pages are in <a href="/my-folder">My folder</a>.`)
}

func TestTheFolderLabPartIsInMalay(t *testing.T) {
	_, ada := labLearner(t)
	ada.createPage("Jualan", myPage)
	ada.switchLanguage("ms", "/")
	res, body := ada.getFolderAsking("ms")
	expectStatus(t, res, body, http.StatusOK, "Buku kerja, halaman Dashboard dan Serahan anda")
	expectStatus(t, res, body, http.StatusOK, "Halaman Dashboard")
	expectStatus(t, res, body, http.StatusOK, "Semasa")
	res, body = ada.getAsking("/lab", "ms")
	expectStatus(t, res, body, http.StatusOK, `Buku kerja dan halaman Dashboard anda ada di <a href="/my-folder">Folder saya</a>.`)
}

func TestTheFolderListsEveryDashboardAWorkbookWasUsedOnAndSaysWhenNone(t *testing.T) {
	_, ada := labLearner(t)
	kewangan := ada.readyDashboard("Kewangan")
	stok := ada.readyDashboard("Stok") // the same file, so the same Workbook
	ada.postFile("/workbooks", "rak.xlsx", shopReport(t, "Gula", 80))

	_, body := ada.getFolder()
	rest := pastIntro(t, body)
	expectAll(t, rest, `<a href="`+kewangan+`">Kewangan</a>`, `<a href="`+stok+`">Stok</a>`, "not used on a Dashboard yet")
	if n := strings.Count(rest, "not used on a Dashboard yet"); n != 1 {
		t.Errorf("%d unused Workbooks, want only rak.xlsx: %s", n, rest)
	}

	ada.switchLanguage("ms", "/")
	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, "belum digunakan pada Dashboard")
}

func TestTheAdminSeesWhichDashboardsAWorkbookWasUsedOnAsPlainText(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")

	res, body := admin.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, "Kewangan")
	if strings.Contains(body, `href="`+path+`"`) {
		t.Errorf("the Admin is linked to a Dashboard that is not theirs to open: %s", body)
	}
}
