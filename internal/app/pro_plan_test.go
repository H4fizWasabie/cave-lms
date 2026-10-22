package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// markProPlan marks the Plan with this name as the Pro Plan, with its button on the Admin's Plans page.
func (s *site) markProPlan(name string) {
	s.t.Helper()
	_, body := s.get("/admin/plans")
	m := regexp.MustCompile(`<td>` + regexp.QuoteMeta(name) + `</td>[\s\S]*?action="(/admin/plans/\d+/pro)"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Make Pro Plan button for %s: %s", name, body)
	}
	res, body := s.post(m[1], nil)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// proPlanName reads which Plan the Admin's Plans page marks as the Pro Plan, or "" when none is.
func (s *site) proPlanName() string {
	s.t.Helper()
	_, body := s.get("/admin/plans")
	m := regexp.MustCompile(`<td>([^<]+) <span class="badge[^"]*">Pro Plan</span></td>`).FindAllStringSubmatch(body, -1)
	if len(m) > 1 {
		s.t.Fatalf("%d Plans marked as the Pro Plan: %s", len(m), body)
	}
	if m == nil {
		return ""
	}
	return m[0][1]
}

func TestTheAdminMarksOnePlanAsTheProPlanAndCanChangeIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3", "")
	s.createPlan("Pro Year", "365", "5", "")
	if got := s.proPlanName(); got != "" {
		t.Fatalf("Pro Plan before marking = %q, want none", got)
	}
	s.markProPlan("Pro Year")
	if got := s.proPlanName(); got != "Pro Year" {
		t.Fatalf("Pro Plan = %q, want Pro Year", got)
	}
	s.markProPlan("Lab Monthly")
	if got := s.proPlanName(); got != "Lab Monthly" {
		t.Fatalf("Pro Plan after changing = %q, want Lab Monthly", got)
	}
	_, body := s.get("/admin/plans")
	if strings.Count(body, "/pro\"") != 1 {
		t.Errorf("want one Make Pro Plan button, on the Plan not marked: %s", body)
	}
}

// proPlanSite is bclSite with a 365-day "Pro Year" Plan of 5 Deployments marked as the Pro Plan.
func proPlanSite(t *testing.T) *site {
	t.Helper()
	s := bclSite(t)
	s.createPlan("Pro Year", "365", "5", "")
	s.markProPlan("Pro Year")
	return s
}

// expectPeriod checks a Plan period of the Pro Plan with this status, starting and ending with Pro (to the minute
// the Admin's page shows).
func expectPeriod(t *testing.T, p shownPeriod, status string, starts, ends time.Time) {
	t.Helper()
	if p.plan != "Pro Year" || p.deployments != "5" || p.status != status {
		t.Fatalf("period = %+v, want Pro Year, 5 Deployments, %s", p, status)
	}
	if !p.starts.Equal(starts.UTC().Truncate(time.Minute)) || !p.ends.Equal(ends.UTC().Truncate(time.Minute)) {
		t.Fatalf("period runs %v to %v, want %v to %v", p.starts, p.ends, starts, ends)
	}
}

func TestAPaidOrderGivesAYearOfProAndAMatchingProPlanPeriod(t *testing.T) {
	s := proPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	until := expectProForAYear(t, ada, token)
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectPeriod(t, periods[0], "Active", periods[0].starts, until)
	near(t, "Plan period start", periods[0].starts, time.Now())

	// Paying while both run queues the next of each after the current ones.
	s.bclPay(paid("LINK-2", "ada@example.com"))
	next := *ada.me(token).ProUntil
	near(t, "Pro until after a second order", next, until.AddDate(1, 0, 0))
	periods = s.periodsOf("ada@example.com")
	if len(periods) != 2 {
		t.Fatalf("periods = %+v, want 2", periods)
	}
	expectPeriod(t, periods[1], "Upcoming", until, next)
}

func TestTheAdminsGiveProAlsoGivesTheProPlanPeriod(t *testing.T) {
	s := proPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	until := expectProForAYear(t, ada, token)
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectPeriod(t, periods[0], "Active", periods[0].starts, until)
}

func TestEndingProEarlyLeavesThePlanPeriodRunning(t *testing.T) {
	s := proPlanSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	until := expectProForAYear(t, ada, token)
	s.endPro("ada@example.com")
	if ada.me(token).Pro {
		t.Fatal("Pro still runs after the Admin ended it")
	}
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectPeriod(t, periods[0], "Active", periods[0].starts, until)
}

func TestWithNoProPlanMarkedAPaidOrderStillGivesProAndWarnsInTheSystemLog(t *testing.T) {
	s := bclSite(t)
	s.createPlan("Pro Year", "365", "5", "") // not marked
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	expectProForAYear(t, ada, token)
	if periods := s.periodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("periods = %+v, want none", periods)
	}
	res, body := s.get("/admin/log?level=error")
	expectStatus(t, res, body, http.StatusOK, "no Plan is marked as the Pro Plan")
}

func TestEditingTheProPlanNeverChangesPeriodsAlreadyGiven(t *testing.T) {
	s := proPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	before := s.periodsOf("ada@example.com")
	_, body := s.get("/admin/plans")
	id := planEditLink.FindStringSubmatch(body)[1]
	res, body := s.post("/admin/plans/"+id, url.Values{"name": {"Pro Year"}, "days": {"30"}, "deployments": {"1"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if after := s.periodsOf("ada@example.com"); len(after) != 1 || after[0] != before[0] {
		t.Fatalf("periods after editing the Pro Plan = %+v, want %+v", after, before)
	}
	if got := s.proPlanName(); got != "Pro Year" {
		t.Fatalf("Pro Plan after editing = %q, want Pro Year", got)
	}
}

func TestUnmatchedPaymentsGiveNoProPlanPeriod(t *testing.T) {
	s := proPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	under := paid("LINK-1", "ada@example.com")
	under.subtotal = "10.00"
	other := paid("LINK-2", "ada@example.com")
	other.formURL = "https://shop.bcl.my/form/something-else"
	nobody := paid("LINK-3", "nobody@example.com")
	for _, p := range []bclPayment{under, other, nobody} {
		s.bclPay(p)
	}
	if periods := s.periodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("periods = %+v, want none", periods)
	}
}
