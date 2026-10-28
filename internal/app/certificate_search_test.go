package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// passedLiveCourse has Ada (IC 951206-08-5555) pass a 2-day Live Course and returns its ID and her Certificate's.
// Ada is logged out afterwards.
func passedLiveCourse(t *testing.T) (admin, ada *site, id, certID string) {
	t.Helper()
	admin, ada, id, qids := liveCourse(t, "2")
	res, body := ada.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	ada.releaseResults()
	_, body = ada.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates, want 1: %s", len(ids), body)
	}
	ada.logOut()
	return admin, ada, id, ids[0]
}

// search posts q to the check page's search form, as a browser asking for lang does, so the IC is never in a URL.
func searchAsking(s *site, q, lang string) (*http.Response, string) {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/certificates", strings.NewReader(url.Values{"q": {q}}.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", lang)
	return s.do(req)
}

func search(s *site, q string) (*http.Response, string) { return searchAsking(s, q, "en") }

func TestTheSearchFormPostsAndAnICInTheURLSearchesNothing(t *testing.T) {
	t.Parallel()
	_, ada, _, certID := passedLiveCourse(t)
	res, body := ada.get("/certificates")
	expectStatus(t, res, body, http.StatusOK, `<form class="verify-card" method="post" action="/certificates">`)
	res, body = ada.get("/certificates?q=951206085555")
	expectStatus(t, res, body, http.StatusOK, "Check a Certificate")
	expectNone(t, body, certID, "951206-08-****")
}

func TestAFullICFindsTheCertificateWithTheICMasked(t *testing.T) {
	t.Parallel()
	_, ada, _, certID := passedLiveCourse(t)
	for _, q := range []string{"951206-08-5555", "951206085555", " 951206 08 5555 "} {
		res, body := search(ada, q)
		expectStatus(t, res, body, http.StatusOK, "951206-08-****")
		expectAll(t, body, "Ada", "Go for Acme", "PCC Level: Foundation", `alt="PCC Foundation badge"`, "Certificate No. PCC-", "Issued ", "Status: Valid", `href="/certificates/`+certID+`"`)
		expectNone(t, content(body), "5555")
	}
}

// A Certificate No. search shows no IC, since the numbers run in order.
func TestACertificateNoFindsThatCertificateAndAPartialICFindsNothing(t *testing.T) {
	t.Parallel()
	_, ada, _, certID := passedLiveCourse(t)
	_, body := ada.get("/certificates/" + certID)
	number := regexp.MustCompile(`PCC-LMS-\d{6}`).FindString(body)

	for _, q := range []string{number, strings.ToLower(number)} {
		res, body := search(ada, q)
		expectStatus(t, res, body, http.StatusOK, `href="/certificates/`+certID+`"`)
		expectNone(t, body, "951206", "IC or passport No.")
	}
	for _, q := range []string{"5555", "08-5555", "95120608555", "9512060855555", number[:len(number)-1]} {
		res, body := search(ada, q)
		expectStatus(t, res, body, http.StatusOK, "No Certificate matches.")
		expectNone(t, body, certID)
	}
}

// A printed Certificate from before PCC-LMS carries its number in the old form, which still finds it.
func TestACertificateIsStillFoundByItsNumberAsCertificatesShowedItBeforePCCLMS(t *testing.T) {
	t.Parallel()
	_, ada, _, certID := passedLiveCourse(t)
	_, body := ada.get("/certificates/" + certID)
	expectAll(t, body, "PCC-LMS-260001") // Ada's is the first of 2026
	for _, q := range []string{"PCC-2026-00001", "pcc 2026 00001"} {
		res, body := search(ada, q)
		expectStatus(t, res, body, http.StatusOK, `href="/certificates/`+certID+`"`)
	}
	res, body := search(ada, "PCC-LMS-2600001") // the shown number with a zero too many
	expectStatus(t, res, body, http.StatusOK, "No Certificate matches.")
}

// The search matches the IC the Certificate was issued with, not the Learner's IC now.
func TestSearchMatchesTheICTheCertificateWasIssuedWith(t *testing.T) {
	t.Parallel()
	admin, ada, id, certID := passedLiveCourse(t)
	_, body := admin.get("/admin/courses/" + id)
	correct := correctICLink.FindStringSubmatch(adminCourseRow(t, body, "ada@example.com"))[1]
	admin.post(correct, url.Values{"ic": {"951206-08-5556"}})

	_, body = search(ada, "951206085555")
	expectAll(t, body, `href="/certificates/`+certID+`"`)
	_, body = search(ada, "951206085556")
	expectAll(t, body, "No Certificate matches.")
}

func TestARevokedCertificateShowsAsRevokedInTheResults(t *testing.T) {
	t.Parallel()
	admin, ada, _, certID := passedLiveCourse(t)
	admin.post("/admin/certificates/"+certID+"/revoke", confirming)
	_, body := search(ada, "951206085555")
	expectAll(t, body, `href="/certificates/`+certID+`"`, "Status: Revoked")
}

func TestTooManySearchesFromOneAddressAreRefused(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for range 30 {
		res, body := search(s, "951206085555")
		expectStatus(t, res, body, http.StatusOK, "No Certificate matches.")
	}
	res, body := search(s, "951206085555")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	res, body = searchAsking(s, "PCC-LMS-260001", "ms")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Terlalu banyak cubaan")

	res, body = s.get("/certificates") // the plain check page and ID box are not searches
	expectStatus(t, res, body, http.StatusOK, "Check a Certificate")
}

func TestTheSearchIsInMalay(t *testing.T) {
	t.Parallel()
	_, ada, _, _ := passedLiveCourse(t)
	res, body := searchAsking(ada, "951206085555", "ms")
	expectStatus(t, res, body, http.StatusOK, "No. KP atau pasport 951206-08-****")
	expectAll(t, body, "Atau cari dengan No. KP atau pasport penuh", "Status: Sah", `alt="Lencana PCC Foundation"`)
	_, body = searchAsking(ada, "5555", "ms")
	expectAll(t, body, "Tiada Sijil yang sepadan.")
}
