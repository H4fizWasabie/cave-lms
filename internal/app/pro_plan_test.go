package app_test

import (
	"net/http"
	"net/url"
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

// proPlanSite is bclSite with a 365-day "Pro Year" Plan of 5 Deployments marked as the Lab Plan.
func proPlanSite(t *testing.T) *site {
	t.Helper()
	s := bclSite(t)
	s.createPlan("Pro Year", "365", "5")
	s.markLabPlan("Pro Year")
	return s
}

// expectProFor checks the Learner has Pro until about days from now and returns when it ends.
func expectProFor(t *testing.T, l *site, token string, days int) time.Time {
	t.Helper()
	me := l.me(token)
	if !me.Pro || me.ProUntil == nil {
		t.Fatalf("me = %+v, want Pro", me)
	}
	near(t, "Pro until", *me.ProUntil, time.Now().AddDate(0, 0, days))
	return *me.ProUntil
}

// expectPeriod checks a Plan period of the Lab Plan with this status, starting and ending with Pro (to the minute
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
	until := expectProFor(t, ada, token, 365)
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	expectPeriod(t, periods[0], "Active", periods[0].starts, until)
	near(t, "Plan period start", periods[0].starts, time.Now())

	// Paying while both run queues the next of each after the current ones.
	s.bclPay(paid("LINK-2", "ada@example.com"))
	next := *ada.me(token).ProUntil
	near(t, "Pro until after a second order", next, until.AddDate(0, 0, 365))
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
	until := expectProFor(t, ada, token, 365)
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
	until := expectProFor(t, ada, token, 365)
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

func TestProAndItsPlanPeriodLastTheProPlansDays(t *testing.T) {
	s := bclSite(t)
	s.createPlan("Pro Month", "30", "5")
	s.markLabPlan("Pro Month")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	until := expectProFor(t, ada, token, 30)
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 || periods[0].plan != "Pro Month" {
		t.Fatalf("periods = %+v, want 1 of Pro Month", periods)
	}
	near(t, "Plan period start", periods[0].starts, time.Now())
	near(t, "Plan period end", periods[0].ends, until)
}

func TestTheProPlanPeriodQueuesAfterAHandAssignedPlanPeriod(t *testing.T) {
	s := proPlanSite(t)
	s.createPlan("Lab Monthly", "30", "3")
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	_, body := s.get("/admin/plans")
	monthly := regexp.MustCompile(`<td>Lab Monthly</td>[\s\S]*?href="/admin/plans/(\d+)/edit"`).FindStringSubmatch(body)
	if monthly == nil {
		t.Fatalf("no Lab Monthly edit link: %s", body)
	}
	res, body := s.assignPlan("ada@example.com", monthly[1])
	expectStatus(t, res, body, http.StatusSeeOther, "")
	handEnds := s.periodsOf("ada@example.com")[0].ends

	s.bclPay(paid("LINK-1", "ada@example.com"))
	expectProFor(t, ada, token, 365) // Pro starts at once
	periods := s.periodsOf("ada@example.com")
	if len(periods) != 2 || periods[0].plan != "Lab Monthly" || periods[0].status != "Active" {
		t.Fatalf("periods = %+v, want the hand-assigned one first and active", periods)
	}
	if p := periods[1]; p.plan != "Pro Year" || p.status != "Upcoming" || !p.starts.Equal(handEnds) {
		t.Fatalf("Pro Plan period = %+v, want Pro Year upcoming from %v", p, handEnds)
	}
	near(t, "Pro Plan period end", periods[1].ends, handEnds.AddDate(0, 0, 365))
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 of 3") // the active hand-assigned period's allowance
}

func TestWithNoLabPlanMarkedAPaidOrderStillGivesProAndWarnsInTheSystemLog(t *testing.T) {
	s := bclSite(t)
	s.createPlan("Pro Year", "365", "5") // not marked
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	expectProForAYear(t, ada, token)
	if periods := s.periodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("periods = %+v, want none", periods)
	}
	res, body := s.get("/admin/log?level=error")
	expectStatus(t, res, body, http.StatusOK, "no Plan is marked as the Lab Plan")
}

func TestEditingTheProPlanNeverChangesPeriodsAlreadyGiven(t *testing.T) {
	s := proPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	before := s.periodsOf("ada@example.com")
	_, body := s.get("/admin/plans")
	id := planEditLink.FindStringSubmatch(body)[1]
	res, body := s.post("/admin/plans/"+id, url.Values{"name": {"Pro Year"}, "days": {"30"}, "deployments": {"1"}, "storage": {"1024"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if after := s.periodsOf("ada@example.com"); len(after) != 1 || after[0] != before[0] {
		t.Fatalf("periods after editing the Pro Plan = %+v, want %+v", after, before)
	}
	if got := s.labPlanName(); got != "Pro Year" {
		t.Fatalf("Lab Plan after editing = %q, want Pro Year", got)
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
