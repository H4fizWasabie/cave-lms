package app_test

import (
	"net/http"
	"strings"
	"testing"
)

// expectNone fails the test for each unwanted string the page shows.
func expectNone(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("page shows %q", u)
		}
	}
}

func TestTheCaveTextPageOffersCaveTextProAndTheProPlanWithTheirPrices(t *testing.T) {
	s := caveTextAdminSite(t)
	cy := s.second()
	cy.loginAsNewLearner("Cy Tan", "cy@example.com", "correct horse battery")

	// No links yet: contact us for each; only the Pro Plan has a Normal price.
	res, body := cy.get("/cavetext")
	expectStatus(t, res, body, http.StatusOK, "RM25 a year")
	expectAll(t, body, "RM99 a year", `<s><span class="sr">Normal price </span>RM119</s>`,
		`href="mailto:adam.caveconsult@gmail.com?subject=CaveText%20Pro">Contact us to buy CaveText Pro</a>`,
		`href="mailto:adam.caveconsult@gmail.com?subject=Pro%20Plan">Contact us to buy Pro Plan</a>`)
	expectNone(t, body, "RM89", "Buy Lab Plan", "Contact us to buy Lab Plan", "processing fee")
	if n := strings.Count(body, "<s>"); n != 1 {
		t.Errorf("%d crossed-out prices, want 1", n)
	}

	// Links and a CaveText Pro promotion: Buy opens each product's form; the Pro Plan's promotion ended.
	s.savePrices(map[string]string{"cavetext-pro_normal": "35", "cavetext-pro_link": "https://bcl.my/pay/cavetext",
		"pro-plan_normal": "", "pro-plan_link": "https://bcl.my/pay/pro-plan"})
	_, body = cy.get("/cavetext")
	expectAll(t, body, `<s><span class="sr">Normal price </span>RM35</s>`, "processing fee",
		`href="https://bcl.my/pay/cavetext">Buy CaveText Pro</a>`, `href="https://bcl.my/pay/pro-plan">Buy Pro Plan</a>`)
	expectNone(t, body, "RM119", "Contact us to buy")

	// A Visitor sees the prices but is asked to log in rather than buy.
	_, body = s.second().get("/cavetext")
	expectAll(t, body, "RM25 a year", "RM99 a year")
	expectNone(t, body, "Buy CaveText Pro", "Buy Pro Plan")
}
