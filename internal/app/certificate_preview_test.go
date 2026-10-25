package app_test

import (
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
