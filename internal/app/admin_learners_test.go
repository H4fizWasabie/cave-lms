package app_test

import (
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"testing"
)

// second returns a client on the same server with its own cookies, so the Admin and a Learner can both be logged in.
func (s *site) second() *site {
	c := *s.client
	c.Jar, _ = cookiejar.New(nil)
	o := *s
	o.client = &c
	return &o
}

func TestAdminSeesTheLearnersEnrolledInACourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	bea := s.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")

	res, body := s.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Basics</h1>")
	expectStatus(t, res, body, http.StatusOK, "Ada Lovelace")
	expectStatus(t, res, body, http.StatusOK, "ada@example.com")
	if regexp.MustCompile(`Bea Hopper|bea@example\.com`).MatchString(body) {
		t.Fatalf("Course page lists a Learner who has not joined: %s", body)
	}
}

var removeLearnerLink = regexp.MustCompile(`action="/admin/courses/\d+/learners/(\d+)/remove"`)

func TestAdminRemovesALearnerWhoLosesAccessAtOnce(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	_, body := s.get("/admin/courses/" + id)
	m := removeLearnerLink.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("Course page has no remove button for Ada: %s", body)
	}

	res, body := s.post("/admin/courses/"+id+"/learners/"+m[1]+"/remove", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/courses/"+id {
		t.Fatalf("remove redirected to %q, want /admin/courses/%s", loc, id)
	}
	if n := s.enrollmentCount("ada@example.com", id); n != 0 {
		t.Fatalf("Enrollments after removal = %d, want 0", n)
	}
	res, body = s.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "No Learners have joined this Course yet.")

	// Ada's very next request sees the Course as not joined.
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `action="/courses/`+id+`/join"`)
	if strings.Contains(body, "You have joined this Course") {
		t.Fatalf("removed Learner still sees the joined Course: %s", body)
	}
	res, body = ada.get("/my-courses")
	expectStatus(t, res, body, http.StatusOK, "You have not joined any Courses yet.")
}

func TestOnlyTheAdminSeesOrRemovesEnrolledLearners(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	_, body := s.get("/admin/courses/" + id)
	m := removeLearnerLink.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("Course page has no remove button for Ada: %s", body)
	}
	removePath := "/admin/courses/" + id + "/learners/" + m[1] + "/remove"

	visitor := s.second()
	res, body := visitor.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor list redirected to %q, want /login", loc)
	}
	res, body = visitor.post(removePath, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor removal redirected to %q, want /login", loc)
	}

	res, body = ada.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = ada.post(removePath, nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")

	if n := s.enrollmentCount("ada@example.com", id); n != 1 {
		t.Fatalf("refused removal changed Enrollments to %d, want 1", n)
	}
}

func TestAdminReachesACoursesLearnersFromTheCourseList(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, `href="/admin/courses/`+id+`">Learners</a>`)
}
