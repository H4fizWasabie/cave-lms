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
