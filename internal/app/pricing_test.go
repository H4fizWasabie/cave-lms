package app_test

import (
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
		`src="/static/logo-cavetext-pro.svg`, "CaveText Pro", "Text expansion with no limits", "RM25 a year", `<ul class="offer-benefits">`, "Unlimited Snippets", "Image and Dashboard master prompts",
		`src="/static/logo-lab-plan.svg`, "Lab Plan", "Live Dashboards, hosted for you", "RM89 a year", `<ul class="offer-benefits">`, "Deploy it online at a link you can share",
		`src="/static/logo-pro-plan.svg`, "Pro Plan", "Everything, one payment", "RM99 a year", `<s><span class="sr">Normal price </span>RM119</s>`, `<ul class="offer-benefits">`, "Everything in CaveText Pro", "Everything in the Lab Plan", "Ebooks", "Your own Dashboard deployment", "1 GB of storage", "One payment a year for both")
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
		"Everything in the Lab Plan", `href="https://pay.example.com/pro">Buy Pro Plan</a>`, "processing fee", `<img src="/static/cost-comparison.webp"`)
	expectNone(t, main, "<s>", "Contact us to buy")

	_, body = ada.getAsking("/pricing", "ms")
	expectInOrder(t, content(body), "<h1>Harga</h1>", "Pengembangan teks tanpa had", "RM25 setahun", "Snippet tanpa had", "Beli CaveText Pro",
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
