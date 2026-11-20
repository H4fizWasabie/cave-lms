package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// assignCaveHost posts the Admin's CaveHost Plan form: a period of this many days for the Learner with this email.
func (s *site) assignCaveHost(email string, days int) (*http.Response, string) {
	return s.post("/admin/plans/periods/cavehost", url.Values{"email": {email}, "days": {strconv.Itoa(days)}})
}

var (
	caveHostRow       = regexp.MustCompile(`<tr class="cavehost-period">\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>`)
	caveHostEndButton = regexp.MustCompile(`href="(/admin/plans/periods/cavehost/\d+/end)"`)
)

// caveHostPeriodsOf reads the CaveHost Plan periods the Admin's page shows for the Learner with this email, in page order.
func (s *site) caveHostPeriodsOf(email string) []shownPeriod {
	s.t.Helper()
	res, body := s.get("/admin/plans/periods?email=" + url.QueryEscape(email))
	expectStatus(s.t, res, body, http.StatusOK, "CaveHost Plan periods")
	var list []shownPeriod
	for _, m := range caveHostRow.FindAllStringSubmatch(body, -1) {
		starts, err := time.ParseInLocation("2 January 2006, 15:04", m[1], malaysiaTime)
		if err != nil {
			s.t.Fatal(err)
		}
		ends, err := time.ParseInLocation("2 January 2006, 15:04", m[2], malaysiaTime)
		if err != nil {
			s.t.Fatal(err)
		}
		list = append(list, shownPeriod{starts: starts, ends: ends, status: m[3]})
	}
	return list
}

// ageAllCaveHostPeriods moves every CaveHost Plan period into the past.
func (s *site) ageAllCaveHostPeriods(days int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE cavehost_plan_periods SET starts_at = starts_at - make_interval(days => $1), ends_at = ends_at - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

// caveHostSite logs in the Admin and Ada, a Learner with no Plan, and returns the Admin's client and Ada's.
func caveHostSite(t *testing.T) (admin, ada *site) {
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	return admin, ada
}

// hasCaveHostLink reports whether the header of a page of the site links to CaveHost.
func hasCaveHostLink(s *site) bool {
	s.t.Helper()
	_, body := s.get("/my-profile")
	return strings.Contains(body, `<a class="hdr-link" href="/cavehost">CaveHost</a>`)
}

func TestACaveHostPlanPeriodStartsNowAndOpensCaveHostForItsLearner(t *testing.T) {
	t.Parallel()
	admin, ada := caveHostSite(t)
	if hasCaveHostLink(ada) {
		t.Fatal("a Learner with no CaveHost Plan sees the CaveHost link")
	}

	res, body := admin.assignCaveHost("ADA@example.com", 30)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/plans/periods?email=ADA%40example.com" {
		t.Fatalf("assign redirected to %q", loc)
	}
	periods := admin.caveHostPeriodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("CaveHost periods = %+v, want 1", periods)
	}
	p := periods[0]
	if since := time.Since(p.starts); since < -time.Minute || since > 2*time.Minute {
		t.Fatalf("period starts %v, want now", p.starts)
	}
	if want := p.starts.AddDate(0, 0, 30); !p.ends.Equal(want) || p.status != "Active" {
		t.Fatalf("period = %+v, want 30 days, Active", p)
	}

	if !hasCaveHostLink(ada) {
		t.Fatal("a Learner with a CaveHost Plan does not see the CaveHost link")
	}
	res, body = ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusOK, "<h1>CaveHost</h1>")
}

func TestCaveHostPlanPeriodsStackAndShowBesidePlanPeriods(t *testing.T) {
	t.Parallel()
	admin, _ := caveHostSite(t)
	admin.createPlan("Lab Monthly", "30", "3")
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	admin.assignCaveHost("ada@example.com", 30)
	admin.assignCaveHost("ada@example.com", 10)

	p := admin.caveHostPeriodsOf("ada@example.com")
	if len(p) != 2 || !p[1].starts.Equal(p[0].ends) || !p[1].ends.Equal(p[1].starts.AddDate(0, 0, 10)) {
		t.Fatalf("CaveHost periods = %+v, want the second to start when the first ends, 10 days long", p)
	}
	if p[0].status != "Active" || p[1].status != "Upcoming" {
		t.Fatalf("CaveHost periods = %+v, want Active then Upcoming", p)
	}
	if lab := admin.periodsOf("ada@example.com"); len(lab) != 1 {
		t.Fatalf("Plan periods = %+v, want the Lab Plan period kept apart from CaveHost's", lab)
	}
}

