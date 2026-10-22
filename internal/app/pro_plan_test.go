package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// markLabPlan marks the Plan with this name as the Lab Plan, with its button on the Admin's Plans page.
func (s *site) markLabPlan(name string) {
	s.t.Helper()
	_, body := s.get("/admin/plans")
	m := regexp.MustCompile(`<td>` + regexp.QuoteMeta(name) + `</td>[\s\S]*?action="(/admin/plans/\d+/lab)"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Make Lab Plan button for %s: %s", name, body)
	}
	res, body := s.post(m[1], nil)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// labPlanName reads which Plan the Admin's Plans page marks as the Lab Plan, or "" when none is.
func (s *site) labPlanName() string {
	s.t.Helper()
	_, body := s.get("/admin/plans")
	m := regexp.MustCompile(`<td>([^<]+) <span class="badge[^"]*">Lab Plan</span></td>`).FindAllStringSubmatch(body, -1)
	if len(m) > 1 {
		s.t.Fatalf("%d Plans marked as the Lab Plan: %s", len(m), body)
	}
	if m == nil {
		return ""
	}
	return m[0][1]
}

func TestTheAdminMarksOnePlanAsTheLabPlanAndCanChangeIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3")
	s.createPlan("Pro Year", "365", "5")
	if got := s.labPlanName(); got != "" {
		t.Fatalf("Lab Plan before marking = %q, want none", got)
	}
	s.markLabPlan("Pro Year")
	if got := s.labPlanName(); got != "Pro Year" {
		t.Fatalf("Lab Plan = %q, want Pro Year", got)
	}
	s.markLabPlan("Lab Monthly")
	if got := s.labPlanName(); got != "Lab Monthly" {
		t.Fatalf("Lab Plan after changing = %q, want Lab Monthly", got)
	}
	_, body := s.get("/admin/plans")
	if len(regexp.MustCompile(`action="/admin/plans/\d+/lab"`).FindAllString(body, -1)) != 1 || strings.Contains(body, "Make Pro Plan") {
		t.Errorf("want one Make Lab Plan button, on the Plan not marked: %s", body)
	}
}

const (
	labFormURL     = "https://shop.bcl.my/form/lab-plan"
	proPlanFormURL = "https://shop.bcl.my/form/pro-plan"
)

