package app_test

import (
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"net/http"
	"strings"
	"testing"
)

func TestCoursesNoLongerOfferTheLab(t *testing.T) {
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

func TestAVisitorFollowingTheLabLinkLandsOnPricing(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/")
	expectStatus(t, res, body, http.StatusOK, `<a href="/lab">Lab</a>`)
	res, body = s.get("/lab")
	expectRedirect(t, res, body, "/pricing")
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
	admin, ada, planID, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ends := admin.periodsOf("ada@example.com")[0].ends.Format("2006-01-02 15:04 UTC")

	res, body := ada.get("/lab")
	for _, want := range []string{"Lab Monthly", "Ends " + ends, "Deployments awake", "0 of 3", "My Dashboards"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, "Upgrade") || strings.Contains(body, "Turn a spreadsheet") {
		t.Fatalf("a Learner with access sees the demo: %s", body)
	}
}

func TestLearnerInACourseWithoutAPlanGoesToPricing(t *testing.T) {
	_, ada, _, courseID := labSetup(t)
	ada.post("/courses/"+courseID+"/join", nil)

	res, body := ada.get("/lab")
	expectRedirect(t, res, body, "/pricing")
}

func TestAnEndedPlanGivesNoLabAccess(t *testing.T) {
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
	admin, ada, planID, courseID := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+courseID+"/join", nil)

	res, body := ada.leaveCourse(courseID)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments awake")
}
