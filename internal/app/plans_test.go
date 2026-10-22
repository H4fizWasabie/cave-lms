package app_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// createPlan posts the Plan form as the logged-in Admin.
func (s *site) createPlan(name, days, deployments, paymentLink string) (*http.Response, string) {
	return s.post("/admin/plans", url.Values{"name": {name}, "days": {days}, "deployments": {deployments}, "storage": {"1024"}, "payment_link": {paymentLink}})
}

func TestAdminCreatesAPlan(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.createPlan("Lab Monthly", "30", "3", "https://pay.example.com/lab")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/plans" {
		t.Fatalf("create redirected to %q, want /admin/plans", loc)
	}
	res, body = s.get("/admin/plans")
	for _, want := range []string{"Lab Monthly", "30 days", "3 Deployments", "https://pay.example.com/lab"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

var planEditLink = regexp.MustCompile(`/admin/plans/(\d+)/edit`)

// onlyPlanID reads the ID of the single Plan listed on the Admin's Plans page.
func (s *site) onlyPlanID() string {
	s.t.Helper()
	_, body := s.get("/admin/plans")
	m := planEditLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Plan on the Admin's Plans page: %s", body)
	}
	return m[1]
}

func TestAdminEditsAPlan(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3", "")
	id := s.onlyPlanID()
	res, body := s.get("/admin/plans/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `value="Lab Monthly"`)

	res, body = s.post("/admin/plans/"+id, url.Values{"name": {"Lab Quarterly"}, "days": {"90"}, "deployments": {"10"}, "storage": {"1024"}, "payment_link": {"https://pay.example.com/q"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/plans")
	for _, want := range []string{"Lab Quarterly", "90 days", "10 Deployments", "https://pay.example.com/q"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, "Lab Monthly") {
		t.Fatalf("Plans page still shows the old name: %s", body)
	}
}

func TestInvalidPlansAreRefusedWithAMessage(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	for _, tc := range []struct{ name, days, deployments, link, want string }{
		{"  ", "30", "3", "", "Enter a name."},
		{"Lab", "0", "3", "", "1 or more"},
		{"Lab", "-5", "3", "", "1 or more"},
		{"Lab", "thirty", "3", "", "1 or more"},
		{"Lab", "30", "-1", "", "0 or more"},
		{"Lab", "30", "3", "javascript:alert(1)", "https://"},
	} {
		res, body := s.createPlan(tc.name, tc.days, tc.deployments, tc.link)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, tc.want)
	}
	res, body := s.get("/admin/plans")
	expectStatus(t, res, body, http.StatusOK, "No Plans yet.")

	s.createPlan("Lab Monthly", "30", "3", "")
	id := s.onlyPlanID()
	res, body = s.post("/admin/plans/"+id, url.Values{"name": {"Lab Monthly"}, "days": {"0"}, "deployments": {"3"}, "storage": {"1024"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "1 or more")
	res, body = s.get("/admin/plans")
	expectStatus(t, res, body, http.StatusOK, "30 days")
}

func TestOnlyTheAdminReachesThePlanPages(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3", "")
	id := s.onlyPlanID()
	visitor := s.second()
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	for _, path := range []string{"/admin/plans", "/admin/plans/new", "/admin/plans/" + id + "/edit", "/admin/plans/periods?email=ada@example.com"} {
		res, body := visitor.get(path)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	form := url.Values{"name": {"Free"}, "days": {"999"}, "deployments": {"99"}, "email": {"ada@example.com"}, "plan_id": {id}}
	for _, path := range []string{"/admin/plans", "/admin/plans/" + id, "/admin/plans/periods"} {
		res, body := visitor.post(path, form)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.post(path, form)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	res, body := s.get("/admin/plans")
	expectStatus(t, res, body, http.StatusOK, "Lab Monthly")
	if strings.Contains(body, "Free") {
		t.Fatalf("a non-Admin changed the Plans: %s", body)
	}
	res, body = s.get("/admin/plans/periods?email=ada@example.com")
	expectStatus(t, res, body, http.StatusOK, "No Plan periods yet.")
}

// assignPlan assigns the Plan to the Learner with this email, as the logged-in Admin.
func (s *site) assignPlan(email, planID string) (*http.Response, string) {
	return s.post("/admin/plans/periods", url.Values{"email": {email}, "plan_id": {planID}})
}

type shownPeriod struct {
	plan, deployments, status string
	starts, ends              time.Time
}

var periodRow = regexp.MustCompile(`<tr>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>(\d+)</td>\s*<td>([^<]+)</td>`)

// periodsOf reads the Plan periods the Admin's page shows for the Learner with this email, in page order.
func (s *site) periodsOf(email string) []shownPeriod {
	s.t.Helper()
	res, body := s.get("/admin/plans/periods?email=" + url.QueryEscape(email))
	expectStatus(s.t, res, body, http.StatusOK, "Plan periods")
	var list []shownPeriod
	for _, m := range periodRow.FindAllStringSubmatch(body, -1) {
		starts, err := time.Parse("2006-01-02 15:04 MST", m[2])
		if err != nil {
			s.t.Fatal(err)
		}
		ends, err := time.Parse("2006-01-02 15:04 MST", m[3])
		if err != nil {
			s.t.Fatal(err)
		}
		list = append(list, shownPeriod{plan: m[1], starts: starts, ends: ends, deployments: m[4], status: m[5]})
	}
	return list
}

// ageAllPeriods moves every Plan period into the past, the way the expiry tests age rows.
func (s *site) ageAllPeriods(days int) {
	s.t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE plan_periods SET starts_at = starts_at - make_interval(days => $1), ends_at = ends_at - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

// adminWithPlanAndLearner logs in the Admin, creates a 30-day Plan with 3 Deployments and registers Ada.
func adminWithPlanAndLearner(t *testing.T) (*site, string) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createPlan("Lab Monthly", "30", "3", "")
	s.second().loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	return s, s.onlyPlanID()
}

func TestAssigningAPlanToAnUnknownEmailIsRefused(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	res, body := s.assignPlan("nobody@example.com", id)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "No Learner has the email nobody@example.com")
	res, body = s.get("/admin/plans/periods?email=nobody@example.com")
	expectStatus(t, res, body, http.StatusNotFound, "No Learner has the email nobody@example.com")
}

func TestAPlanAssignedWithNoActivePeriodStartsNow(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	res, body := s.assignPlan("ADA@example.com", id)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	periods := s.periodsOf("ada@example.com")
	if len(periods) != 1 {
		t.Fatalf("periods = %+v, want 1", periods)
	}
	p := periods[0]
	if since := time.Since(p.starts); since < -time.Minute || since > 2*time.Minute {
		t.Fatalf("period starts %v, want now", p.starts)
	}
	if want := p.starts.AddDate(0, 0, 30); !p.ends.Equal(want) {
		t.Fatalf("period ends %v, want %v (30 days on)", p.ends, want)
	}
	if p.plan != "Lab Monthly" || p.deployments != "3" || p.status != "Active" {
		t.Fatalf("period = %+v, want Lab Monthly, 3 Deployments, Active", p)
	}

	// Once that period is over, a new one starts now again rather than at its old end.
	s.ageAllPeriods(40)
	s.assignPlan("ada@example.com", id)
	periods = s.periodsOf("ada@example.com")
	if len(periods) != 2 || periods[0].status != "Ended" || periods[1].status != "Active" {
		t.Fatalf("periods = %+v, want an Ended one then an Active one", periods)
	}
	if since := time.Since(periods[1].starts); since < -time.Minute || since > 2*time.Minute {
		t.Fatalf("new period starts %v, want now", periods[1].starts)
	}
}

func TestPlanPeriodsStackSoNoDaysAreLost(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	s.createPlan("Lab Week", "7", "1", "")
	_, body := s.get("/admin/plans")
	weekID := regexp.MustCompile(`Lab Week</td>[\s\S]*?/admin/plans/(\d+)/edit`).FindStringSubmatch(body)[1]

	s.assignPlan("ada@example.com", id)
	s.assignPlan("ada@example.com", weekID)
	s.assignPlan("ada@example.com", id)

	p := s.periodsOf("ada@example.com")
	if len(p) != 3 {
		t.Fatalf("periods = %+v, want 3", p)
	}
	if !p[1].starts.Equal(p[0].ends) || !p[2].starts.Equal(p[1].ends) {
		t.Fatalf("periods do not stack end to start: %+v", p)
	}
	if !p[1].ends.Equal(p[1].starts.AddDate(0, 0, 7)) || !p[2].ends.Equal(p[2].starts.AddDate(0, 0, 30)) {
		t.Fatalf("stacked periods have the wrong length: %+v", p)
	}
	if p[0].status != "Active" || p[1].status != "Upcoming" || p[1].plan != "Lab Week" || p[2].status != "Upcoming" {
		t.Fatalf("periods = %+v, want Active, then Upcoming Lab Week, then Upcoming", p)
	}
}

func TestEditingAPlanLeavesAssignedPeriodsAlone(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	s.assignPlan("ada@example.com", id)
	s.post("/admin/plans/"+id, url.Values{"name": {"Lab Monthly"}, "days": {"30"}, "deployments": {"9"}, "storage": {"1024"}})
	s.assignPlan("ada@example.com", id)

	p := s.periodsOf("ada@example.com")
	if len(p) != 2 || p[0].deployments != "3" || p[1].deployments != "9" {
		t.Fatalf("periods = %+v, want 3 Deployments then 9", p)
	}
}

var endPeriodButton = regexp.MustCompile(`action="(/admin/plans/periods/\d+/end)"`)

func TestAdminEndsAPeriodEarly(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	s.assignPlan("ada@example.com", id)
	s.assignPlan("ada@example.com", id)
	_, body := s.get("/admin/plans/periods?email=ada@example.com")
	buttons := endPeriodButton.FindAllStringSubmatch(body, -1)
	if len(buttons) != 2 {
		t.Fatalf("want an end button per period: %s", body)
	}

	res, body := s.post(buttons[0][1], nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/plans/periods?email=ada%40example.com" {
		t.Fatalf("end redirected to %q", loc)
	}
	p := s.periodsOf("ada@example.com")
	if len(p) != 2 || !strings.HasPrefix(p[0].status, "Ended early on ") || p[1].status != "Upcoming" {
		t.Fatalf("periods = %+v, want the first Ended early", p)
	}
	_, body = s.get("/admin/plans/periods?email=ada@example.com")
	if n := len(endPeriodButton.FindAllString(body, -1)); n != 1 {
		t.Fatalf("an ended period still has an end button: %s", body)
	}
}
