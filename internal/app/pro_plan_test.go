package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
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

// labPlanSite is bclSite with a 30-day "Lab Month" Plan of 5 Deployments marked as the Lab Plan.
func labPlanSite(t *testing.T) *site {
	t.Helper()
	s := bclSite(t)
	s.createPlan("Lab Month", "30", "5")
	s.markLabPlan("Lab Month")
	return s
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
