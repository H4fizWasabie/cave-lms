package app_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// createCourse posts the Course form as the logged-in Admin.
func (s *site) createCourse(title, description, startDate string) (*http.Response, string) {
	return s.post("/admin/courses", url.Values{"title": {title}, "description": {description}, "start_date": {startDate}})
}

// logOut drops the session cookie so the client is a Visitor again.
func (s *site) logOut() {
	s.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		s.t.Fatal(err)
	}
	s.client.Jar = jar
}

var courseEditLink = regexp.MustCompile(`/admin/courses/(\d+)/edit`)

// onlyCourseID reads the ID of the single Course listed on the Admin's Course page.
func (s *site) onlyCourseID() string {
	s.t.Helper()
	_, body := s.get("/admin/courses")
	m := courseEditLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Course on the Admin Course list: %s", body)
	}
	return m[1]
}

func TestAdminCreatesACourseThatVisitorsCanSee(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.createCourse("Go Basics", "Learn Go from scratch.", "2027-01-15")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	res, body = s.get("/courses")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
	expectStatus(t, res, body, http.StatusOK, "Learn Go from scratch.")
	expectStatus(t, res, body, http.StatusOK, "2027-01-15")
	expectStatus(t, res, body, http.StatusOK, "Open to new joins")
}

func TestCourseListAndPageShowTheCourseToLearnersToo(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	res, body := s.get("/courses")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Basics</h1>")
	expectStatus(t, res, body, http.StatusOK, "Learn Go from scratch.")
	expectStatus(t, res, body, http.StatusOK, "Open to new joins")
}

func TestUnknownCourseIsNotFound(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/courses/999")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Course at this address.")
	res, body = s.get("/courses/abc")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Course at this address.")
}

func TestAdminEditsACourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `value="Go Basics"`)

	res, body = s.post("/admin/courses/"+id, url.Values{"title": {"Go Advanced"}, "description": {"Concurrency and more."}, "start_date": {"2027-03-01"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Advanced</h1>")
	expectStatus(t, res, body, http.StatusOK, "Concurrency and more.")
	expectStatus(t, res, body, http.StatusOK, "Start date: 2027-03-01")
	if strings.Contains(body, "Go Basics") {
		t.Fatalf("edited Course still shows its old title: %s", body)
	}
}

func TestCourseFormRejectsInvalidInput(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.createCourse("  ", "Learn Go.", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a title.")
	res, body = s.createCourse("Go Basics", "", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a description.")
	res, body = s.createCourse("Go Basics", "Learn Go.", "15/01/2027")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the start date as YYYY-MM-DD")

	res, body = s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, "No Courses yet.")
}

func TestAdminOpensAndClosesACourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.post("/admin/courses/"+id+"/close", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Closed to new joins")

	res, body = s.post("/admin/courses/"+id+"/open", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Open to new joins")
}

func TestAdminDeletesACourseAfterConfirming(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses/" + id + "/delete")
	expectStatus(t, res, body, http.StatusOK, "Delete &ldquo;Go Basics&rdquo;? This cannot be undone.")

	res, body = s.post("/admin/courses/"+id+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusNotFound, "There is no Course at this address.")
	res, body = s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, "No Courses yet.")
}

func TestCourseAdminPagesRefuseVisitorsAndLearners(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()

	pages := []struct{ method, path string }{
		{http.MethodGet, "/admin/courses"},
		{http.MethodGet, "/admin/courses/new"},
		{http.MethodPost, "/admin/courses"},
		{http.MethodGet, "/admin/courses/" + id + "/edit"},
		{http.MethodPost, "/admin/courses/" + id},
		{http.MethodPost, "/admin/courses/" + id + "/close"},
		{http.MethodGet, "/admin/courses/" + id + "/delete"},
		{http.MethodPost, "/admin/courses/" + id + "/delete"},
	}
	check := func(role string, wantStatus int, wantText string) {
		t.Helper()
		for _, p := range pages {
			var res *http.Response
			var body string
			if p.method == http.MethodPost {
				res, body = s.post(p.path, url.Values{"title": {"Hacked"}, "description": {"Hacked"}})
			} else {
				res, body = s.get(p.path)
			}
			if res.StatusCode != wantStatus || !strings.Contains(body, wantText) {
				t.Fatalf("%s %s as %s = %d, want %d containing %q", p.method, p.path, role, res.StatusCode, wantStatus, wantText)
			}
		}
	}

	check("a Visitor", http.StatusSeeOther, "")
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	check("a Learner", http.StatusForbidden, "Only the Admin can open this page")

	res, body := s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
}
