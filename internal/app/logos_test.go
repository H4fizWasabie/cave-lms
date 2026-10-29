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

func TestTheHeaderFooterAndAdminBarShowTheCAVELMSLogoAndTheFaviconStaysTheCaveConsultsInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	brand := map[string]string{"en": "CAVELMS, your digital learning hub", "ms": "CAVELMS, hab pembelajaran digital anda"}
	for lang, alt := range brand {
		_, body := s.getAsking("/", lang)
		head, page, _ := strings.Cut(body, `<main id="main">`)
		s.expectImage(head, alt)
		expectAll(t, head, `<a class="brand" href="/"><img src="/static/cavelms-logo.webp`)
		_, footer, _ := strings.Cut(page, `<footer`)
		s.expectImage(footer, alt)
		expectAll(t, footer, `<img src="/static/cavelms-logo-light.webp`)
		// The Cave Consult logo is left only in the trust strip's TCC cell.
		_, trust, _ := strings.Cut(page, `class="trust"`)
		trust, rest, _ := strings.Cut(trust, `</section>`)
		expectAll(t, trust, "cave-consult-logo.webp")
		expectNone(t, head+rest, "cave-consult-logo", "cavelms.svg", "logo-light.svg")
		m := regexp.MustCompile(`<link rel="icon" href="(/static/[^"]+)">`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("page has no favicon")
		}
		if res, _ := s.get(m[1]); res.StatusCode != http.StatusOK || !strings.HasPrefix(m[1], "/static/favicon.png") {
			t.Errorf("GET %s = %d, want The Cave Consult favicon", m[1], res.StatusCode)
		}
	}

	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	_, body := s.getAsking("/admin", "ms")
	s.expectImage(body, brand["ms"])
	expectAll(t, body, `<a class="adm-brand" href="/admin"><img src="/static/cavelms-logo-light.webp`)
	expectNone(t, body, "logo-light.svg")
}

func TestTheTrustStripShowsALogoBesideEachHeadingInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for lang, alts := range map[string][]string{
		"en": {"The Cave Consult logo", "TCC", "HRD Corp logo", "HRD Corp", "ePerolehan logo", "MOF", "JPK logo", "JPK"},
		"ms": {"Logo The Cave Consult", "TCC", "Logo HRD Corp", "HRD Corp", "Logo ePerolehan", "MOF", "Logo JPK", "JPK"},
	} {
		_, body := s.getAsking("/", lang)
		_, trust, _ := strings.Cut(body, `class="trust"`)
		trust, _, _ = strings.Cut(trust, `</section>`)
		for i := 0; i < len(alts); i += 2 {
			s.expectImage(trust, alts[i])
		}
		expectInOrder(t, trust, alts...)
	}
}

func TestCoachAdamsCardShowsHisPhotoThenTheIssuerLogosWithTheHRDCorpBadgeInOneRowInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for lang, alts := range map[string][]string{
		"en": {"Coach Adam Daniel speaking at a podium", "UMPSA logo", "ICTC logo", "HRD Corp logo", "HRD Corp Registered Training Provider badge", "JPK logo", "Exemplar Global logo"},
		"ms": {"Coach Adam Daniel berucap di podium", "Logo UMPSA", "Logo ICTC", "Logo HRD Corp", "Lencana Penyedia Latihan Berdaftar HRD Corp", "Logo JPK", "Logo Exemplar Global"},
	} {
		_, body := s.getAsking("/", lang)
		_, coach, _ := strings.Cut(body, `id="coach"`)
		coach, _, _ = strings.Cut(coach, `</section>`)
		s.expectImage(coach, alts[0])
		_, issuers, _ := strings.Cut(coach, `class="coach-issuers"`)
		issuers, _, _ = strings.Cut(issuers, `</ul>`)
		for _, alt := range alts[1:] {
			s.expectImage(issuers, alt)
		}
		expectInOrder(t, coach, alts[0], `class="coach-creds"`, `class="coach-issuers"`)
		expectInOrder(t, issuers, alts[1:]...)
		expectNone(t, body, "Claimable", "CLAIMABLE", "coach-accredited")
	}
}

func TestTheAboutPageShowsTheCAVELMSLogoInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for lang, alt := range map[string]string{"en": "CAVELMS, your digital learning hub", "ms": "CAVELMS, hab pembelajaran digital anda"} {
		_, body := s.getAsking("/about", lang)
		page := mainOf(body)
		s.expectImage(page, alt)
		expectAll(t, page, "cavelms-logo.webp")
		expectNone(t, page, "kpm-crest", "KPM")
	}
}
