package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ebookSite is labPlanSite with the Pro Plan at RM249, a running CAVE99 Promo at RM99 on it that comes with Ebooks,
// and one ebook everyone may see listed, at the returned address. The Admin is logged in.
func ebookSite(t *testing.T) (*site, string) {
	t.Helper()
	s := labPlanSite(t)
	return s, s.setUpEbooks()
}

// setUpEbooks sets the prices and CAVE99 as ebookSite has them, as the logged-in Admin, and uploads the ebook.
func (s *site) setUpEbooks() string {
	t := s.t
	t.Helper()
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL,
		"pro-plan_link": proPlanFormURL, "pro-plan_price": "249", "pro-plan_normal": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	form := promoForm("CAVE99", "pro-plan", "99", "2026-01-01", "2099-12-31")
	form.Set("ebooks", "on")
	res, body = s.post("/admin/promos", form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return s.uploadAs("Workbook", "everyone", pdfFile)
}

// expectEbookOpens checks l can download the ebook at item.
func expectEbookOpens(t *testing.T, l *site, item string) {
	t.Helper()
	res, body := l.get(item + "/file")
	expectStatus(t, res, body, http.StatusOK, "")
	if !strings.HasPrefix(body, "%PDF") {
		t.Fatalf("the ebook file is not the PDF: %.40q", body)
	}
}

// expectEbookLocked checks l sees the ebook at item listed but locked: its page offers no download, and its file
// sends l back to that page without the PDF.
func expectEbookLocked(t *testing.T, l *site, item string) {
	t.Helper()
	res, body := l.get(item + "/file")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != item {
		t.Errorf("a locked ebook's file redirected to %q, want its page %q", loc, item)
	}
	if strings.Contains(body, "%PDF") {
		t.Fatal("a locked ebook's file gave the PDF")
	}
	res, body = l.get(item)
	expectStatus(t, res, body, http.StatusOK, "Workbook")
	expectAll(t, content(body), "Ebooks come with the COMBO Pro Plan")
	expectNone(t, content(body), item+"/file")
}

func TestAProPlanPaidWithAnEbooksPromoCodeOpensTheEbooks(t *testing.T) {
	s, ebook := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	expectEbookLocked(t, ada, ebook)
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00")) // CAVE99
	expectEbookOpens(t, ada, ebook)
}

// Only the Pro Plan bought with the code of a Promo that comes with Ebooks opens them: not the Pro Plan at full price
// or with another Promo's code, and not CaveText Pro, even at the same RM99 and with the same code typed.
func TestOtherPaymentsLeaveTheEbooksLocked(t *testing.T) {
	s, ebook := ebookSite(t)
	s.addPromo("CAVE89", "pro-plan", "89", "2026-01-01", "2099-12-31") // no Ebooks
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	full := paidProPlan("LINK-1", "ada@example.com")
	full.subtotal = "249.00"
	s.bclPay(full)
	bea, _ := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	other := couponProPlan("LINK-2", "bea@example.com", "249.00", "160.00")
	other.extra["coupon_code"] = "CAVE89"
	s.bclPay(other)
	cy, cyToken := s.newCaveTextLearner("Cy Tan", "cy@example.com")
	text := paid("LINK-3", "cy@example.com") // CaveText Pro's form
	text.subtotal = "99.00"
	text.extra = map[string]any{"coupon_code": "CAVE99"}
	s.bclPay(text)
	expectProForAYear(t, cy, cyToken)
	for _, l := range []*site{ada, bea, cy} {
		expectEbookLocked(t, l, ebook)
	}
}

// A Visitor sees the ebook listed but locked, pointed to the COMBO Pro Plan while its Promo runs; Promotional
// material stays open to all; the Admin opens every ebook.
func TestVisitorsSeeEbooksLockedButPromotionalOnesOpen(t *testing.T) {
	s, ebook := ebookSite(t)
	fields := titleFields("Free sampler", "everyone")
	fields["promotional"] = "on"
	res, body := s.upload(fields, pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	sampler := res.Header.Get("Location")
	expectEbookOpens(t, s, ebook)

	visitor := s.second()
	visitor.logOut()
	expectEbookLocked(t, visitor, ebook)
	_, body = visitor.get(ebook)
	expectAll(t, content(body), `href="/pricing">Get the COMBO Pro Plan`)
	_, body = visitor.get("/gallery")
	expectAll(t, content(body), "Workbook")
	expectEbookOpens(t, visitor, sampler)

	s.endPromo("CAVE99")
	_, body = visitor.get(ebook)
	expectAll(t, content(body), "Ebooks come with the COMBO Pro Plan")
	expectNone(t, content(body), "Get the COMBO Pro Plan")
}

// A Learner without Ebook access who joined the Course still opens a Lesson whose Gallery item is an ebook: its
// text, and the ebook locked in place of its download.
func TestALessonShowsItsEbookLockedToACourseMemberWithoutEbookAccess(t *testing.T) {
	s, ebook := ebookSite(t)
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	res, body := s.post("/admin/courses/"+id+"/lessons", url.Values{"title": {"Notes lesson"},
		"gallery_item_id": {strings.TrimPrefix(ebook, "/gallery/")}, "body": {"Read chapter one."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	lessonPath := "/courses/" + id + "/lessons/" + s.lessonIDByTitle("Notes lesson")

	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.post("/courses/"+id+"/join", nil)
	res, body = ada.get(lessonPath)
	expectStatus(t, res, body, http.StatusOK, "Read chapter one.")
	expectAll(t, content(body), "Workbook", "Ebooks come with the COMBO Pro Plan")
	expectNone(t, content(body), ebook+"/file")
	expectEbookLocked(t, ada, ebook)

	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	_, body = ada.get(lessonPath)
	expectAll(t, content(body), ebook+"/file")
	expectNone(t, content(body), "Ebooks come with the COMBO Pro Plan")
}

// Ebook access lasts the year of CaveText Pro that came with it: ending that Pro locks the ebooks again, and Pro
// given again without Ebooks does not reopen them.
func TestEndingTheComboYearLocksTheEbooksAgain(t *testing.T) {
	s, ebook := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	expectEbookOpens(t, ada, ebook)
	s.endPro("ada@example.com")
	expectEbookLocked(t, ada, ebook)
	s.givePro("ada@example.com")
	expectEbookLocked(t, ada, ebook)
}

// The Admin gives the COMBO Pro Plan by hand, as for a COMBO payment BCL could not match: the Pro Plan with Ebooks.
func TestTheAdminGivesTheComboProPlanByHand(t *testing.T) {
	s, ebook := ebookSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	res, body := s.post(s.proAction("ada@example.com", "combo"), nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	expectProPlan(t, s, ada, token, "ada@example.com", time.Time{}, nil)
	expectEbookOpens(t, ada, ebook)
}

// Live, BCL's order detail decides, its coupon too. While no live detail has shown whether BCL names the coupon there,
// a detail without one takes the webhook's code; a detail naming another code wins over the webhook's.
func TestALiveOrderTakesBCLsCouponCodeOrElseTheWebhooks(t *testing.T) {
	combo := func(email, coupon string) bclOrder {
		o := confirmed(email)
		o.subtotal, o.discount, o.amount, o.coupon, o.formID = "249", "150", "100", coupon, proPlanFormID
		return o
	}
	bcl := newFakeBCL(t, map[string]bclOrder{
		"LINK-1": combo("ada@example.com", "cave99"), // BCL names CAVE99; the webhook names none
		"LINK-2": combo("bea@example.com", ""),       // BCL names none; the webhook names CAVE99
		"LINK-3": combo("cy@example.com", "CAVE89")}) // BCL names another code than the webhook's CAVE99
	s := confirmingSite(t, bcl)
	ebook := s.setUpEbooks()
	s.addPromo("CAVE89", "pro-plan", "89", "2026-01-01", "2099-12-31")
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, _ := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	cy, _ := s.newCaveTextLearner("Cy Tan", "cy@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	s.bclPay(couponProPlan("LINK-2", "bea@example.com", "249.00", "150.00"))
	s.bclPay(couponProPlan("LINK-3", "cy@example.com", "249.00", "150.00"))
	expectEbookOpens(t, ada, ebook)
	expectEbookOpens(t, bea, ebook)
	expectEbookLocked(t, cy, ebook)
}

// The Admin marks a Pro Plan Promo as coming with Ebooks on the Promos page, which lists it so; no other product's.
func TestOnlyAProPlanPromoComesWithEbooks(t *testing.T) {
	s, _ := ebookSite(t)
	_, body := s.get("/admin/promos")
	expectAll(t, body, "CAVE99", "With Ebooks")
	form := promoForm("LAB10", "lab-plan", "10", "2026-01-01", "2099-12-31")
	form.Set("ebooks", "on")
	res, body := s.post("/admin/promos", form)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Only a Pro Plan Promo can come with Ebooks.")
	expectAll(t, body, `name="ebooks" value="on" checked`)
}
