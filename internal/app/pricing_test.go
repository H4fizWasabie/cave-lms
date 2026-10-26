package app_test

import (
	"html"
	"net/http"
	"strings"
	"testing"
)

func TestThePricingMenuLinksPlansToThePricingPageBeforeCaveText(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Pricing<", `<a href="/pricing">Plans</a>`, `href="/cavetext"`, ">Program<")
	expectNone(t, header(t, body), ">Shop<")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, header(t, body), ">Harga<", `<a href="/pricing">Pelan</a>`, `href="/cavetext"`)
	expectNone(t, header(t, body), ">Kedai<")
}

func TestTheOldPlansAddressMovesToPricingKeepingItsQuery(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/plans?utm_source=fb&x=1")
	expectStatus(t, res, body, http.StatusMovedPermanently, "")
	if loc := res.Header.Get("Location"); loc != "/pricing?utm_source=fb&x=1" {
		t.Fatalf("/plans redirected to %q, want /pricing?utm_source=fb&x=1", loc)
	}
	res, body = s.get("/plans")
	expectStatus(t, res, body, http.StatusMovedPermanently, "")
	if loc := res.Header.Get("Location"); loc != "/pricing" {
		t.Fatalf("/plans redirected to %q, want /pricing", loc)
	}
}

func TestThePricingPageShowsTheThreeProductsWithTheirPricesButNotPlansAssignedByHand(t *testing.T) {
	s, _ := adminWithPlanAndLearner(t)

	// A Visitor sees each card top to bottom: logo, name, tagline, price, benefits; only the Pro Plan has a Normal price.
	res, body := s.second().getAsking("/pricing", "en")
	expectStatus(t, res, body, http.StatusOK, "<h1>Pricing</h1>")
	main := content(body)
	expectInOrder(t, main,
		`src="/static/logo-cavetext-pro.svg`, "CaveText Pro", "Text expansion for up to 2,000 Snippets", "RM25 a year", `<ul class="offer-benefits">`, "Up to 2,000 Snippets", "Image and Dashboard master prompts",
		`src="/static/logo-lab-plan.svg`, "Lab Plan", "Live Dashboards, hosted for you", "RM89 a year", `<ul class="offer-benefits">`, "Deploy it online at a link you can share",
		`src="/static/logo-pro-plan.svg`, "Pro Plan", "Everything, one payment", "RM99 a year", `<s><span class="sr">Normal price </span>RM119</s>`, `<ul class="offer-benefits">`, "Everything in CaveText Pro", "Everything in the Lab Plan", "Your own Dashboard deployment", "0.5 GB of storage", "One payment a year for both")
	// Ebooks are the COMBO Pro Plan's, a card shown only while a Promo runs.
	cards, _, _ := strings.Cut(main, `id="comparison"`)
	expectNone(t, cards, "Ebooks", "1 GB of storage", "COMBO Pro Plan")
	if n := strings.Count(main, "<s>"); n != 1 {
		t.Errorf("%d crossed-out prices, want 1", n)
	}
	expectNone(t, main, "Lab Monthly", "Buy CaveText Pro", "monthly")
	for _, logo := range []string{"cavetext-pro", "lab-plan", "pro-plan"} {
		res, img := s.get("/static/logo-" + logo + ".svg")
		expectStatus(t, res, img, http.StatusOK, "<svg")
	}

	// With links, a Learner's Buy, after the benefits, opens each product's form; with no Normal price none is crossed out.
	s.savePrices(map[string]string{"cavetext-pro_link": "https://pay.example.com/cavetext", "lab-plan_link": "https://pay.example.com/lab",
		"pro-plan_normal": "", "pro-plan_link": "https://pay.example.com/pro"})
	ada := s.second()
	ada.login("ada@example.com", "correct horse battery")
	_, body = ada.getAsking("/pricing", "en")
	main = content(body)
	expectInOrder(t, main, "Image and Dashboard master prompts", `href="https://pay.example.com/cavetext">Buy CaveText Pro</a>`,
		"Deploy it online at a link you can share", `href="https://pay.example.com/lab">Buy Lab Plan</a>`,
		"Everything in the Lab Plan", `href="https://pay.example.com/pro">Buy Pro Plan</a>`, "processing fee", `id="comparison"`)
	expectNone(t, main, "<s>", "Contact us to buy")

	_, body = ada.getAsking("/pricing", "ms")
	expectInOrder(t, content(body), "<h1>Harga</h1>", "Pengembangan teks untuk sehingga 2,000 Snippet", "RM25 setahun", "Sehingga 2,000 Snippet", "Beli CaveText Pro",
		"Dashboard langsung", "Semua sekali, satu bayaran", "Semua dalam CaveText Pro")
}

