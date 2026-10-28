package app_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestALearnerSendsOneSubmissionUploadAtATime(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	ben := admin.second()
	ben.loginAsNewLearner("Ben Bell", "ben@example.com", "correct horse battery")
	ben.post("/courses/"+strings.Split(path, "/")[2]+"/join", nil)

	// Ada's first upload stays open: its body is a pipe that is not finished.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	t.Cleanup(func() { pw.Close() }) // so a failure here does not leave the server waiting for the body
	req, err := http.NewRequest(http.MethodPost, ada.server.URL+path+"/files", pr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept-Language", "en")
	done := make(chan int, 1)
	go func() {
		res, err := ada.client.Do(req)
		if err != nil {
			done <- 0
			return
		}
		res.Body.Close()
		done <- res.StatusCode
	}()
	go func() { // the first bytes make the client send the request
		fw, _ := mw.CreateFormFile("file", "prompt.pdf")
		fw.Write(pdfHandIn)
	}()

	// Wait until the server holds Ada's place. Probing with another upload instead would race the first one for it:
	// a probe that wins makes the first one 429, and the server then waits on its unfinished body forever.
	deadline := time.Now().Add(5 * time.Second)
	for ada.application.Uploading() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the first upload never reached the server")
		}
		time.Sleep(5 * time.Millisecond)
	}
	res, body := ada.postFiles(path+"/files", upload{"second.pdf", pdfHandIn})
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
	res, body = ben.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	mw.Close()
	pw.Close()
	if code := <-done; code != http.StatusSeeOther {
		t.Fatalf("first upload got %d, want 303", code)
	}
	res, body = ada.postFiles(path+"/files", upload{"more.pdf", pdfHandIn})
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

// A minimal file of each kind: only the first bytes are checked.
var (
	pdfHandIn  = []byte("%PDF-1.4\n% a hand-in\n")
	docxHandIn = []byte("PK\x03\x04 a Word document")
	pngHandIn  = []byte("\x89PNG\r\n\x1a\n a picture")
)

type upload struct {
	name    string
	content []byte
}

// postFiles posts a multipart form with every upload in a field named "file".
func (s *site) postFiles(path string, files ...upload) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		fw, err := mw.CreateFormFile("file", f.name)
		if err != nil {
			s.t.Fatal(err)
		}
		fw.Write(f.content)
	}
	mw.Close()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req)
}

// publishedAssignment makes "Go Basics" with Ada enrolled and a published Assignment, and returns the Admin, Ada and
// the Assignment's page.
func publishedAssignment(t *testing.T) (admin, ada *site, path string) {
	admin, ada, id := assignmentCourse(t)
	admin.addAssignment(id, "Write a prompt", "Write one prompt.")
	aid := admin.assignmentIDByTitle("Write a prompt")
	admin.post("/admin/assignments/"+aid+"/publish", nil)
	return admin, ada, "/courses/" + id + "/assignments/" + aid
}

var fileLink = regexp.MustCompile(`href="(/submission-files/\d+)"`)

// fileLinks returns the download links on the page at path.
func (s *site) fileLinks(path string) []string {
	s.t.Helper()
	_, body := s.get(path)
	var links []string
	for _, m := range fileLink.FindAllStringSubmatch(body, -1) {
		links = append(links, m[1])
	}
	return links
}

func TestALearnerHandsInFilesAndDownloadsThem(t *testing.T) {
	t.Parallel()
	_, ada, path := publishedAssignment(t)
	res, body := ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn}, upload{"notes.docx", docxHandIn})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if res.Header.Get("Location") != path {
		t.Fatalf("upload redirected to %q, want %q", res.Header.Get("Location"), path)
	}
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "prompt.pdf")
	expectStatus(t, res, body, http.StatusOK, "notes.docx")
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want 2", n)
	}
	links := ada.fileLinks(path)
	if len(links) != 2 {
		t.Fatalf("%d download links, want 2: %s", len(links), body)
	}
	res, body = ada.get(links[0])
	expectStatus(t, res, body, http.StatusOK, "")
	if body != string(pdfHandIn) || res.Header.Get("Content-Type") != "application/pdf" ||
		!strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download = %q, %q, %q", body, res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
}

