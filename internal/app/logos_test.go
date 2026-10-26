package app_test

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// expectImage finds the <img> with alt in body and fetches it: it must be served as a WebP or PNG.
func (s *site) expectImage(body, alt string) {
	s.t.Helper()
	m := regexp.MustCompile(`<img src="(/static/[^"]+)" alt="` + regexp.QuoteMeta(alt) + `"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Errorf("page has no image %q", alt)
		return
	}
	res, img := s.get(m[1])
	webp := len(img) > 12 && img[:4] == "RIFF" && img[8:12] == "WEBP"
	if res.StatusCode != http.StatusOK || !webp && !bytes.HasPrefix([]byte(img), []byte("\x89PNG")) {
		s.t.Errorf("GET %s = %d, want the %q WebP or PNG", m[1], res.StatusCode, alt)
	}
}

func TestTheHeaderFooterAdminBarAndFaviconShowTheCaveConsultLogoInBothLanguages(t *testing.T) {
	s := newSite(t)
	for lang, alt := range map[string]string{"en": "The Cave Consult logo", "ms": "Logo The Cave Consult"} {
		_, body := s.getAsking("/", lang)
		head, page, _ := strings.Cut(body, `<main id="main">`)
		s.expectImage(head, alt)
		s.expectImage(page, alt)
		expectNone(t, body, "cavelms.svg", "logo-light.svg")
		m := regexp.MustCompile(`<link rel="icon" href="(/static/[^"]+)">`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("page has no favicon")
		}
		if res, _ := s.get(m[1]); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want the favicon", m[1], res.StatusCode)
		}
	}

	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	_, body := s.getAsking("/admin", "ms")
	s.expectImage(body, "Logo The Cave Consult")
	expectNone(t, body, "logo-light.svg")
}

func TestCoachAdamsCardShowsTheIssuerLogosAndTheHRDCorpBadgeInBothLanguages(t *testing.T) {
	s := newSite(t)
	for lang, alts := range map[string][]string{
		"en": {"HRD Corp Registered Training Provider badge", "UMPSA logo", "ICTC logo", "HRD Corp logo", "JPK logo", "Exemplar Global logo"},
		"ms": {"Lencana Penyedia Latihan Berdaftar HRD Corp", "Logo UMPSA", "Logo ICTC", "Logo HRD Corp", "Logo JPK", "Logo Exemplar Global"},
	} {
		_, body := s.getAsking("/", lang)
		_, coach, _ := strings.Cut(body, `id="coach"`)
		coach, _, _ = strings.Cut(coach, `</section>`)
		for _, alt := range alts {
			s.expectImage(coach, alt)
		}
		expectNone(t, body, "Claimable", "CLAIMABLE")
	}
}

func TestTheAboutPageShowsTheKPMCrestInBothLanguages(t *testing.T) {
	s := newSite(t)
	for lang, alt := range map[string]string{"en": "Crest of the Ministry of Education Malaysia (KPM)", "ms": "Jata Kementerian Pendidikan Malaysia (KPM)"} {
		_, body := s.getAsking("/about", lang)
		s.expectImage(content(body), alt)
	}
}