func TestThePricingPageOffersToRenewCaveTextProWhileItRuns(t *testing.T) {
	s := caveTextAdminSite(t)
	s.savePrices(map[string]string{"cavetext-pro_link": "https://pay.example.com/cavetext", "lab-plan_link": "https://pay.example.com/lab",
		"pro-plan_link": "https://pay.example.com/pro"})
	ada, _ := s.newCaveTextLearner("Ada", "ada@example.com")
	s.givePro("ada@example.com")
	_, body := ada.getAsking("/pricing", "en")
	expectAll(t, content(body), `href="https://pay.example.com/cavetext">Renew CaveText Pro</a>`, ">Buy Lab Plan</a>", ">Buy Pro Plan</a>")
}

// The comparison transcribes the client's picture (static/cost-comparison.webp): its Malay word for word, its English a translation.
func TestThePricingPageComparesYearlyCostsInThePicturesOwnWords(t *testing.T) {
	s := newSite(t)
	for lang, want := range map[string][]string{
		"ms": {"Perbandingan Kos Tahunan", "<table",
			"Fungsi / Komponen", "CAVELMS", "RM99/tahun", "TextExpander Pro", "RM258/tahun", "Hostinger Premium", "RM119.88/tahun",
			"Cloudflare Paid", "RM258/tahun", "Render Starter", "RM361.2/tahun", "Netlify Personal", "RM464.4/tahun",
			"Vercel Pro", "RM1,000/tahun", "Supabase Pro", "RM1,300/tahun",
			"Text Expansion", "AI Prompt Templates", "Subdomain peribadi", "(domain promosi)",
			"Web hosting / deployment", "SSL / HTTPS", "Static to dynamic dashboard (tanpa coding)", "Upload Excel sebagai data",
			"Storan pengguna", "0.5 GB", "~10 GB", "Bergantung pelan", "Penyelenggaraan aplikasi",
			"Anggaran Add-on (jika perlu)", "Domain sendiri (.com)", "Tidak perlu", "(subdomain)", "~RM50/tahun",
			"Storan tambahan", "Termasuk", "~RM80/tahun", "(20 GB)", "Bergantung penggunaan", "~RM91/tahun", "(100 GB)",
			"Back-up tambahan", "Termasuk", "~RM40/tahun", "Bergantung penggunaan",
			"Sokongan teknikal", "Termasuk", "Add-on berbayar",
			"Jumlah Kos Tahunan", "(termasuk fungsi asas + anggaran add-on)",
			"RM99", "RM258", "RM269.88", "RM258+", "RM361.2+", "RM464.4+", "RM1,000+", "RM1,300+", "</table>"},
		"en": {"Annual Cost Comparison", "<table",
			"Feature / Component", "CAVELMS", "RM99/year", "TextExpander Pro", "RM258/year", "Render Starter", "RM361.2/year", "Supabase Pro", "RM1,300/year",
			"Text Expansion", "AI Prompt Templates", "Personal subdomain", "(promotional domain)",
			"Web hosting / deployment", "Static to dynamic dashboard (no coding)", "Upload Excel as data",
			"User storage", "0.5 GB", "Depends on plan", "App maintenance",
			"Add-on estimate (if needed)", "Own domain (.com)", "Not needed", "~RM50/year",
			"Extra storage", "Included", "~RM80/year", "Depends on usage", "Extra backups", "~RM40/year", "Technical support", "Paid add-on",
			"Total Annual Cost", "(includes basic features + add-on estimate)",
			"RM99", "RM269.88", "RM361.2+", "RM464.4+", "RM1,300+", "</table>"},
	} {
		_, body := s.getAsking("/pricing", lang)
		expectInOrder(t, html.UnescapeString(mainOf(body)), want...)
		expectNone(t, body, "cost-comparison.webp", "Prompt Library") // the client dropped the KPM Prompt Library row
	}
}