func TestAnUploadOutsideTheLimitsIsRefusedAndNothingIsStored(t *testing.T) {
	t.Parallel()
	_, ada, path := publishedAssignment(t)
	big := make([]byte, 20<<20+1)
	copy(big, pdfHandIn)
	for _, c := range []struct {
		files []upload
		want  string
	}{
		{[]upload{{"notes.txt", []byte("plain text")}}, "notes.txt is not a PDF, Word, Excel, PowerPoint, PNG, JPG or zip file."},
		{[]upload{{"fake.pdf", pngHandIn}}, "fake.pdf is not a PDF, Word, Excel, PowerPoint, PNG, JPG or zip file."},
		{[]upload{{"prompt.pdf", pdfHandIn}, {"big.pdf", big}}, "big.pdf is over 20 MB."},
		{[]upload{{"1.pdf", pdfHandIn}, {"2.pdf", pdfHandIn}, {"3.pdf", pdfHandIn}, {"4.pdf", pdfHandIn}, {"5.pdf", pdfHandIn}, {"6.pdf", pdfHandIn}},
			"A Submission holds up to 5 files."},
		{nil, "Choose a file to upload."},
	} {
		res, body := ada.postFiles(path+"/files", c.files...)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		if n := ada.storedFiles(); n != 0 {
			t.Fatalf("after %q, %d stored files, want 0", c.want, n)
		}
	}

	ada.postFiles(path+"/files", upload{"1.pdf", pdfHandIn}, upload{"2.pdf", pdfHandIn}, upload{"3.pdf", pdfHandIn}, upload{"4.pdf", pdfHandIn})
	res, body := ada.postFiles(path+"/files", upload{"5.pdf", pdfHandIn}, upload{"6.pdf", pdfHandIn})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "A Submission holds up to 5 files.")
	if n := ada.storedFiles(); n != 4 {
		t.Fatalf("%d stored files, want 4", n)
	}
}

