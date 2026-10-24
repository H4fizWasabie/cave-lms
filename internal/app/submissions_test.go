package app_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

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

func TestALearnerDeletesAFileAndAddsAnother(t *testing.T) {
	_, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"draft.pdf", pdfHandIn})
	link := ada.fileLinks(path)[0]
	res, body := ada.post(link+"/delete", nil)
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
	if res, _ := bea.post(link+"/delete", nil); res.StatusCode != http.StatusNotFound {
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
	_, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	link := ada.fileLinks(path)[0]
	courseID := strings.Split(path, "/")[2]
	ada.post("/courses/"+courseID+"/leave", nil)

	res, body := ada.get(link)
	expectStatus(t, res, body, http.StatusOK, string(pdfHandIn))
	if res, _ := ada.postFiles(path+"/files", upload{"more.pdf", pdfHandIn}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner who left uploaded: %d", res.StatusCode)
	}
	if res, _ := ada.post(link+"/delete", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner who left deleted a file: %d", res.StatusCode)
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want 1", n)
	}
}

func TestDeletingAnAssignmentOrALearnerDeletesTheirFilesFromStorage(t *testing.T) {
	admin, ada, path := publishedAssignment(t)
	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	admin.post("/admin/assignments/"+strings.Split(path, "/")[4]+"/delete", nil)
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
	s := newSite(t)
	res, body := s.getAsking("/privacy", "en")
	expectStatus(t, res, body, http.StatusOK, "Only you and the Admins see them")
	res, body = s.getAsking("/privacy", "ms")
	expectStatus(t, res, body, http.StatusOK, "Hanya anda dan Pentadbir yang melihatnya")
}
