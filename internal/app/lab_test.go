package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// createLabCourse creates a Course that includes the Lab, as the logged-in Admin, and returns its ID.
func (s *site) createLabCourse(title string) string {
	s.t.Helper()
	res, body := s.post("/admin/courses", url.Values{"title": {title}, "description": {"Dashboards from Excel."}, "includes_lab": {"on"}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return s.courseIDByTitle(title)
}

func TestAdminMarksACourseAsIncludingTheLab(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.createLabCourse("Excel Dashboards")

	labRow := regexp.MustCompile(`Excel Dashboards</a> <span class="badge[^"]*">Includes the Lab`)
	res, body := s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, "Includes the Lab")
	if strings.Count(body, "Includes the Lab") != 1 || !labRow.MatchString(body) {
		t.Fatalf("want only Excel Dashboards marked as including the Lab: %s", body)
	}
	res, body = s.get("/admin/courses/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `name="includes_lab" value="on" checked`)

	// Saving the form without the box ticked turns it off.
	res, body = s.post("/admin/courses/"+id, url.Values{"title": {"Excel Dashboards"}, "description": {"Dashboards from Excel."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses")
	if strings.Contains(body, "Includes the Lab") {
		t.Fatalf("the Lab flag is still on after unticking it: %s", body)
	}
}

func TestVisitorSeesWhatTheLabIsWithRegister(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/")
	expectStatus(t, res, body, http.StatusOK, `<a href="/lab">Lab</a>`)
	res, body = s.get("/lab")
	for _, want := range []string{"Turn a spreadsheet into a live dashboard", "Register to get started", `href="/register"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

func TestLearnerWithoutAPlanIsOfferedUpgrade(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Contact us to upgrade")
	expectStatus(t, res, body, http.StatusOK, "Building, uploading and deploying need a Plan")
	if strings.Contains(body, "Register to get started") {
		t.Fatalf("a Learner is asked to register: %s", body)
	}

	s.createPlan("Lab Monthly", "30", "3", "https://pay.example.com/lab")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `href="https://pay.example.com/lab">Upgrade</a>`)
	if strings.Contains(body, "Contact us to upgrade") {
		t.Fatalf("Contact us shown although a payment link is set: %s", body)
	}
}

// labSetup logs in the Admin, creates a 30-day Plan with 3 Deployments, a Course that includes
// the Lab and one that does not, and returns the Admin, a logged-in Learner Ada and the Plan and Course IDs.
func labSetup(t *testing.T) (admin, ada *site, planID, labCourse, otherCourse string) {
	admin, planID = adminWithPlanAndLearner(t)
	labCourse = admin.createLabCourse("Excel Dashboards")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	otherCourse = admin.courseIDByTitle("Go Basics")
	ada = admin.second()
	ada.login("ada@example.com", "correct horse battery")
	return admin, ada, planID, labCourse, otherCourse
}

func TestLearnerWithAPlanButNoLabCourseIsToldWhichCourseToJoin(t *testing.T) {
	admin, ada, planID, labCourse, otherCourse := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+otherCourse+"/join", nil)

	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Your Plan is active until")
	expectStatus(t, res, body, http.StatusOK, `href="/courses/`+labCourse+`">Join Excel Dashboards</a>`)
	if strings.Contains(body, "Go Basics") || strings.Contains(body, "Upgrade") {
		t.Fatalf("want only the Lab Course offered, and no Upgrade: %s", body)
	}
}

func TestLearnerWithAPlanAndALabCourseGetsTheLab(t *testing.T) {
	admin, ada, planID, labCourse, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+labCourse+"/join", nil)
	ends := admin.periodsOf("ada@example.com")[0].ends.Format("2006-01-02 15:04 UTC")

	res, body := ada.get("/lab")
	for _, want := range []string{"Lab Monthly", "Ends " + ends, "Deployments left this period", "3 of 3", "My Dashboards"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, "Upgrade") || strings.Contains(body, "Turn a spreadsheet") {
		t.Fatalf("a Learner with access sees the demo: %s", body)
	}
}

func TestAnEndedPlanGivesNoLabAccess(t *testing.T) {
	admin, ada, planID, labCourse, _ := labSetup(t)
	ada.post("/courses/"+labCourse+"/join", nil)
	admin.assignPlan("ada@example.com", planID)

	admin.ageAllPeriods(31)
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Building, uploading and deploying need a Plan")

	admin.assignPlan("ada@example.com", planID)
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments left this period")

	_, page := admin.get("/admin/plans/periods?email=ada@example.com")
	admin.post(endPeriodButton.FindStringSubmatch(page)[1], nil)
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Building, uploading and deploying need a Plan")
}

func TestLeavingTheLastLabCourseWarnsAndLeavesOnlyAfterConfirmation(t *testing.T) {
	admin, ada, planID, labCourse, _ := labSetup(t)
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+labCourse+"/join", nil)

	res, body := ada.post("/courses/"+labCourse+"/leave", nil)
	expectStatus(t, res, body, http.StatusOK, "your Lab will lock")
	expectStatus(t, res, body, http.StatusOK, `name="confirm" value="yes"`)
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments left this period")

	res, body = ada.post("/courses/"+labCourse+"/leave", url.Values{"confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Join Excel Dashboards")
	if strings.Contains(body, "Deployments left this period") {
		t.Fatalf("the Lab is still open after leaving its only Course: %s", body)
	}
}

func TestLeavingALabCourseWithAnotherStillJoinedDoesNotWarn(t *testing.T) {
	admin, ada, planID, labCourse, _ := labSetup(t)
	second := admin.createLabCourse("Excel Dashboards Two")
	admin.assignPlan("ada@example.com", planID)
	ada.post("/courses/"+labCourse+"/join", nil)
	ada.post("/courses/"+second+"/join", nil)

	res, body := ada.post("/courses/"+labCourse+"/leave", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Deployments left this period")
}