func TestALearnersSubmissionsStopAt200MBInTotal(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	// Ada already holds a file of 10 bytes under 200 MB; only its record matters for the total.
	if _, err := admin.db.Exec(`INSERT INTO submission_files (assignment_id, learner_id, name, size, content_type, storage_key)
		SELECT a.id, l.id, 'old.zip', 200 * 1024 * 1024 - 10, 'application/zip', 'old-key'
		FROM assignments a, learners l WHERE l.email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	res, body := ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "over 200 MB")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files, want 0", n)
	}
}

func TestALearnerDeletesAFileOnlyAfterConfirmingAndAddsAnother(t *testing.T) {
	t.Parallel()
	_, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"draft.pdf", pdfHandIn})
	link := ada.fileLinks(path)[0]
	confirmPage := link + "/delete"
	_, body := ada.get(path)
	expectAll(t, body, `href="`+confirmPage+`"`)
	res, body := ada.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "Delete “draft.pdf” from your Submission for “Write a prompt”? You cannot get it back.")
	expectAll(t, body, `href="`+path+`">Keep the file</a>`, `name="confirm" value="yes"`)
	res, body = ada.post(confirmPage, nil)
	expectRedirect(t, res, body, confirmPage)
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files after an unconfirmed delete, want 1", n)
	}

	res, body = ada.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get(path)
	if strings.Contains(body, "draft.pdf") {
		t.Fatalf("the deleted file is still listed: %s", body)
	}
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after the delete, want 0", n)
	}
	res, body = ada.postFiles(path+"/files", upload{"final.png", pngHandIn})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if links := ada.fileLinks(path); len(links) != 1 {
		t.Fatalf("%d files, want 1", len(links))
	}
}

func TestOnlyTheOwnerAndTheAdminDownloadAFile(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	link := ada.fileLinks(path)[0]

	res, body := admin.get(link)
	expectStatus(t, res, body, http.StatusOK, string(pdfHandIn))
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	if res, body := bea.get(link); res.StatusCode != http.StatusNotFound || strings.Contains(body, "a hand-in") {
		t.Fatalf("another Learner got %d downloading Ada's file", res.StatusCode)
	}
	if res, _ := bea.get(link + "/delete"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner got %d asking to delete Ada's file", res.StatusCode)
	}
	if res, _ := bea.post(link+"/delete", confirming); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner got %d deleting Ada's file", res.StatusCode)
	}
	if res, _ := admin.second().get(link); res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("a Visitor got %d to %q, want sent to log in", res.StatusCode, res.Header.Get("Location"))
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want 1", n)
	}
}

func TestALearnerWhoLeftTheCourseKeepsTheirFilesReadOnly(t *testing.T) {
	t.Parallel()
	_, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	link := ada.fileLinks(path)[0]
	courseID := strings.Split(path, "/")[2]
	ada.leaveCourse(courseID)

	res, body := ada.get(link)
	expectStatus(t, res, body, http.StatusOK, string(pdfHandIn))
	if res, _ := ada.postFiles(path+"/files", upload{"more.pdf", pdfHandIn}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner who left uploaded: %d", res.StatusCode)
	}
	if res, _ := ada.post(link+"/delete", confirming); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner who left deleted a file: %d", res.StatusCode)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want 1", n)
	}
}

func TestDeletingAnAssignmentOrALearnerDeletesTheirFilesFromStorage(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	admin.post("/admin/assignments/"+strings.Split(path, "/")[4]+"/delete", confirming)
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Assignment, want 0", n)
	}

	courseID := strings.Split(path, "/")[2]
	admin.addAssignment(courseID, "Second", "Again.")
	aid := admin.assignmentIDByTitle("Second")
	admin.post("/admin/assignments/"+aid+"/publish", nil)
	ada.postFiles("/courses/"+courseID+"/assignments/"+aid+"/files", upload{"prompt.pdf", pdfHandIn})
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want 1", n)
	}
	// There is no delete-account page; the Admin deletes the account row by hand (docs/deploy.md, Data requests).
	if _, err := admin.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	admin.prune()
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Learner, want 0", n)
	}
}

func TestThePrivacyNoticeSaysWhoSeesSubmissions(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.getAsking("/privacy", "en")
	expectStatus(t, res, body, http.StatusOK, "Only you and the Admins see them")
	res, body = s.getAsking("/privacy", "ms")
	expectStatus(t, res, body, http.StatusOK, "Hanya anda dan Pentadbir yang melihatnya")
}

func TestTheAdminSeesWhoHandedInAndDownloadsTheirFiles(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	courseID, aid := strings.Split(path, "/")[2], strings.Split(path, "/")[4]
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn}, upload{"notes.docx", docxHandIn})
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	bea.post("/courses/"+courseID+"/join", nil)
	cy := admin.second()
	cy.loginAsNewLearner("Cy Young", "cy@example.com", "correct horse battery")
	cy.post("/courses/"+courseID+"/join", nil)
	cy.postFiles(path+"/files", upload{"cy.png", pngHandIn})
	cy.leaveCourse(courseID)
	dan := admin.second()
	dan.loginAsNewLearner("Dan Brown", "dan@example.com", "correct horse battery")

	res, body := admin.get("/admin/assignments/" + aid)
	expectStatus(t, res, body, http.StatusOK, "<h1>Write a prompt</h1>")
	expectStatus(t, res, body, http.StatusOK, "<th>Submission</th>")
	if strings.Contains(body, "Your Submission") {
		t.Fatalf("the Admin's Submissions table is headed as if it were theirs: %s", body)
	}
	row := func(name string) string {
		_, after, ok := strings.Cut(body, name)
		if !ok {
			t.Fatalf("%s is not listed: %s", name, body)
		}
		end, _, _ := strings.Cut(after, "</tr>")
		return end
	}
	for name, want := range map[string]string{"Ada Lovelace": "2 files", "Bea Hopper": "Nothing yet", "Cy Young": "Left the Course"} {
		if r := row(name); !strings.Contains(r, want) {
			t.Errorf("%s's row lacks %q: %s", name, want, r)
		}
	}
	if !strings.Contains(row("Cy Young"), "1 file") {
		t.Errorf("Cy's row lacks 1 file")
	}
	if strings.Contains(body, "Dan Brown") {
		t.Errorf("a Learner who never joined is listed: %s", body)
	}
	links := fileLink.FindAllStringSubmatch(row("Ada Lovelace"), -1)
	if len(links) != 2 {
		t.Fatalf("Ada's row has %d download links, want 2", len(links))
	}
	res, body = admin.get(links[0][1])
	expectStatus(t, res, body, http.StatusOK, string(pdfHandIn))
}

// adaID reads Ada's ID from the database.
func (s *site) adaID() string {
	s.t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id::text FROM learners WHERE email = 'ada@example.com'`).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func TestTheAdminCommentsOnASubmissionAndTheLearnerSeesIt(t *testing.T) {
	t.Parallel()
	admin, ada, path := publishedAssignment(t)
	aid := strings.Split(path, "/")[4]
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	comment := "/admin/assignments/" + aid + "/learners/" + admin.adaID() + "/comment"

	res, body := admin.post(comment, url.Values{"comment": {"Good start.\nAdd an example."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Good start.\nAdd an example.")

	admin.post(comment, url.Values{"comment": {"Well done."}})
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Well done.")
	if strings.Contains(body, "Good start.") {
		t.Fatalf("the old comment still shows: %s", body)
	}
	res, body = admin.get("/admin/assignments/" + aid)
	expectStatus(t, res, body, http.StatusOK, ">Well done.</textarea>")

	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	var beaID string
	admin.db.QueryRow(`SELECT id::text FROM learners WHERE email = 'bea@example.com'`).Scan(&beaID)
	if res, _ := admin.post("/admin/assignments/"+aid+"/learners/"+beaID+"/comment", url.Values{"comment": {"Hi."}}); res.StatusCode != http.StatusNotFound {
		t.Fatalf("a comment for a Learner who never joined got %d, want 404", res.StatusCode)
	}

	if res, _ := ada.get("/admin/assignments/" + aid); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner opened the Admin's Assignment page: %d", res.StatusCode)
	}
	if res, _ := ada.post(comment, url.Values{"comment": {"I am the Admin."}}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner commented: %d", res.StatusCode)
	}
}
