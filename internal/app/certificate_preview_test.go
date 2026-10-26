package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// certArt is the first Certificate preview on the page body, or fails the test.
func certArt(t *testing.T, body string) string {
	t.Helper()
	_, art, ok := strings.Cut(body, `<svg class="cert-art"`)
	art, _, closed := strings.Cut(art, "</svg>")
	if !ok || !closed {
		t.Fatalf("page shows no Certificate preview: %s", body)
	}
	return art
}

func TestTheCertificatePreviewShowsTheLearnersNameOrAPlaceholderAndNeverAnIC(t *testing.T) {
	s := newSite(t)
	courseID, _ := s.adaEarns() // Ada's IC is ADAEXAMPLECOM
	for _, path := range []string{"/", "/courses/" + courseID, "/my-certificates"} {
		_, body := s.get(path)
		art := certArt(t, body)
		expectAll(t, art, "CERTIFICATE", "OF COMPETENCY", "THIS IS TO CERTIFY THAT", ">Ada<", "demonstrated throughout")
		expectNone(t, art, "ADAEXAMPLECOM", "Your name here")
		if path != "/" {
			expectAll(t, art, ">Go Basics<")
		}
	}

	s.logOut()
	for _, path := range []string{"/", "/courses/" + courseID} {
		_, body := s.get(path)
		expectAll(t, certArt(t, body), "Your name here")
		_, body = s.getAsking(path, "ms")
		expectAll(t, certArt(t, body), "Nama anda di sini", "CERTIFICATE") // the artwork itself stays English
	}
}

func TestOnceABackgroundIsUploadedThePreviewShowsItUnderTheDetailsInsteadOfTheBuiltInArtwork(t *testing.T) {
	s := newSite(t)
	courseID, _ := s.adaEarns()
	admin := s.second()
	admin.loginAdmin()
	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})

	background := regexp.MustCompile(`<image href="(/certificate-background/[^"]+)" x="0" y="0" width="210" height="297"`)
	var src string
	for _, path := range []string{"/", "/courses/" + courseID, "/my-certificates"} {
		_, body := s.get(path)
		art := certArt(t, body)
		m := background.FindStringSubmatch(art)
		if m == nil {
			t.Fatalf("%s: preview does not draw the uploaded background: %s", path, art)
		}
		src = m[1]
		expectAll(t, art, ">Ada<")
		expectNone(t, art, "THIS IS TO CERTIFY THAT", "demonstrated throughout", "cert-seal", "cert-signature")
	}

	s.logOut() // a Visitor sees the home page's preview, so the background is theirs to load
	res, body := s.get(src)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || body != string(pngOfSize(t, 3, 5)) {
		t.Fatalf("GET %s = %d %q, want the uploaded PNG", src, res.StatusCode, res.Header.Get("Content-Type"))
	}

	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 7, 9)})
	if res, _ := s.get(src); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET %s after the background was replaced = %d, want 404", src, res.StatusCode)
	}
}

func TestEveryCertificatePreviewOnMyCertificatesHasItsOwnIDs(t *testing.T) {
	s := newSite(t)
	s.adaEarns() // "Go Basics"
	s.logOut()
	s.loginAdmin()
	s.createCourse("Go Testing", "Learn it.", "")
	second := s.courseIDByTitle("Go Testing")
	s.addTextLesson(second, "Tables", "", "Body.", false)
	s.logOut()
	s.login("ada@example.com", certPassword)
	s.post("/courses/"+second+"/join", nil)
	s.markLesson(second, s.lessonIDByTitle("Tables"), true)

	_, body := s.get("/my-certificates")
	if n := strings.Count(body, `<svg class="cert-art"`); n != 2 {
		t.Fatalf("My certificates shows %d previews, want 2: %s", n, body)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		if seen[m[1]] {
			t.Errorf("id %q appears more than once on My certificates", m[1])
		}
		seen[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`url\(#([^)]+)\)`).FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			t.Errorf("url(#%s) points at no id on the page", m[1])
		}
	}
}
