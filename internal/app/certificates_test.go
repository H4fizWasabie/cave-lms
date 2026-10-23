package app_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

const certPassword = "correct horse battery"

var certificateLink = regexp.MustCompile(`/certificates/([A-Za-z0-9_-]+)/pdf`)

// learnerEnrolledIn makes a Course with the given published text Lessons, then logs in as Ada, who has joined it.
func (s *site) learnerEnrolledIn(title string, lessons ...string) string {
	s.t.Helper()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse(title, "Learn it.", "")
	id := s.onlyCourseID()
	for _, l := range lessons {
		s.addTextLesson(id, l, "", "Body.", false)
	}
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	s.post("/courses/"+id+"/join", nil)
	return id
}

// certificateIDs returns the Certificate IDs linked from a page, in the order listed.
func certificateIDs(body string) []string {
	var ids []string
	for _, m := range certificateLink.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// pdfBody downloads a Certificate PDF, from a browser asking for English, and checks it is a PDF, returning its bytes as text.
func (s *site) pdfBody(path string) string {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodGet, s.server.URL+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", "en") // a saved language still wins
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(raw), "%PDF") {
		s.t.Fatalf("GET %s = %d %q, want a PDF; body starts %.40q", path, res.StatusCode, res.Header.Get("Content-Type"), raw)
	}
	return string(raw)
}

func TestLearnerGetsCertificateWhenLastLessonIsCompleted(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")

	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")

	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
}

func TestNoCertificateWhileALessonIsStillIncomplete(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables", "Functions")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")
}

func TestLearnerDownloadsACertificatePDFWithTheirDetails(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Download PDF")
	certID := certificateIDs(body)[0]

	pdf := s.pdfBody("/certificates/" + certID + "/pdf")
	for _, want := range []string{certID, "Ada", "Go Basics", "https://cave.example.invalid/certificates/" + certID} {
		if !strings.Contains(pdf, want) {
			t.Fatalf("certificate PDF lacks %q", want)
		}
	}
}

func TestCertificateIDIsLongAndUniquePerCertificate(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.post("/courses/"+id+"/lessons/"+s.lessonIDByTitle("Variables")+"/incomplete", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("got %d Certificates after completing twice, want 1: %v", len(ids), ids)
	}
	if len(ids[0]) < 22 {
		t.Fatalf("certificate ID %q is shorter than 16 random bytes encode to", ids[0])
	}
}

func TestOtherLearnersAndVisitorsCannotDownloadACertificate(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	path := "/certificates/" + certificateIDs(body)[0] + "/pdf"

	s.logOut()
	res, body := s.get(path)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.loginAsNewLearner("Grace", "grace@example.com", certPassword)
	res, body = s.get(path)
	expectStatus(t, res, body, http.StatusNotFound, "There is no Certificate at this address.")
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")
}

func TestAdminCanDownloadAnyCertificate(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	path := "/certificates/" + certificateIDs(body)[0] + "/pdf"

	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.pdfBody(path)
}

func TestDraftLessonDoesNotHoldBackACertificate(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse("Go Basics", "Learn it.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Body.", false)
	s.addTextLesson(id, "Secret", "", "Not yet.", true)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	s.post("/courses/"+id+"/join", nil)

	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
}

func TestUsableExamNeedsAPassAndAnUnusableOneDoesNot(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "What is a slice?")

	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")

	res, _ = s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})
	s.releaseResults()
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")

	res, _ = s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.") // the pass is not released yet
	s.releaseResults()
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
}

func TestCertificateStaysValidWhenTheCourseChanges(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	path := "/certificates/" + certificateIDs(body)[0] + "/pdf"

	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.post("/admin/courses/"+id, url.Values{"title": {"Go Advanced"}, "description": {"More."}, "start_date": {""}})
	if _, err := s.db.Exec(`UPDATE learners SET name = 'Ada Lovelace' WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}

	pdf := s.pdfBody(path)
	if !strings.Contains(pdf, "Go Basics") || strings.Contains(pdf, "Go Advanced") || strings.Contains(pdf, "Ada Lovelace") {
		t.Fatalf("issued Certificate changed with the Course or the Learner's name")
	}
	s.logOut()
	s.loginAsNewLearner("Grace", "grace@example.com", certPassword)
	s.post("/courses/"+id+"/join", nil)
	_, body = s.get("/my-certificates")
	if !strings.Contains(body, "You have no Certificates yet.") {
		t.Fatalf("another Learner got a Certificate: %s", body)
	}
}

// postCertificateSettings posts the Admin's Certificate details form, with files attached under their field names.
func (s *site) postCertificateSettings(fields map[string]string, files map[string][]byte) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			s.t.Fatal(err)
		}
	}
	for name, content := range files {
		fw, err := mw.CreateFormFile(name, name+".bin")
		if err != nil {
			s.t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			s.t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/admin/certificate-settings", &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req)
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAdminSetsCertificateDetailsOnEveryPDF(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	path := "/certificates/" + certificateIDs(body)[0] + "/pdf"

	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	res, body := s.postCertificateSettings(map[string]string{"organisation_name": "Cave Academy", "signatory_name": "Dr Jane Smith"},
		map[string][]byte{"logo": pngBytes(t), "signature": pngBytes(t)})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	pdf := s.pdfBody(path)
	for _, want := range []string{"Cave Academy", "Dr Jane Smith"} {
		if !strings.Contains(pdf, want) {
			t.Fatalf("certificate PDF lacks %q", want)
		}
	}
	res, body = s.get("/admin/certificate-settings")
	expectStatus(t, res, body, http.StatusOK, "Logo set.")
}

func TestCertificateSettingsRefuseLearnersAndBadImages(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	res, body := s.get("/admin/certificate-settings")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.postCertificateSettings(map[string]string{"organisation_name": "Hijack", "signatory_name": "Nobody"}, nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")

	s.logOut()
	res, body = s.get("/admin/certificate-settings")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	res, body = s.postCertificateSettings(map[string]string{"organisation_name": "Cave Academy", "signatory_name": "Dr Jane Smith"},
		map[string][]byte{"logo": []byte("not an image at all")})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "The logo must be a JPEG or PNG image.")
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("refused logo left %d stored file(s)", len(entries))
	}
}

func TestCertificateSettingsRefuseATooLargeImageOrANonMultipartForm(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	big := append(pngBytes(t), make([]byte, 5<<20)...)
	res, body := s.postCertificateSettings(map[string]string{"organisation_name": "Cave Academy", "signatory_name": "Dr Jane Smith"},
		map[string][]byte{"logo": big})
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "Images are limited to 5 MB")
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "Certificate details")
	if strings.Contains(body, "2 GB") {
		t.Fatalf("certificate settings error speaks of Gallery uploads: %s", body)
	}

	res, body = s.post("/admin/certificate-settings", url.Values{"organisation_name": {"Cave Academy"}})
	expectStatus(t, res, body, http.StatusBadRequest, "Certificate details")
}

func TestRemovingTheLastIncompleteLessonIssuesTheCertificate(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables", "Functions")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	res, body := s.post("/admin/lessons/"+s.lessonIDByTitle("Functions")+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	s.login("ada@example.com", certPassword)
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
}
