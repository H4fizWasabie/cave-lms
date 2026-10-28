package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestThePrivacyNoticeIsInBothLanguagesAndTheEnglishPrevails(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, c := range []struct {
		lang string
		want []string
		gone []string
	}{
		{"en", []string{"Privacy Notice", "PDPA", "English version prevails", "The Cave Consult", "Wasabie Tech",
			"private cloud server", "encrypted", "may leave Malaysia", "18 or older", "21 days", "6 months", "30 days", "adam.caveconsult@gmail.com"},
			[]string{"France", "United States", "Contabo", "Cloudflare", "Resend"}},
		{"ms", []string{"Notis Privasi", "PDPA", "versi bahasa Inggeris", "The Cave Consult", "Wasabie Tech",
			"pelayan awan persendirian", "tersulit", "mungkin keluar dari Malaysia", "18 tahun", "21 hari", "6 bulan", "30 hari", "adam.caveconsult@gmail.com"},
			[]string{"Perancis", "Amerika Syarikat", "Contabo", "Cloudflare", "Resend"}},
	} {
		res, body := s.getAsking("/privacy", c.lang)
		expectStatus(t, res, body, http.StatusOK, "")
		expectLanguage(t, body, c.lang, c.want[0])
		for _, want := range c.want {
			if !strings.Contains(body, want) {
				t.Errorf("the %s notice lacks %q", c.lang, want)
			}
		}
		for _, gone := range c.gone {
			if strings.Contains(body, gone) {
				t.Errorf("the %s notice names %q", c.lang, gone)
			}
		}
	}
}

func TestEveryPagesFooterLinksToThePrivacyNotice(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, path := range []string{"/", "/courses", "/login", "/gallery", "/privacy"} {
		_, body := s.get(path)
		_, footer, ok := strings.Cut(body, `<footer`)
		if !ok || !strings.Contains(footer, `<a href="/privacy">Privacy Notice</a>`) {
			t.Errorf("the footer of %s does not link to the Privacy Notice", path)
		}
	}
}

func TestRegisterAsksForConsentToThePrivacyNoticeInThePagesLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/register", "en")
	expectLanguage(t, body, "en", `By registering you confirm you are 18 or older and agree to the <a href="/privacy">Privacy Notice</a>.`)
	_, body = s.getAsking("/register", "ms")
	expectLanguage(t, body, "ms", `Dengan mendaftar, anda mengesahkan bahawa anda berumur 18 tahun ke atas dan bersetuju dengan <a href="/privacy">Notis Privasi</a>.`)
}