// labPlanSite is bclSite with a 30-day "Lab Month" Plan of 5 Deployments marked as the Lab Plan, and payment links
// for all three products.
func labPlanSite(t *testing.T) *site {
	t.Helper()
	s := bclSite(t)
	res, body := s.savePrices(map[string]string{"cavetext-pro_link": proFormURL, "lab-plan_link": labFormURL, "pro-plan_link": proPlanFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.createPlan("Lab Month", "30", "5")
	s.markLabPlan("Lab Month")
	return s
}

// paidLabPlan is a paid RM89 order on the Lab Plan form naming email in both email fields.
func paidLabPlan(order, email string) bclPayment {
	p := paid(order, email)
	p.formURL, p.subtotal = labFormURL, "89.00"
	return p
}

// expectLabMonth checks a period of the Lab Plan with this status, lasting its 30 days from starts.
func expectLabMonth(t *testing.T, p shownPeriod, status string, starts time.Time) {
	t.Helper()
	if p.plan != "Lab Month" || p.deployments != "5" || p.status != status {
		t.Fatalf("period = %+v, want Lab Month, 5 Deployments, %s", p, status)
	}
	near(t, "Plan period start", p.starts, starts)
	near(t, "Plan period end", p.ends, starts.AddDate(0, 0, 30))
}

func TestALabPlanPaymentGivesALabPlanPeriodAtOnceAndNoCaveTextPro(t *testing.T) {
	s := labPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidLabPlan("LINK-1", "ada@example.com"))
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectLabMonth(t, periods[0], "Active", time.Now())
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 of 5")
	if ada.me(token).Pro {
		t.Fatal("a Lab Plan Payment gave CaveText Pro")
	}

	// Paying while the period runs queues the next one after it.
	s.bclPay(paidLabPlan("LINK-2", "ada@example.com"))
	periods = s.periodsOf("ada@example.com")
	if len(periods) != 2 {
		t.Fatalf("periods = %+v, want 2", periods)
	}
	expectLabMonth(t, periods[1], "Upcoming", periods[0].ends)
}

func TestALabPlanPeriodQueuesAfterAHandAssignedPlanPeriod(t *testing.T) {
	s := labPlanSite(t)
	s.createPlan("Lab Week", "7", "3")
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	_, body := s.get("/admin/plans")
	week := regexp.MustCompile(`<td>Lab Week</td>[\s\S]*?href="/admin/plans/(\d+)/edit"`).FindStringSubmatch(body)
	if week == nil {
		t.Fatalf("no Lab Week edit link: %s", body)
	}
	res, body := s.assignPlan("ada@example.com", week[1])
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.bclPay(paidLabPlan("LINK-1", "ada@example.com"))
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 2 || periods[0].plan != "Lab Week" || periods[0].status != "Active" {
		t.Fatalf("periods = %+v, want the hand-assigned one first and active", periods)
	}
	expectLabMonth(t, periods[1], "Upcoming", periods[0].ends)
}

func TestLabPlanPaymentsUnderThePriceNamingNoLearnerOrWithNoLabPlanMarkedWaitAsUnmatched(t *testing.T) {
	s := labPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	under := paidLabPlan("LINK-1", "ada@example.com")
	under.subtotal = "25.00" // the CaveText Pro price, not the Lab Plan's
	s.bclPay(under)
	s.bclPay(paidLabPlan("LINK-2", "nobody@example.com"))
	if _, err := s.db.Exec(`UPDATE plans SET lab = false`); err != nil { // no Admin action unmarks it
		t.Fatal(err)
	}
	s.bclPay(paidLabPlan("LINK-3", "ada@example.com"))
	if periods := s.periodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("periods = %+v, want none", periods)
	}
	if ada.me(token).Pro {
		t.Fatal("an unmatched Lab Plan Payment gave CaveText Pro")
	}
	_, body := s.get("/admin/cavetext")
	for _, order := range []string{"LINK-1", "LINK-2", "LINK-3"} {
		if got := unmatchedProduct(t, body, order); got != "Lab Plan" {
			t.Errorf("%s is shown as for %q, want Lab Plan", order, got)
		}
	}
}

// expectOnlyTheLabDemo checks the Learner has no Plan period and sees only the Lab's demo.
func expectOnlyTheLabDemo(t *testing.T, s, l *site, email string) {
	t.Helper()
	if periods := s.periodsOf(email); len(periods) != 0 {
		t.Fatalf("periods = %+v, want none", periods)
	}
	res, body := l.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Building, uploading and deploying need a Plan")
}

func TestACaveTextProPaymentGivesAYearOfCaveTextProAndNoPlanPeriod(t *testing.T) {
	s := labPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	until := expectProForAYear(t, ada, token)
	expectOnlyTheLabDemo(t, s, ada, "ada@example.com")

	// Paying while CaveText Pro runs queues the next year after it.
	s.bclPay(paid("LINK-2", "ada@example.com"))
	near(t, "Pro until after a second order", *ada.me(token).ProUntil, until.AddDate(1, 0, 0))
	expectOnlyTheLabDemo(t, s, ada, "ada@example.com")
}

func TestTheAdminsGiveProGivesCaveTextProOnly(t *testing.T) {
	s := labPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	expectProForAYear(t, ada, token)
	expectOnlyTheLabDemo(t, s, ada, "ada@example.com")
}

func TestEndingProEarlyLeavesAPlanPeriodRunning(t *testing.T) {
	s := labPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	res, body := s.assignPlan("ada@example.com", s.onlyPlanID())
	expectStatus(t, res, body, http.StatusSeeOther, "")
	before := s.periodsOf("ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	s.endPro("ada@example.com")
	if ada.me(token).Pro {
		t.Fatal("Pro still runs after the Admin ended it")
	}
	if after := s.periodsOf("ada@example.com"); len(after) != 1 || after[0] != before[0] {
		t.Fatalf("periods after ending Pro = %+v, want %+v", after, before)
	}
}
