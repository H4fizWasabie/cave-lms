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

var (
	svgLogo     = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 10"><rect width="20" height="10" fill="red"/></svg>`)
	svgScript   = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><script>alert(1)</script></svg>`)
	svgOnload   = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" onload="alert(1)"><rect width="10" height="10"/></svg>`)
	svgJSHref   = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><a href="javascript:alert(1)"><rect width="10" height="10"/></a></svg>`)
	orgEditLink = regexp.MustCompile(`/admin/organisations/(\d+)/edit`)
)

// saveOrganisation posts the Organisation form to path as multipart, with logo as its file when not nil.
func (s *site) saveOrganisation(path, name string, logo []byte) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("name", name); err != nil {
		s.t.Fatal(err)
	}
	if logo != nil {
		fw, err := mw.CreateFormFile("logo", "logo")
		if err != nil {
			s.t.Fatal(err)
		}
		if _, err := fw.Write(logo); err != nil {
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

// addOrganisation adds an Organisation as the logged-in Admin and returns its ID.
func (s *site) addOrganisation(name string, logo []byte) string {
	s.t.Helper()
	res, body := s.saveOrganisation("/admin/organisations", name, logo)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/organisations")
	m := orgEditLink.FindAllStringSubmatch(body, -1)
	if m == nil {
		s.t.Fatalf("no Organisation on the Admin's Organisations page: %s", body)
	}
	return m[len(m)-1][1]
}

// organisationLogo fetches the logo the Organisation's edit page shows.
func (s *site) organisationLogo(id string) (*http.Response, string) {
	s.t.Helper()
	_, body := s.get("/admin/organisations/" + id + "/edit")
	m := regexp.MustCompile(`src="(/admin/organisations/` + id + `/logo[^"]*)"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no logo on the edit page: %s", body)
	}
	return s.get(strings.ReplaceAll(m[1], "&amp;", "&"))
}

// liveCourseFor makes a self-paced Course and duplicates it into a Live Course for Organisation org ("" for none).
func (s *site) liveCourseFor(org string) string {
	s.t.Helper()
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	res, body := s.post("/admin/courses/"+s.onlyCourseID()+"/duplicate", url.Values{"title": {"Go for PDRM"}, "description": {"For PDRM."},
		"start_date": {"2027-02-01"}, "end_date": {"2027-02-03"}, "days": {"2"}, "organisation_id": {org}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return regexp.MustCompile(`\d+`).FindString(res.Header.Get("Location"))
}

func TestAdminAddsAnOrganisationAndTheListShowsItsNameLogoAndSearchLink(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.addOrganisation("Kementerian Pendidikan", pngFile)

	res, body := s.get("/admin/organisations")
	expectStatus(t, res, body, http.StatusOK, "Kementerian Pendidikan")
	expectInOrder(t, body, `src="/admin/organisations/`+id+`/logo`, `href="https://www.google.com/search?tbm=isch&amp;q=Kementerian&#43;Pendidikan" target="_blank" rel="noopener"`)
	res, logo := s.organisationLogo(id)
	expectStatus(t, res, logo, http.StatusOK, "")
	if logo != string(pngFile) || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("served logo %q as %q, want the upload as image/png", logo, res.Header.Get("Content-Type"))
	}

	s.post("/language", url.Values{"lang": {"ms"}, "next": {"/admin/organisations"}})
	_, body = s.get("/admin/organisations")
	expectInOrder(t, body, "Organisasi", "Kementerian Pendidikan")
}

func TestAnOrganisationNeedsANameAndALogo(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.saveOrganisation("/admin/organisations", "  ", pngFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a name.")
	res, body = s.saveOrganisation("/admin/organisations", "PDRM", nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Upload a PNG, JPG or SVG logo.")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
	_, body = s.get("/admin/organisations")
	expectNone(t, body, "PDRM")
}

func TestAnOrganisationLogoMustBeAPNGJPGOrScriptFreeSVG(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	for name, logo := range map[string][]byte{"PDF": pdfFile, "script": svgScript, "onload": svgOnload, "javascript link": svgJSHref} {
		res, body := s.saveOrganisation("/admin/organisations", "PDRM", logo)
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "Upload a PNG, JPG or SVG logo.") {
			t.Errorf("%s logo: got %d, want 422 with the message", name, res.StatusCode)
		}
	}
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
	s.addOrganisation("Giatmara", jpegFile)
}

func TestAnOrganisationLogoOver2MBIsRefusedAndNothingIsStored(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	big := append(append([]byte{}, pngFile...), bytes.Repeat([]byte{0}, 2<<20)...)
	res, body := s.saveOrganisation("/admin/organisations", "PDRM", big)
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "Logos are limited to 2 MB")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
	s.addOrganisation("PDRM", big[:2<<20]) // exactly 2 MB is allowed
}

func TestAnSVGLogoIsStoredAsAPNG(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.addOrganisation("PDRM", svgLogo)
	res, logo := s.organisationLogo(id)
	expectStatus(t, res, logo, http.StatusOK, "")
	if !strings.HasPrefix(logo, "\x89PNG") || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("SVG logo served as %q, want a PNG", res.Header.Get("Content-Type"))
	}
}