func TestAdminEndsACaveHostPlanPeriodEarly(t *testing.T) {
	t.Parallel()
	admin, ada := caveHostSite(t)
	admin.assignCaveHost("ada@example.com", 30)
	_, body := admin.get("/admin/plans/periods?email=ada@example.com")
	m := caveHostEndButton.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no end button on the CaveHost Plan period: %s", body)
	}

	res, body := admin.post(m[1], nil)
	expectRedirect(t, res, body, m[1])
	if !hasCaveHostLink(ada) {
		t.Fatal("an unconfirmed end took CaveHost away")
	}
	res, body = admin.get(m[1])
	expectStatus(t, res, body, http.StatusOK, "End the CaveHost Plan period for ada@example.com now?")
	res, body = admin.post(m[1], confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	if p := admin.caveHostPeriodsOf("ada@example.com"); len(p) != 1 || !strings.HasPrefix(p[0].status, "Ended early on ") {
		t.Fatalf("CaveHost periods = %+v, want one Ended early", p)
	}
	if hasCaveHostLink(ada) {
		t.Fatal("a Learner whose CaveHost Plan period ended early still sees the CaveHost link")
	}
	_, body = admin.get("/admin/plans/periods?email=ada@example.com")
	if caveHostEndButton.MatchString(body) {
		t.Fatalf("an ended period still has an end button: %s", body)
	}
}

func TestACaveHostPlanIsRefusedForAnUnknownEmailOrBadLength(t *testing.T) {
	t.Parallel()
	admin, _ := caveHostSite(t)
	res, body := admin.assignCaveHost("nobody@example.com", 30)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "No Learner has the email nobody@example.com")
	for _, days := range []string{"0", "-3", "thirty", ""} {
		res, body = admin.post("/admin/plans/periods/cavehost", url.Values{"email": {"ada@example.com"}, "days": {days}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "whole number of days, 1 to 3650")
	}
	if p := admin.caveHostPeriodsOf("ada@example.com"); len(p) != 0 {
		t.Fatalf("CaveHost periods = %+v, want none", p)
	}
}

func TestOnlyTheAdminAssignsCaveHostPlans(t *testing.T) {
	t.Parallel()
	_, ada := caveHostSite(t)
	ada.assignCaveHost("ada@example.com", 30)
	if hasCaveHostLink(ada) {
		t.Fatal("a Learner gave themselves CaveHost")
	}
}

func TestALearnerWithoutACaveHostPlanIsToldItComesWithTheProPlan(t *testing.T) {
	t.Parallel()
	admin, ada := caveHostSite(t)
	res, body := ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusForbidden, "CaveHost comes with the Pro Plan")
	expectAll(t, body, `href="/pricing"`)

	// An ended period gives nothing either.
	admin.assignCaveHost("ada@example.com", 30)
	admin.ageAllCaveHostPeriods(40)
	res, body = ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusForbidden, "CaveHost comes with the Pro Plan")

	// A Visitor logs in first.
	res, body = admin.second().get("/cavehost")
	expectRedirect(t, res, body, "/login?next=%2Fcavehost")
}

func TestTheAdminOpensCaveHostWithoutACaveHostPlan(t *testing.T) {
	t.Parallel()
	admin, _ := caveHostSite(t)
	res, body := admin.get("/cavehost")
	expectStatus(t, res, body, http.StatusOK, "<h1>CaveHost</h1>")
	if !hasCaveHostLink(admin) {
		t.Fatal("the Admin does not see the CaveHost link")
	}
}

func TestCaveHostAndTheLabAreSeparateAccesses(t *testing.T) {
	t.Parallel()
	admin, ada := caveHostSite(t)
	admin.assignCaveHost("ada@example.com", 30)
	res, body := ada.get("/lab")
	expectRedirect(t, res, body, "/pricing")

	bea := admin.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	admin.createPlan("Lab Monthly", "30", "3")
	admin.assignPlan("bea@example.com", admin.onlyPlanID())
	if res, body = bea.get("/lab"); res.StatusCode != http.StatusOK {
		t.Fatalf("Bea with a Lab Plan cannot open the Lab: %d %s", res.StatusCode, body)
	}
	res, body = bea.get("/cavehost")
	expectStatus(t, res, body, http.StatusForbidden, "CaveHost comes with the Pro Plan")
	if hasCaveHostLink(bea) {
		t.Fatal("a Lab Plan gave the CaveHost link")
	}
}

func TestCaveHostScreensReadInMalay(t *testing.T) {
	t.Parallel()
	admin, ada := caveHostSite(t)
	res, body := ada.getAsking("/cavehost", "ms")
	expectStatus(t, res, body, http.StatusForbidden, "CaveHost disertakan dengan Pelan Pro")

	admin.assignCaveHost("ada@example.com", 30)
	res, body = admin.getAsking("/admin/plans/periods?email=ada@example.com", "ms")
	expectStatus(t, res, body, http.StatusOK, "Tempoh Pelan CaveHost")
	m := caveHostEndButton.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no end button: %s", body)
	}
	res, body = admin.getAsking(m[1], "ms")
	expectStatus(t, res, body, http.StatusOK, "Tamatkan Tempoh Pelan CaveHost")
}
