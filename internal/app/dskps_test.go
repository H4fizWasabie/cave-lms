package app_test

import (
	"net/http"
	"net/url"
	"testing"
)

const adminPassword = "correct horse battery"

// newDSKPAdmin makes a site with the Admin logged in.
func newDSKPAdmin(t *testing.T) *site {
	t.Helper()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", adminPassword)
	return s
}

// uploadDSKP posts the DSKP upload form as the logged-in Admin.
func (s *site) uploadDSKP(subject, year, language, filename string, pdf []byte) (*http.Response, string) {
	s.t.Helper()
	return s.postFile("/admin/dskps", filename, pdf, url.Values{"subject": {subject}, "year": {year}, "language": {language}})
}

func TestTheAdminAndTeachersAreToldDSKPReadingIsOffWhileItsModelIsUnset(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	s.ai.DSKPOff()

	res, body := s.get("/admin/dskps")
	expectStatus(t, res, body, http.StatusOK, "DSKP reading is off")
	expectNone(t, body, `name="file"`)
	res, body = s.uploadDSKP("Sains", "Y4", "ms", "sains.pdf", pdfFile)
	expectStatus(t, res, body, http.StatusServiceUnavailable, "DSKP reading is off")
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("an upload while reading is off was read: %d requests", len(got))
	}

	teacher := s.second()
	teacher.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	res, body = teacher.saveTeacherProfile("Sains", "Y4")
	expectRedirect(t, res, body, "/teachers")
	res, body = teacher.get("/teachers/dskps")
	expectStatus(t, res, body, http.StatusOK, "DSKP reading is off")
}
