package app_test

import (
	"net/http"
	"testing"
	"time"
)

// addHiddenPromo adds a Hidden Promo on the Pro Plan that comes with Ebooks, as the logged-in Admin.
func (s *site) addHiddenPromo(code, price string) {
	s.t.Helper()
	form := promoForm(code, "pro-plan", price, "2026-01-01", "2099-12-31")
	form.Set("ebooks", "on")
	form.Set("hidden", "on")
	res, body := s.post("/admin/promos", form)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// A Hidden Promo below the public Promo leaves every price a visitor sees alone, yet still accepts its lower payment
// and opens the Ebooks.
func TestAHiddenPromoIsPaidAndGivesEbooksButNeverChangesThePricesShown(t *testing.T) {
	s, ebook := ebookSite(t) // CAVE99 at RM99 with Ebooks, public
	s.addHiddenPromo("TEST10", "9")
	_, body := s.get("/admin/promos")
	expectAll(t, body, "TEST10", "Hidden")

	_, body = s.second().getAsking("/pricing", "en")
	expectAll(t, content(body), "RM99 a year", "RM99 with a promo code")
	expectNone(t, content(body), "RM9 ")

	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	pay := couponProPlan("LINK-1", "ada@example.com", "249.00", "240.00")
	pay.extra["coupon_code"] = "TEST10"
	s.bclPay(pay) // RM9, under the public RM99
	expectProPlan(t, s, ada, token, "ada@example.com", time.Time{}, nil)
	expectEbookOpens(t, ada, ebook, "inline")
}

// With only a Hidden Promo running, nothing advertises a Promo: no COMBO card, no pointer from a locked ebook.
func TestAHiddenPromoAloneDoesNotMakeTheComboAppear(t *testing.T) {
	s := labPlanSite(t)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "249", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ebook := s.uploadAs("Workbook", "everyone", pdfFile)
	s.addHiddenPromo("TEST10", "9")

	visitor := s.second()
	visitor.logOut()
	_, body = visitor.getAsking("/pricing", "en")
	expectNone(t, content(body), "COMBO Pro Plan", "with a promo code", "RM9 ")
	_, body = visitor.get(ebook)
	expectAll(t, content(body), "Ebooks come with the COMBO Pro Plan")
	expectNone(t, content(body), "Get the COMBO Pro Plan")
}