func TestAdminRenamesAnOrganisationAndANewLogoKeepsTheOldFile(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.addOrganisation("PDMR", pngFile)

	res, body := s.saveOrganisation("/admin/organisations/"+id, "PDRM", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/organisations")
	expectStatus(t, res, body, http.StatusOK, "PDRM")
	expectNone(t, body, "PDMR")
	if _, logo := s.organisationLogo(id); logo != string(pngFile) {
		t.Fatalf("renaming changed the logo")
	}

	res, body = s.saveOrganisation("/admin/organisations/"+id, "PDRM", jpegFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if _, logo := s.organisationLogo(id); logo != string(jpegFile) {
		t.Fatalf("the logo was not replaced")
	}
	if n := s.storedFiles(); n != 2 {
		t.Fatalf("%d files stored, want the old logo and the new one", n)
	}
}

func TestAnOrganisationIsDeletedOnlyWhenNoLiveCourseUsesIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	org := s.addOrganisation("PDRM", pngFile)
	course := s.liveCourseFor(org)

	res, body := s.post("/admin/organisations/"+org+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "A Live Course uses this Organisation")

	res, body = s.post("/admin/courses/"+course, url.Values{"title": {"Go for PDRM"}, "description": {"For PDRM."},
		"start_date": {"2027-02-01"}, "end_date": {"2027-02-03"}, "days": {"2"}, "organisation_id": {""}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.post("/admin/organisations/"+org+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/organisations")
	expectNone(t, body, "PDRM")
}

func TestTheLiveCourseFormSetsAndClearsTheOrganisationAndADuplicateKeepsIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	org := s.addOrganisation("PDRM", pngFile)
	s.addOrganisation("Giatmara", pngFile)
	course := s.liveCourseFor("")
	selected := `<option value="` + org + `" selected>PDRM</option>`

	res, body := s.get("/admin/courses/" + course + "/edit")
	expectStatus(t, res, body, http.StatusOK, "Giatmara")
	expectNone(t, body, selected)
	live := url.Values{"title": {"Go for PDRM"}, "description": {"For PDRM."}, "start_date": {"2027-02-01"}, "end_date": {"2027-02-03"}, "days": {"2"}}
	live.Set("organisation_id", org)
	s.post("/admin/courses/"+course, live)
	res, body = s.get("/admin/courses/" + course + "/edit")
	expectStatus(t, res, body, http.StatusOK, selected)

	// The duplicate form comes with the original's Organisation picked, so saving it as shown keeps it.
	res, body = s.get("/admin/courses/" + course + "/duplicate")
	expectStatus(t, res, body, http.StatusOK, selected)
	res, body = s.post("/admin/courses/"+course+"/duplicate", live)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, selected)

	live.Set("organisation_id", "")
	s.post("/admin/courses/"+course, live)
	_, body = s.get("/admin/courses/" + course + "/edit")
	expectNone(t, body, selected)

	// A self-paced Course's form has no Organisation.
	_, body = s.get("/admin/courses/new")
	expectNone(t, body, "organisation_id")
}

func TestTheLiveCourseFormRefusesAnOrganisationThatIsNotThere(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	org := s.addOrganisation("PDRM", pngFile)
	course := s.liveCourseFor("")
	s.post("/admin/organisations/"+org+"/delete", nil) // deleted while the form was open
	live := url.Values{"title": {"Go for PDRM"}, "description": {"For PDRM."}, "start_date": {"2027-02-01"}, "end_date": {"2027-02-03"}, "days": {"2"}}
	for _, id := range []string{org, "x"} {
		live.Set("organisation_id", id)
		res, body := s.post("/admin/courses/"+course, live)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose an Organisation from the list.")
		res, body = s.post("/admin/courses/"+course+"/duplicate", live)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose an Organisation from the list.")
	}
}
