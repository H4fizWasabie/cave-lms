package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestThePlansPageIsInTheShopMenuBeforeCaveText(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Shop<", `<a href="/plans">Plans</a>`, `href="/cavetext"`, ">Program<")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, header(t, body), ">Kedai<", `<a href="/plans">Pelan</a>`, `href="/cavetext"`)
}

func TestThePlansPageShowsTheThreeProductsWithTheirPricesButNotPlansAssignedByHand(t *testing.T) {
	s, _ := adminWithPlanAndLearner(t)

	// A Visitor sees the three products, what each includes and its price, and only the Pro Plan's Normal price.
	res, body := s.second().getAsking("/plans", "en")
	expectStatus(t, res, body, http.StatusOK, "<h1>Plans</h1>")
	main := content(body)
	expectInOrder(t, main, "CaveText Pro", "RM25 a year", "unlimited Snippets",
		"Lab Plan", "RM89 a year", "build, upload and deploy",
		"Pro Plan", "RM99 a year", `<s><span class="sr">Normal price </span>RM119</s>`, "CaveText Pro and the Lab Plan")
	if n := strings.Count(main, "<s>"); n != 1 {
		t.Errorf("%d crossed-out prices, want 1", n)
	}
	expectNone(t, main, "Lab Monthly")

	// With links, a Learner's Buy opens each product's form; with no Normal price none is crossed out.
	s.savePrices(map[string]string{"cavetext-pro_link": "https://pay.example.com/cavetext", "lab-plan_link": "https://pay.example.com/lab",
		"pro-plan_normal": "", "pro-plan_link": "https://pay.example.com/pro"})
	ada := s.second()
	ada.login("ada@example.com", "correct horse battery")
	_, body = ada.getAsking("/plans", "en")
	main = content(body)
	expectAll(t, main, `href="https://pay.example.com/cavetext">Buy CaveText Pro</a>`,
		`href="https://pay.example.com/lab">Buy Lab Plan</a>`, `href="https://pay.example.com/pro">Buy Pro Plan</a>`)
	expectNone(t, main, "<s>", "Contact us to buy")

	_, body = ada.getAsking("/plans", "ms")
	expectInOrder(t, content(body), "<h1>Pelan</h1>", "RM25 setahun", "Beli CaveText Pro")
}
