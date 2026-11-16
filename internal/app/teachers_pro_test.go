package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	teachersMonthFormURL = "https://shop.bcl.my/form/teachers-month"
	teachersYearFormURL  = "https://shop.bcl.my/form/teachers-year"
)

func TestTheAdminSetsThePricesPaymentLinksAndPromosOfTeachersPortalProByTheMonthAndYear(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	res, body := s.get("/admin/prices")
	expectStatus(t, res, body, http.StatusOK, "Teachers Portal Pro, a month")
	expectAll(t, body, "Teachers Portal Pro, a year", `name="teachers-month_price" value="`, `name="teachers-year_price" value="`,
		`name="teachers-month_link" value=""`, `name="teachers-year_link" value=""`)

	res, body = s.savePrices(map[string]string{
		"teachers-month_price": "15", "teachers-month_normal": "19", "teachers-month_link": teachersMonthFormURL,
		"teachers-year_price": "150", "teachers-year_link": teachersYearFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/prices")
	expectAll(t, body, `name="teachers-month_price" value="15"`, `name="teachers-month_normal" value="19"`,
		`name="teachers-month_link" value="`+teachersMonthFormURL+`"`, `name="teachers-year_price" value="150"`,
		`name="teachers-year_link" value="`+teachersYearFormURL+`"`)

	// Two products may not share a payment link.
	res, body = s.savePrices(map[string]string{"teachers-month_link": teachersYearFormURL, "teachers-year_link": teachersYearFormURL})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Teachers Portal Pro, a year")

	// A Promo runs on either one, like on the other products.
	for _, product := range []string{"teachers-month", "teachers-year"} {
		res, body = s.post("/admin/promos", url.Values{"code": {"GURU-" + product}, "product": {product}, "price": {"9"},
			"starts_on": {"2020-01-01"}, "ends_on": {"2099-12-31"}})
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	_, body = s.get("/admin/promos")
	expectAll(t, body, "GURU-teachers-month", "GURU-teachers-year", "Teachers Portal Pro, a month")

	// CAVELMS's own Pricing page keeps to the products it always sold.
	_, body = s.get("/pricing")
	expectNone(t, body, "Teachers Portal Pro")
}

// teachersSite is bclSite with payment links for CaveText Pro and both Teachers Portal Pro products, and a Teacher, Ada,
// with a Teacher profile (so her 14-day Teacher trial runs). Her client is returned.
func teachersSite(t *testing.T) (*site, *site) {
	t.Helper()
	s := bclSite(t)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL,
		"teachers-month_link": teachersMonthFormURL, "teachers-year_link": teachersYearFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	res, body = ada.saveTeacherProfile("Sains", "Y4")
	expectRedirect(t, res, body, "/teachers")
	return s, ada
}

// paidTeachers is a paid order on a Teachers Portal Pro form naming email in both email fields.
func paidTeachers(order, email, form, subtotal string) bclPayment {
	p := paid(order, email)
	p.formURL, p.subtotal = form, subtotal
	return p
}

func TestAPaymentOnATeachersPortalProFormGivesAMonthOrAYearAfterTheTrialAndStacks(t *testing.T) {
	t.Parallel()
	s, ada := teachersSite(t)
	trialEnds := time.Now().AddDate(0, 0, 14)

	s.bclPay(paidTeachers("TP-1", "ADA@example.com", teachersMonthFormURL, "15.00"))
	periods := s.teacherPeriodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectTeacherPeriod(t, periods[0], "A month", "Payment", "Upcoming", trialEnds, 28, 31)

	s.bclPay(paidTeachers("TP-2", "ada@example.com", teachersYearFormURL, "150.00"))
	s.bclPay(paidTeachers("TP-3", "ada@example.com", teachersMonthFormURL, "15.00"))
	periods = s.teacherPeriodsOf("ada@example.com")
	if len(periods) != 3 {
		t.Fatalf("periods = %+v, want 3", periods)
	}
	expectTeacherPeriod(t, periods[1], "A year", "Payment", "Upcoming", periods[0].ends, 365, 366)
	expectTeacherPeriod(t, periods[2], "A month", "Payment", "Upcoming", periods[1].ends, 28, 31)

	// The same order sent twice gives once.
	s.bclPay(paidTeachers("TP-3", "ada@example.com", teachersMonthFormURL, "15.00"))
	if got := len(s.teacherPeriodsOf("ada@example.com")); got != 3 {
		t.Fatalf("a repeated order made %d periods, want 3", got)
	}

	// A Payment made after the trial ended starts at once, and one made while a period runs starts when it ends.
	ada.ageTrial(20)
	bob := s.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	s.bclPay(paidTeachers("TP-4", "bob@example.com", teachersYearFormURL, "150.00"))
	expectTeacherPeriod(t, s.teacherPeriodsOf("bob@example.com")[0], "A year", "Payment", "Active", time.Now(), 365, 366)
	s.bclPay(paidTeachers("TP-5", "bob@example.com", teachersMonthFormURL, "15.00"))
	bobs := s.teacherPeriodsOf("bob@example.com")
	expectTeacherPeriod(t, bobs[1], "A month", "Payment", "Upcoming", bobs[0].ends, 28, 31)
}

func TestAPromoPriceIsEnoughForATeachersPortalProPayment(t *testing.T) {
	t.Parallel()
	s, _ := teachersSite(t)
	res, body := s.post("/admin/promos", url.Values{"code": {"GURU9"}, "product": {"teachers-month"}, "price": {"9"},
		"starts_on": {"2020-01-01"}, "ends_on": {"2099-12-31"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.bclPay(paidTeachers("TP-1", "ada@example.com", teachersMonthFormURL, "9.00"))
	if got := len(s.teacherPeriodsOf("ada@example.com")); got != 1 {
		t.Fatalf("a RM9 payment with the Promo made %d periods, want 1", got)
	}
}

func TestTeachersPortalProPaymentsUnderThePriceNamingNoAccountOrOnAnOldFormWaitAsUnmatched(t *testing.T) {
	t.Parallel()
	s, _ := teachersSite(t)
	s.bclPay(paidTeachers("TP-1", "ada@example.com", teachersMonthFormURL, "10.00"))    // under the RM15 price
	s.bclPay(paidTeachers("TP-2", "nobody@example.com", teachersYearFormURL, "150.00")) // no such account
	old := paidTeachers("TP-3", "ada@example.com", "https://shop.bcl.my/form/old-teachers-form", "15.00")
	s.bclPay(old)
	if got := len(s.teacherPeriodsOf("ada@example.com")); got != 0 {
		t.Fatalf("payments that should wait gave %d periods", got)
	}
	res, body := s.get("/admin/cavetext")
	expectStatus(t, res, body, http.StatusOK, "Payments not matched")
	expectAll(t, body, "TP-1", "RM10", "TP-2", "nobody@example.com", "TP-3")
	for order, product := range map[string]string{"TP-1": "Teachers Portal Pro, a month", "TP-2": "Teachers Portal Pro, a year", "TP-3": "-"} {
		if got := unmatchedProduct(t, body, order); got != product {
			t.Errorf("%s is shown as for %q, want %q", order, got, product)
		}
	}
	expectAll(t, body, `href="/admin/teachers-pro?email=nobody%40example.com"`, `href="/admin/teachers-pro?email=ada%40example.com"`)
	// The Admin sorts one out by giving the month by hand, then marks it done.
	s.giveTeacherPro("ada@example.com", "month")
	res, body = s.post("/admin/cavetext/payments/done", url.Values{"order": {"TP-1"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/cavetext")
	expectNone(t, body, "TP-1")
}

func TestTeachersPortalProGivesNeitherCaveTextProNorTheLabAndTheyDoNotGiveIt(t *testing.T) {
	t.Parallel()
	s, _ := teachersSite(t)
	s.createPlan("Lab Month", "30", "5")
	s.markLabPlan("Lab Month")
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL, "pro-plan_link": proPlanFormURL,
		"teachers-month_link": teachersMonthFormURL, "teachers-year_link": teachersYearFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.bclPay(paidTeachers("TP-1", "ada@example.com", teachersYearFormURL, "150.00"))
	s.giveTeacherPro("ada@example.com", "month")
	if got := len(s.periodsOf("ada@example.com")); got != 0 {
		t.Fatalf("Teachers Portal Pro gave %d Plan periods", got)
	}
	var accounts int
	if err := s.db.QueryRow(`SELECT count(*) FROM cavetext_accounts`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("Teachers Portal Pro made %d CaveText accounts (%v), want 0", accounts, err)
	}

	// CaveText Pro, the Lab Plan, the Pro Plan and the Admin's gives of them leave the lapsed Teacher with only the pay page.
	cara := s.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", teachersPassword)
	cara.saveTeacherProfile("Sains", "Y4")
	cara.ageTrial(20)
	s.bclPay(paid("CT-1", "cara@example.com"))
	s.bclPay(paidLabPlan("CT-2", "cara@example.com"))
	s.bclPay(paidProPlan("CT-3", "cara@example.com"))
	if got := len(s.teacherPeriodsOf("cara@example.com")); got != 0 {
		t.Fatalf("CaveText Pro, the Lab Plan and the Pro Plan gave %d Teachers Portal Pro periods", got)
	}
	_, body = cara.get("/teachers")
	expectAll(t, body, "Your free trial has ended.", "Teachers Portal Pro, a month")
}

func TestMyMembershipShowsTheTeacherTrialAndTeachersPortalProAndWhenTheyEnd(t *testing.T) {
	t.Parallel()
	s, ada := teachersSite(t)
	plain := s.second()
	plain.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	_, body := plain.get("/my-membership")
	expectNone(t, content(body), "Teacher trial", "Teachers Portal")

	trialEnds := time.Now().AddDate(0, 0, 14).In(malaysiaTime).Format("2 January 2006")
	_, body = ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "Teacher trial", "Until "+trialEnds)
	expectNone(t, content(body), "Teachers Portal Pro")

	// A Payment made during the trial is shown with when it ends, after the trial.
	s.bclPay(paidTeachers("TP-1", "ada@example.com", teachersYearFormURL, "150.00"))
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET ends_at = $1`, lateInTheUTCDay(100)); err != nil {
		t.Fatal(err)
	}
	_, body = ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "Teacher trial", "Until "+trialEnds, "Teachers Portal Pro", "Until "+lateInTheUTCDay(100).In(malaysiaTime).Format("2 January 2006"))
	_, body = ada.getAsking("/my-membership", "ms")
	expectInOrder(t, content(body), "Percubaan percuma", "Hingga", "Teachers Portal Pro", "Hingga")

	// Once the trial is over and Pro is running, only Pro is shown.
	ada.ageTrial(20)
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET starts_at = now() - interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	_, body = ada.get("/my-membership")
	expectInOrder(t, content(body), "Teachers Portal Pro", "Until "+lateInTheUTCDay(100).In(malaysiaTime).Format("2 January 2006"))
	expectNone(t, content(body), "Teacher trial")

	// When both have ended, it says so and leads to the pay page.
	s.agePeriods(200)
	_, body = ada.get("/my-membership")
	expectAll(t, content(body), "Your Teachers Portal Pro has ended.", `href="/teachers"`)
	expectNone(t, content(body), "Teacher trial", "Until")
}

func TestALapsedTeacherSeesOnlyThePayPageWithBothPricesAndPayingBringsThePortalBackAsTheyLeftIt(t *testing.T) {
	t.Parallel()
	s, ada := teachersSite(t)
	res, body := s.savePrices(map[string]string{"teachers-month_price": "15", "teachers-month_normal": "19", "teachers-year_price": "150",
		"cavetext-pro_link": proFormURL, "teachers-month_link": teachersMonthFormURL, "teachers-year_link": teachersYearFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	// While the trial runs the portal is open, and a Payment made now waits for the trial's end.
	_, body = ada.get("/teachers")
	expectAll(t, body, "14 days left in your free trial.", "Sains")
	expectNone(t, body, "Teachers Portal Pro, a month")
	s.bclPay(paidTeachers("TP-1", "ada@example.com", teachersMonthFormURL, "15.00"))
	_, body = ada.get("/teachers")
	expectAll(t, body, "14 days left in your free trial.", "Sains")

	// Once the trial and Pro have both ended, only the pay page is left.
	ada.ageTrial(60)
	s.agePeriods(60)
	res, body = ada.get("/teachers")
	expectStatus(t, res, body, http.StatusOK, "Your Teachers Portal Pro has ended.")
	expectAll(t, body, "Teachers Portal Pro, a month", "RM15", "RM19", "Teachers Portal Pro, a year", "RM150",
		`href="`+teachersMonthFormURL+`"`, `href="`+teachersYearFormURL+`"`, "Buy Teachers Portal Pro, a month")
	expectNone(t, body, "Sains", `href="/teachers/profile"`)
	if n := strings.Count(header(t, body), `href="/"`); n != 1 {
		t.Errorf("the pay page has %d links to CAVELMS, want 1", n)
	}
	_, body = ada.getAsking("/teachers", "ms")
	expectAll(t, body, "Teachers Portal Pro anda telah tamat.", "RM150")

	// A running Promo shows on the pay page.
	res, body = s.post("/admin/promos", url.Values{"code": {"GURU9"}, "product": {"teachers-month"}, "price": {"9"},
		"starts_on": {"2020-01-01"}, "ends_on": {"2099-12-31"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/teachers")
	expectAll(t, body, "RM9 with a promo code")

	// The profile is kept, and not changed from the pay page.
	res, body = ada.get("/teachers/profile")
	expectRedirect(t, res, body, "/teachers")
	res, body = ada.saveTeacherProfile("Bahasa Melayu", "Y6")
	expectRedirect(t, res, body, "/teachers")

	// A Payment brings the portal back as it was left.
	s.bclPay(paidTeachers("TP-2", "ada@example.com", teachersYearFormURL, "150.00"))
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET ends_at = $1 WHERE starts_at <= now()`, lateInTheUTCDay(10)); err != nil {
		t.Fatal(err)
	}
	_, body = ada.get("/teachers")
	expectAll(t, body, "<dd>Sains</dd>", "Teachers Portal Pro until "+lateInTheUTCDay(10).In(malaysiaTime).Format("2 January 2006"))
	expectNone(t, body, "trial has ended", "RM150", "<dd>Bahasa Melayu</dd>")
	res, body = ada.get("/teachers/profile")
	expectStatus(t, res, body, http.StatusOK, `value="Sains"`)
}

// agePeriods moves every Teachers Portal Pro period days into the past.
func (s *site) agePeriods(days int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET starts_at = starts_at - make_interval(days => $1),
		ends_at = ends_at - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

func TestThePublicTeachersPortalPageShowsTheMonthlyAndYearlyPrices(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	res, body := s.savePrices(map[string]string{"teachers-month_price": "15", "teachers-month_normal": "19", "teachers-year_price": "150.50"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for lang, want := range map[string][]string{"en": {"Teachers Portal Pro, a month", "Teachers Portal Pro, a year"}, "ms": {"Teachers Portal Pro, sebulan", "Teachers Portal Pro, setahun"}} {
		_, body = s.second().getAsking("/teachers", lang)
		expectAll(t, content(body), append(want, "RM15", "RM19", "RM150.50")...)
		expectNone(t, body, "Buy Teachers")
	}
}

// teacherPeriod is a Teachers Portal Pro period as the Admin's page shows it.
type teacherPeriod struct {
	length, given, status string
	starts, ends          time.Time
}

var teacherPeriodRow = regexp.MustCompile(`<tr>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>`)

// teacherPeriodsOf reads the Teachers Portal Pro periods the Admin's page lists for the account with this email, in page order.
func (s *site) teacherPeriodsOf(email string) []teacherPeriod {
	s.t.Helper()
	res, body := s.get("/admin/teachers-pro?email=" + url.QueryEscape(email))
	expectStatus(s.t, res, body, http.StatusOK, "Teachers Portal Pro")
	var list []teacherPeriod
	for _, m := range teacherPeriodRow.FindAllStringSubmatch(body, -1) {
		starts, err := time.ParseInLocation("2 January 2006, 15:04", m[2], malaysiaTime)
		if err != nil {
			s.t.Fatal(err)
		}
		ends, err := time.ParseInLocation("2 January 2006, 15:04", m[3], malaysiaTime)
		if err != nil {
			s.t.Fatal(err)
		}
		list = append(list, teacherPeriod{length: m[1], starts: starts, ends: ends, given: m[4], status: m[5]})
	}
	return list
}

// expectTeacherPeriod checks a period of this length ("A month" or "A year") given this way, starting about starts and
// lasting between minDays and maxDays (a month is 28 to 31 days, a year 365 or 366).
func expectTeacherPeriod(t *testing.T, p teacherPeriod, length, given, status string, starts time.Time, minDays, maxDays int) {
	t.Helper()
	if p.length != length || p.given != given || p.status != status {
		t.Fatalf("period = %+v, want %s, %s, %s", p, length, given, status)
	}
	near(t, "Teachers Portal Pro start", p.starts, starts)
	if days := p.ends.Sub(p.starts).Hours() / 24; days < float64(minDays)-0.1 || days > float64(maxDays)+0.1 {
		t.Fatalf("period lasts %.1f days, want %d to %d: %+v", days, minDays, maxDays, p)
	}
}

// giveTeacherPro gives the account with this email a month ("month") or a year ("year") of Teachers Portal Pro by hand,
// through its confirmation page.
func (s *site) giveTeacherPro(email, length string) {
	s.t.Helper()
	_, body := s.get("/admin/teachers-pro?email=" + url.QueryEscape(email))
	m := regexp.MustCompile(`href="(/admin/teachers-pro/\d+/give/` + length + `)"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no give %s link for %s: %s", length, email, body)
	}
	res, body := s.get(m[1])
	expectStatus(s.t, res, body, http.StatusOK, "Teachers Portal Pro")
	res, body = s.post(m[1], confirming)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

func TestTheAdminGivesAMonthOrAYearOfTeachersPortalProByHandAndTheyStackAfterTheTrial(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	ada.saveTeacherProfile("Sains", "Y4") // the trial runs 14 days
	trialEnds := time.Now().AddDate(0, 0, 14)

	res, body := s.get("/admin/teachers-pro?email=ada%40example.com")
	expectStatus(t, res, body, http.StatusOK, "No Teachers Portal Pro yet")
	// Nothing is given by a plain post: the confirmation page comes first.
	res, body = s.post("/admin/teachers-pro/1/give/month", nil)
	expectRedirect(t, res, body, "/admin/teachers-pro/1/give/month")

	s.giveTeacherPro("ada@example.com", "month")
	periods := s.teacherPeriodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectTeacherPeriod(t, periods[0], "A month", "By hand", "Upcoming", trialEnds, 28, 31)

	s.giveTeacherPro("ada@example.com", "year")
	periods = s.teacherPeriodsOf("ada@example.com")
	if len(periods) != 2 {
		t.Fatalf("periods = %+v, want 2", periods)
	}
	expectTeacherPeriod(t, periods[1], "A year", "By hand", "Upcoming", periods[0].ends, 365, 366)

	// An account with no Teacher profile has no trial to wait for, and one with no account is not found.
	bob := s.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	s.giveTeacherPro("bob@example.com", "month")
	expectTeacherPeriod(t, s.teacherPeriodsOf("bob@example.com")[0], "A month", "By hand", "Active", time.Now(), 28, 31)
	res, body = s.get("/admin/teachers-pro?email=nobody%40example.com")
	expectStatus(t, res, body, http.StatusNotFound, "nobody@example.com")

	// Only an Admin.
	res, body = ada.get("/admin/teachers-pro")
	expectStatus(t, res, body, http.StatusForbidden, "")
}

func TestThePortalNavLinksToMyMembershipWhichATeacherWithoutLearnerThingsOpensInThePortal(t *testing.T) {
	t.Parallel()
	s, ada := teachersSite(t)
	for lang, want := range map[string]string{"en": "My membership", "ms": "Keahlian saya"} {
		_, body := ada.getAsking("/teachers", lang)
		expectAll(t, header(t, body), `href="/my-membership"`, want)
	}
	// A Teacher with nothing else sees My membership in the portal, about the portal only.
	res, body := ada.getAsking("/my-membership", "en")
	expectStatus(t, res, body, http.StatusOK, "Teacher trial")
	expectAll(t, header(t, body), `aria-label="Teachers Portal sections"`, `href="/teachers/profile"`)
	expectNone(t, content(body), "No Plan", "See the Plans", "CaveText Pro", "Snippet cap")
	_, body = ada.getAsking("/my-membership", "ms")
	expectAll(t, header(t, body), `aria-label="Bahagian Teachers Portal"`)
	expectAll(t, content(body), "Percubaan percuma")
	expectNone(t, content(body), "Tiada Pelan", "Had Snippet")

	// A Teacher who is also a Learner (here with CaveText Pro running) keeps CAVELMS's page, with its Plan rows.
	if _, err := s.db.Exec(`INSERT INTO cavetext_accounts (learner_id) SELECT id FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at) SELECT id, now() - interval '1 day', now() + interval '30 days' FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	_, body = ada.getAsking("/my-membership", "en")
	expectAll(t, content(body), "No Plan", "CaveText Pro", "Teacher trial")
	expectNone(t, header(t, body), `aria-label="Teachers Portal sections"`)
}

func TestThePayPageSaysTheFreeTrialEndedOnlyWhenNoTeachersPortalProEverRan(t *testing.T) {
	t.Parallel()
	s, ada := teachersSite(t)
	ada.ageTrial(20)
	_, body := ada.getAsking("/teachers", "en")
	expectAll(t, content(body), "Your free trial has ended.")
	_, body = ada.getAsking("/teachers", "ms")
	expectAll(t, content(body), "Percubaan percuma anda telah tamat.")

	// A month of Teachers Portal Pro that has run out is not a free trial.
	s.giveTeacherPro("ada@example.com", "month")
	s.agePeriods(60)
	_, body = ada.getAsking("/teachers", "en")
	expectAll(t, content(body), "Your Teachers Portal Pro has ended.")
	expectNone(t, content(body), "free trial")
	_, body = ada.getAsking("/teachers", "ms")
	expectAll(t, content(body), "Teachers Portal Pro anda telah tamat.")
	expectNone(t, content(body), "Percubaan percuma")
	_, body = ada.getAsking("/my-membership", "en")
	expectAll(t, content(body), "Your Teachers Portal Pro has ended.")
	expectNone(t, content(body), "free trial")
}

func TestATeacherWhoPaidDuringTheTrialIsToldWhenProStartsWhenUploadingBeforeThen(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newDSKPTeacher(s)
	trialEnds := time.Now().AddDate(0, 0, 14).In(malaysiaTime).Format("2 January 2006")
	s.giveTeacherPro("ada@example.com", "month")

	_, body := teacher.getAsking("/teachers/dskps", "en")
	expectAll(t, body, "Uploading a DSKP needs Teachers Portal Pro, which starts on "+trialEnds)
	expectNone(t, body, `name="file"`)
	res, body := teacher.uploadOwnDSKP("Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusForbidden, "Uploading a DSKP needs Teachers Portal Pro, which starts on "+trialEnds)
	s.application.WaitForDSKPReads()
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("an upload before Pro started was read: %d requests", len(got))
	}
	_, body = teacher.getAsking("/teachers/dskps", "ms")
	expectAll(t, body, "memerlukan Teachers Portal Pro, yang bermula pada")
}
