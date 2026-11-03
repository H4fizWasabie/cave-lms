package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestTheRefundPolicyIsPublicAndInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, c := range []struct {
		lang string
		want []string
	}{
		{"en", []string{"Refund Policy", "Last updated 3 November 2026",
			"All purchases on CAVELMS are final and non-refundable, including for a change of mind or unused access.",
			"We will fix the problem or refund you if you were charged in error, charged twice, or did not receive what you paid for and we can&#39;t resolve it.",
			"Email us within 14 days of purchase:", `<a href="mailto:adam.caveconsult@gmail.com">adam.caveconsult@gmail.com</a>`,
			"Nothing here limits any rights you have under Malaysian consumer law."}},
		{"ms", []string{"Polisi Bayaran Balik", "Kemas kini terakhir 3 November 2026",
			"Semua pembelian di CAVELMS adalah muktamad dan tidak boleh dibayar balik, termasuk kerana perubahan fikiran atau akses yang tidak digunakan.",
			"Kami akan membetulkan masalah itu atau membayar balik wang anda jika anda dicaj secara silap, dicaj dua kali, atau tidak menerima apa yang anda bayar dan kami tidak dapat menyelesaikannya.",
			"E-mel kami dalam tempoh 14 hari dari tarikh pembelian:", `<a href="mailto:adam.caveconsult@gmail.com">adam.caveconsult@gmail.com</a>`,
			"Tiada apa-apa di sini mengehadkan hak anda di bawah undang-undang pengguna Malaysia."}},
	} {
		res, body := s.getAsking("/refunds", c.lang)
		expectStatus(t, res, body, http.StatusOK, "")
		expectLanguage(t, body, c.lang, c.want[0])
		for _, want := range c.want {
			if !strings.Contains(body, want) {
				t.Errorf("the %s Refund Policy lacks %q", c.lang, want)
			}
		}
	}
}

// The notice stands once under a set of Buy buttons, before payment; Contact us and a Visitor's buttonless cards get none.
func TestBuyButtonsCarryTheRefundNoticeOnceAndOnlyWithAPaymentLink(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	notice := map[string]string{
		"en": `All purchases are non-refundable. See the <a href="/refunds">Refund Policy</a>.`,
		"ms": `Semua pembelian tidak boleh dibayar balik. Lihat <a href="/refunds">Polisi Bayaran Balik</a>.`,
	}
	count := func(who *site, path, lang string) int {
		_, body := who.getAsking(path, lang)
		return strings.Count(body, notice[lang])
	}

	for _, path := range []string{"/pricing", "/cavetext"} {
		for lang := range notice {
			if n := count(ada, path, lang); n != 0 {
				t.Errorf("%s in %s shows the notice %d times with no payment link, want 0", path, lang, n)
			}
		}
	}

	s.savePrices(map[string]string{"cavetext-pro_link": "https://pay.example.com/cavetext", "pro-plan_link": "https://pay.example.com/pro"})
	for _, path := range []string{"/pricing", "/cavetext"} {
		for lang := range notice {
			if n := count(ada, path, lang); n != 1 {
				t.Errorf("%s in %s shows the notice %d times beside the Buy buttons, want 1", path, lang, n)
			}
			if n := count(s.second(), path, lang); n != 0 {
				t.Errorf("%s in %s shows a Visitor, who has no Buy buttons, the notice %d times, want 0", path, lang, n)
			}
		}
	}
}

func TestEveryPagesFooterLinksToTheRefundPolicy(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, path := range []string{"/", "/courses", "/login", "/gallery", "/refunds"} {
		_, body := s.get(path)
		_, footer, ok := strings.Cut(body, `<footer`)
		if !ok || !strings.Contains(footer, `<a href="/refunds">Refund Policy</a>`) {
			t.Errorf("the footer of %s does not link to the Refund Policy", path)
		}
	}
}
