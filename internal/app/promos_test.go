package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// promoForm is the Promos page's form for code on product at price, running from starts to ends.
func promoForm(code, product, price, starts, ends string) url.Values {
	return url.Values{"code": {code}, "product": {product}, "price": {price}, "starts_on": {starts}, "ends_on": {ends}}
}

// addPromo adds a Promo as the logged-in Admin.
func (s *site) addPromo(code, product, price, starts, ends string) {
	s.t.Helper()
	res, body := s.post("/admin/promos", promoForm(code, product, price, starts, ends))
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// endPromo ends the Promo with code early, as the logged-in Admin.
func (s *site) endPromo(code string) {
	s.t.Helper()
	res, body := s.post(s.endPromoLink(code), confirming)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// endPromoLink is the address of the End link of the Promo with code, as the logged-in Admin sees it.
func (s *site) endPromoLink(code string) string {
	s.t.Helper()
	_, body := s.get("/admin/promos")
	m := regexp.MustCompile(`href="(/admin/promos/\d+/end)" aria-label="End ` + code + `"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no End link for %s:\n%s", code, body)
	}
	return m[1]
}

func TestAnyAdminAddsListsAndEndsPromosAndPastOnesStayListed(t *testing.T) {
	s := caveTextAdminSite(t)
	res, body := s.get("/admin/promos")
	expectStatus(t, res, body, http.StatusOK, "<h1>Admin: Promos</h1>")
	expectAll(t, body, `href="/admin/promos" aria-current="page"`, "No Promos yet.")

	s.addPromo("CAVE99", "pro-plan", "99", "2026-01-01", "2099-12-31")
	s.addPromo("OLD50", "lab-plan", "50", "2025-01-01", "2025-01-31")
	s.addPromo("NEXT", "cavetext-pro", "20", "2099-01-01", "2099-01-31")
	_, body = s.get("/admin/promos")
	expectInOrder(t, content(body), "NEXT", "CaveText Pro", "RM20", "Not started",
		"CAVE99", "Pro Plan", "RM99", "Running", "OLD50", "Lab Plan", "RM50", "Over")
	expectNone(t, body, `aria-label="End OLD50"`) // an Over Promo has nothing to end

	confirmPage := s.endPromoLink("CAVE99")
	res, body = s.post(confirmPage, nil)
	expectRedirect(t, res, body, confirmPage)
	_, body = s.get("/admin/promos")
	expectInOrder(t, content(body), "CAVE99", "Running")
	res, body = s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "End the Promo CAVE99 now? Its price stops at once. This cannot be undone.")
	expectAll(t, body, `href="/admin/promos"`)

	s.endPromo("CAVE99")
	_, body = s.get("/admin/promos")
	expectInOrder(t, content(body), "CAVE99", "Ended early", "OLD50", "Over")
	expectNone(t, body, `aria-label="End CAVE99"`)
}

func TestThePromosPageRefusesMistakes(t *testing.T) {
	s := caveTextAdminSite(t)
	for _, c := range []struct {
		form url.Values
		want string
	}{
		{promoForm("", "pro-plan", "99", "2026-01-01", "2026-12-31"), "Enter the code"},
		{promoForm("CAVE 99", "pro-plan", "99", "2026-01-01", "2026-12-31"), "Enter the code"},
		{promoForm("CAVE99", "gold-plan", "99", "2026-01-01", "2026-12-31"), "Choose a product"},
		{promoForm("CAVE99", "pro-plan", "abc", "2026-01-01", "2026-12-31"), "Enter the promo price in RM"},
		{promoForm("CAVE99", "pro-plan", "0", "2026-01-01", "2026-12-31"), "Enter the promo price in RM"},
		{promoForm("CAVE99", "pro-plan", "99", "", "2026-12-31"), "Enter the start and end dates"},
		{promoForm("CAVE99", "pro-plan", "99", "2026-12-31", "2026-01-01"), "The end date cannot be before the start date"},
	} {
		res, body := s.post("/admin/promos", c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, `name="price" value="`+c.form.Get("price")+`"`) // kept to correct
	}
	_, body := s.get("/admin/promos")
	expectAll(t, body, "No Promos yet.")
}

func TestOnlyAnAdminManagesPromos(t *testing.T) {
	s := caveTextAdminSite(t)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := ada.get("/admin/promos")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	res, body = ada.post("/admin/promos", promoForm("CAVE99", "pro-plan", "1", "2026-01-01", "2099-12-31"))
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	s.addPromo("CAVE99", "pro-plan", "99", "2026-01-01", "2099-12-31")
	_, body = s.get("/admin/promos")
	id := regexp.MustCompile(`/admin/promos/(\d+)/end`).FindStringSubmatch(body)[1]
	res, body = ada.post("/admin/promos/"+id+"/end", confirming)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
}

func TestThePromosPageIsInMalay(t *testing.T) {
	s := caveTextAdminSite(t)
	s.switchLanguage("ms", "/")
	s.addPromo("CAVE99", "pro-plan", "99", "2026-01-01", "2099-12-31")
	_, body := s.get("/admin/promos")
	expectLanguage(t, body, "ms", "Pentadbir: Promosi")
	expectAll(t, body, "Pelan Pro", "Sedang berjalan")
}

// promoSite is labPlanSite with the Pro Plan at RM249 and a running CAVE99 Promo at RM99 on it.
func promoSite(t *testing.T) *site {
	t.Helper()
	s := labPlanSite(t)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "249", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.addPromo("CAVE99", "pro-plan", "99", "2026-01-01", "2099-12-31")
	return s
}

func TestAPaymentAtTheRunningPromoPriceGivesTheProduct(t *testing.T) {
	s := promoSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com")) // RM99, under the RM249 price
	expectProPlan(t, s, ada, token, "ada@example.com", time.Time{}, nil)
	_, body := s.get("/admin/log")
	expectAll(t, body, "LINK-1")
}

func TestAPromoPaymentAfterThePromoEndsWaitsAsUnmatched(t *testing.T) {
	s := promoSite(t)
	s.endPromo("CAVE99")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	if ada.me(token).Pro {
		t.Fatal("a RM99 Pro Plan Payment after CAVE99 ended gave CaveText Pro")
	}
	_, body := s.get("/admin/cavetext")
	if got := unmatchedProduct(t, body, "LINK-1"); got != "Pro Plan" {
		t.Errorf("LINK-1 is shown as for %q, want Pro Plan", got)
	}
}

func TestWithTwoPromosRunningTheLowestPriceCountsAndOnlyOnItsOwnProduct(t *testing.T) {
	s := promoSite(t)
	s.addPromo("CAVE89", "pro-plan", "89", "2026-01-01", "2099-12-31")
	s.addPromo("LAB10", "lab-plan", "10", "2026-01-01", "2099-12-31")
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	at89 := paidProPlan("LINK-1", "ada@example.com")
	at89.subtotal = "89.00"
	s.bclPay(at89)
	expectProPlan(t, s, ada, adaToken, "ada@example.com", time.Time{}, nil)

	// RM10 is the Lab Plan's Promo, not the Pro Plan's.
	bob, bobToken := s.newCaveTextLearner("Bob", "bob@example.com")
	at10 := paidProPlan("LINK-2", "bob@example.com")
	at10.subtotal = "10.00"
	s.bclPay(at10)
	if bob.me(bobToken).Pro {
		t.Fatal("a RM10 Pro Plan Payment gave CaveText Pro")
	}
}

func TestAPromoThatHasNotStartedOrIsOverDoesNotCount(t *testing.T) {
	s := labPlanSite(t)
	s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "249", "pro-plan_normal": ""})
	s.addPromo("SOON", "pro-plan", "99", "2099-01-01", "2099-12-31")
	s.addPromo("PAST", "pro-plan", "99", "2025-01-01", "2025-12-31")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	if ada.me(token).Pro {
		t.Fatal("a RM99 Pro Plan Payment counted with no Promo running")
	}
}

func TestOffersShowTheRunningPromoPriceButNeverItsCode(t *testing.T) {
	s := promoSite(t)
	visitor := s.second()
	_, body := visitor.getAsking("/pricing", "en")
	main := content(body)
	expectInOrder(t, main, "Pro Plan", "RM249 a year", "RM99 with a promo code")
	expectNone(t, main, "CAVE99")
	if n := strings.Count(main, "with a promo code"); n != 1 {
		t.Errorf("%d promo lines, want 1 (only the Pro Plan has a Promo)", n)
	}
	_, body = visitor.getAsking("/pricing", "ms")
	expectAll(t, content(body), "RM99 dengan kod promosi")

	s.endPromo("CAVE99")
	_, body = visitor.getAsking("/pricing", "en")
	expectNone(t, content(body), "with a promo code")
}

// couponProPlan is a paid Pro Plan order as BCL sends one with a coupon: the subtotal before it, the discount apart.
func couponProPlan(order, email, subtotal, discount string) bclPayment {
	p := paidProPlan(order, email)
	p.subtotal = subtotal
	p.extra = map[string]any{"discount_amount": discount, "coupon_code": "CAVE99"}
	return p
}

func TestACouponPaymentCountsWhatWasPaidAfterTheDiscount(t *testing.T) {
	s := promoSite(t) // Pro Plan RM249, CAVE99 at RM99 running
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	expectProPlan(t, s, ada, token, "ada@example.com", time.Time{}, nil)

	// A deeper discount than any running Promo allows waits for an Admin, showing what was paid.
	bob, bobToken := s.newCaveTextLearner("Bob", "bob@example.com")
	s.bclPay(couponProPlan("LINK-2", "bob@example.com", "249.00", "200.00"))
	if bob.me(bobToken).Pro {
		t.Fatal("a RM49 Pro Plan Payment after a RM200 discount gave CaveText Pro")
	}
	_, body := s.get("/admin/cavetext")
	if got := unmatchedProduct(t, body, "LINK-2"); got != "Pro Plan" {
		t.Errorf("LINK-2 is shown as for %q, want Pro Plan", got)
	}
	expectAll(t, body, "RM49")
}

func TestACouponPaymentWithNoPromoRunningWaitsAsUnmatched(t *testing.T) {
	s := promoSite(t)
	s.endPromo("CAVE99")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	if ada.me(token).Pro {
		t.Fatal("a coupon payment of RM99 with no Promo running gave CaveText Pro")
	}
}

func TestAnUnreadableDiscountWaitsForAnAdmin(t *testing.T) {
	s := promoSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "lots"))
	if ada.me(token).Pro {
		t.Fatal("an unreadable discount gave CaveText Pro")
	}
	_, body := s.get("/admin/log")
	expectAll(t, body, "LINK-1", "unreadable")
}
