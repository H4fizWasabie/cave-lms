package app_test

import (
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"net/http"
	"strings"
	"testing"
)

func TestCoursesNoLongerOfferTheLab(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.courseIDByTitle("Go Basics")

	for _, path := range []string{"/admin/courses", "/admin/courses/new", "/admin/courses/" + id + "/edit"} {
		res, body := s.get(path)
		expectStatus(t, res, body, http.StatusOK, "Course")
		if strings.Contains(body, "includes_lab") || strings.Contains(body, "Includes the Lab") {
			t.Fatalf("%s still offers or shows includes the Lab: %s", path, body)
		}
	}
}

func TestAVisitorSeesNoLabLinkAndTheLabGoesToPricing(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, path := range []string{"/", "/gallery", "/pricing"} {
		res, body := s.get(path)
		expectStatus(t, res, body, http.StatusOK, "")
		expectNone(t, body, `href="/lab"`)
	}
	for _, path := range []string{"/lab", "/lab/new", "/lab/dashboards/1", "/lab/dashboards/1/data"} {
		res, body := s.get(path)
		expectRedirect(t, res, body, "/pricing")
	}
}

// expectLabLinks fails unless the page at path links to the Lab in the header twice (wide and Menu) and in the footer.
func expectLabLinks(t *testing.T, s *site, path string) {
	t.Helper()
	_, body := s.getAsking(path, "en")
	head := header(t, body)
	if n := strings.Count(head, `<a class="hdr-link" href="/lab">Lab</a>`); n != 2 {
		t.Errorf("%s: header has %d Lab links, want 2 (wide and Menu); header:\n%s", path, n, head)
	}
	_, foot, _ := strings.Cut(content(body), "</main>")
	expectAll(t, foot, `<a href="/lab">Lab</a>`)
}

// The Lab link shows only to those the Lab is open to: a Learner in an active Plan period, and the Admin (ADR 0018).
func TestTheLabLinkShowsOnlyToAPlanHolderAndTheAdmin(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	for _, path := range []string{"/", "/my-courses"} {
		_, body := ada.get(path)
		expectNone(t, body, `href="/lab"`)
	}

	admin.assignPlan("ada@example.com", planID)
	expectLabLinks(t, ada, "/")
	expectLabLinks(t, ada, "/my-courses")
	expectLabLinks(t, admin, "/")

	admin.ageAllPeriods(31)
	for _, path := range []string{"/", "/my-courses"} {
		_, body := ada.get(path)
		expectNone(t, body, `href="/lab"`)
	}
}

// labSetup logs in the Admin, creates a 30-day Plan with 3 Deployments and a Course, and returns the Admin,
// a logged-in Learner Ada, the Plan ID and the Course ID.
func labSetup(t *testing.T, opts ...func(*config.Config)) (admin, ada *site, planID, courseID string) {
	admin, planID = adminWithPlanAndLearner(t, opts...)
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	courseID = admin.courseIDByTitle("Go Basics")
	ada = admin.second()
	ada.login("ada@example.com", "correct horse battery")
	return admin, ada, planID, courseID
}

func TestLearnerWithAPlanAndNoCourseGetsTheLab(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ends := admin.periodsOf("ada@example.com")[0].ends.Format("2006-01-02 15:04 UTC")

	res, body := ada.get("/lab")
	for _, want := range []string{"Lab Plan", "Ends " + ends, "Deployments awake", "0 of 3", "My Dashboards"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	expectNone(t, body, "Lab Monthly") // the Admin's name for the Plan stays in admin
	if strings.Contains(body, "Upgrade") || strings.Contains(body, "Turn a spreadsheet") {
		t.Fatalf("a Learner with access sees the demo: %s", body)
	}
}

func TestLearnerInACourseWithoutAPlanGoesToPricing(t *testing.T) {
	t.Parallel()
	_, ada, _, courseID := labSetup(t)
	ada.post("/courses/"+courseID+"/join", nil)

	res, body := ada.get("/lab")
	expectRedirect(t, res, body, "/pricing")
}

func TestAnEndedPlanGivesNoLabAccess(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)

	admin.ageAllPeriods(31)
	res, body := ada.get("/lab")
	expectRedirect(t, res, body, "/pricing")

	admin.assignPlan("ada@example.com", planID)
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments awake")

	_, page := admin.get("/admin/plans/periods?email=ada@example.com")
	admin.post(endPeriodButton.FindStringSubmatch(page)[1], confirming)
	res, body = ada.get("/lab")
	expectRedirect(t, res, body, "/pricing")
}

func TestLeavingACourseNeverWarnsAboutTheLab(t *testing.T) {
	t.Parallel()
	admin, ada, planID, courseID := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+courseID+"/join", nil)

	res, body := ada.leaveCourse(courseID)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments awake")
}
