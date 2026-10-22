package app_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// saveProgramme posts the Programme form to path as multipart, with picture as its file when not nil.
func (s *site) saveProgramme(path string, fields map[string]string, picture []byte) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			s.t.Fatal(err)
		}
	}
	if picture != nil {
		fw, err := mw.CreateFormFile("picture", "picture.png")
		if err != nil {
			s.t.Fatal(err)
		}
		if _, err := fw.Write(picture); err != nil {
			s.t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req)
}

// addProgramme adds a Programme as the logged-in Admin.
func (s *site) addProgramme(title, startsOn, blurb, link string, picture []byte) {
	s.t.Helper()
	res, body := s.saveProgramme("/admin/programmes", map[string]string{"title": title, "starts_on": startsOn, "blurb": blurb, "link": link}, picture)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

var programmeEditLink = regexp.MustCompile(`/admin/programmes/(\d+)/edit`)

func (s *site) onlyProgrammeID() string {
	s.t.Helper()
	_, body := s.get("/admin/programmes")
	m := programmeEditLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Programme on the Admin's Programmes page: %s", body)
	}
	return m[1]
}

func TestAdminAddsAProgrammeAndTheProgramPageListsIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("AI Bootcamp", "2099-03-01", "Two days of hands-on AI.", "", nil)

	res, body := s.get("/admin/programmes")
	expectStatus(t, res, body, http.StatusOK, "AI Bootcamp")
	res, body = s.get("/programmes")
	expectInOrder(t, content(body), "AI Bootcamp", "1 March 2099", "Two days of hands-on AI.")
	expectNone(t, content(body), "/programmes/pictures/", "No upcoming Programmes")
}

func TestTheProgramPageShowsAProgrammesPictureAndLinkButTheGalleryDoesNot(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("AI Bootcamp", "2099-03-01", "Two days.", "https://example.com/bootcamp", pngFile)

	_, body := s.get("/programmes")
	expectInOrder(t, content(body), `src="/programmes/pictures/`, `href="https://example.com/bootcamp"`)
	src := regexp.MustCompile(`src="(/programmes/pictures/[^"]+)"`).FindStringSubmatch(body)[1]
	res, picture := s.get(src)
	expectStatus(t, res, picture, http.StatusOK, "")
	if picture != string(pngFile) {
		t.Fatalf("served picture differs from the upload")
	}

	if n := s.countGalleryItems(); n != 0 {
		t.Fatalf("Gallery has %d items, want 0", n)
	}
	_, body = s.get("/gallery")
	expectNone(t, body, "/programmes/pictures/")
}

func TestAProgrammePictureMustBeAPicture(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.saveProgramme("/admin/programmes", map[string]string{"title": "AI Bootcamp", "starts_on": "2099-03-01"}, pdfFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "JPEG, PNG or WebP")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
}

func TestInvalidProgrammesAreRefusedWithAMessage(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	for _, tc := range []struct{ title, startsOn, link, want string }{
		{"  ", "2099-03-01", "", "Enter a title."},
		{"AI Bootcamp", "", "", "Enter a start date."},
		{"AI Bootcamp", "2099-03-01", "javascript:alert(1)", "https://"},
	} {
		res, body := s.saveProgramme("/admin/programmes", map[string]string{"title": tc.title, "starts_on": tc.startsOn, "link": tc.link}, nil)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, tc.want)
	}
}

func TestAdminEditsAProgramme(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("AI Bootcamp", "2099-03-01", "Two days.", "", pngFile)
	id := s.onlyProgrammeID()
	res, body := s.get("/admin/programmes/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `value="AI Bootcamp"`)

	res, body = s.saveProgramme("/admin/programmes/"+id, map[string]string{"title": "AI Masterclass", "starts_on": "2099-04-02", "blurb": "One day.", "link": "https://example.com/m"}, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/programmes")
	expectInOrder(t, content(body), "/programmes/pictures/", "AI Masterclass", "2 April 2099", "One day.", "https://example.com/m")
	expectNone(t, body, "AI Bootcamp")
}

func TestAdminDeletesAProgrammeAndItsPicture(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("AI Bootcamp", "2099-03-01", "Two days.", "", pngFile)
	id := s.onlyProgrammeID()

	res, body := s.post("/admin/programmes/"+id+"/delete", url.Values{})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/programmes")
	expectNone(t, body, "AI Bootcamp")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
}

func TestThePastProgrammesLeaveTheProgramPageButStayInTheAdminArea(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.addProgramme("Later one", "2099-06-01", "", "", nil)
	s.addProgramme("Past one", "2020-01-01", "", "", nil)
	s.addProgramme("Sooner one", "2099-01-01", "", "", nil)

	_, body := s.get("/programmes")
	expectInOrder(t, content(body), "Sooner one", "Later one")
	expectNone(t, body, "Past one")
	_, body = s.get("/admin/programmes")
	for _, want := range []string{"Past one", "Sooner one", "Later one"} {
		if !strings.Contains(body, want) {
			t.Errorf("Admin list lacks %q", want)
		}
	}
}

func TestTheProgramPageSaysWhenNoneAreComingInBothLanguages(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/programmes", "en")
	expectInOrder(t, content(body), "No upcoming Programmes")
	_, body = s.getAsking("/programmes", "ms")
	expectInOrder(t, content(body), "Tiada Program akan datang")
}

func TestTheProgramMenuLeadsToTheProgramPage(t *testing.T) {
	s := newSite(t)
	_, body := s.get("/")
	expectInOrder(t, header(t, body), ">Program<", `href="/programmes"`, ">Certification<")
}

func TestTheAdminSidebarHasProgrammes(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	_, body := s.get("/admin")
	expectInOrder(t, body, `href="/admin/programmes"`, "Programmes")
}

func TestOnlyTheAdminManagesProgrammes(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := s.saveProgramme("/admin/programmes", map[string]string{"title": "AI Bootcamp", "starts_on": "2099-03-01"}, nil)
	if res.StatusCode == http.StatusSeeOther {
		t.Fatalf("a Learner added a Programme; body:\n%s", body)
	}
}
