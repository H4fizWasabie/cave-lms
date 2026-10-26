package app_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// courseIDByTitle reads the ID of the Course titled title from the public Course list.
func (s *site) courseIDByTitle(title string) string {
	s.t.Helper()
	_, body := s.get("/courses")
	m := regexp.MustCompile(`href="/courses/(\d+)">` + regexp.QuoteMeta(title) + `</a>`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no Course titled %q on the Course list: %s", title, body)
	}
	return m[1]
}

// leaveCourse is the logged-in Learner leaving a Course through its confirmation page.
func (s *site) leaveCourse(courseID string) (*http.Response, string) {
	return s.post("/courses/"+courseID+"/leave", confirming)
}

// enrollmentCount counts the Enrollments of a Learner in a Course, read straight from the database.
func (s *site) enrollmentCount(learnerEmail, courseID string) int {
	s.t.Helper()
	var n int
	err := s.db.QueryRowContext(context.Background(), `
		SELECT count(*) FROM enrollments e JOIN learners l ON l.id = e.learner_id
		WHERE lower(l.email) = lower($1) AND e.course_id = $2`, learnerEmail, courseID).Scan(&n)
	if err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestLearnerJoinsAnOpenCourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	res, body := s.post("/courses/"+id+"/join", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/courses/"+id {
		t.Fatalf("join redirected to %q, want /courses/%s", loc, id)
	}
	if n := s.enrollmentCount("ada@example.com", id); n != 1 {
		t.Fatalf("Enrollments after join = %d, want 1", n)
	}
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "You have joined this Course")
}

func TestCoursePageOffersJoinOnlyToLearnersWhoHaveNotJoined(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()

	res, body := s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `href="/login"`)
	if strings.Contains(body, `action="/courses/`+id+`/join"`) {
		t.Fatalf("Visitor sees a join button: %s", body)
	}

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `action="/courses/`+id+`/join"`)
	if strings.Contains(body, "You have joined this Course") {
		t.Fatalf("Learner who has not joined sees the joined message: %s", body)
	}
}

func TestClosedCourseRefusesNewJoinsButKeepsExistingMembers(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	s.logOut()

	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.post("/admin/courses/"+id+"/close", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()

	s.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	res, body = s.post("/courses/"+id+"/join", nil)
	expectStatus(t, res, body, http.StatusConflict, "This Course is closed to new joins.")
	if n := s.enrollmentCount("bea@example.com", id); n != 0 {
		t.Fatalf("closed Course enrolled Bea: %d Enrollments", n)
	}
	s.logOut()

	s.login("ada@example.com", "correct horse battery")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "You have joined this Course")
	if n := s.enrollmentCount("ada@example.com", id); n != 1 {
		t.Fatalf("closing the Course removed Ada's Enrollment: %d", n)
	}
}

func TestVisitorWhoJoinsIsSentToLogIn(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()

	res, body := s.post("/courses/"+id+"/join", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor join redirected to %q, want /login", loc)
	}
	if n := s.enrollmentCount("admin@example.com", id); n != 0 {
		t.Fatalf("Visitor join created %d Enrollments", n)
	}
}

func TestLearnerLeavesACourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	res, body := s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `href="/courses/`+id+`/leave"`)

	res, body = s.get("/courses/" + id + "/leave")
	expectStatus(t, res, body, http.StatusOK, "Leave “Go Basics”?")
	expectAll(t, body, `action="/courses/`+id+`/leave"`, "Stay in this Course")
	if n := s.enrollmentCount("ada@example.com", id); n != 1 {
		t.Fatalf("the confirmation page alone left the Course: %d Enrollments", n)
	}

	res, body = s.post("/courses/"+id+"/leave", nil) // not confirmed, as from an old Course page
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/courses/"+id+"/leave" {
		t.Fatalf("unconfirmed leave went to %q, want the confirmation page", loc)
	}
	if n := s.enrollmentCount("ada@example.com", id); n != 1 {
		t.Fatalf("an unconfirmed leave left the Course: %d Enrollments", n)
	}

	res, body = s.post("/courses/"+id+"/leave", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := s.enrollmentCount("ada@example.com", id); n != 0 {
		t.Fatalf("Enrollments after leaving = %d, want 0", n)
	}
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `action="/courses/`+id+`/join"`)
}

func TestMyCoursesListsOnlyTheLearnersCourses(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	s.createCourse("Rust Basics", "Learn Rust from scratch.", "")
	s.createCourse("Java Basics", "Learn Java from scratch.", "")
	goID, rustID := s.courseIDByTitle("Go Basics"), s.courseIDByTitle("Rust Basics")
	s.logOut()

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := s.get("/my-courses")
	expectStatus(t, res, body, http.StatusOK, "You have not joined any Courses yet.")

	s.post("/courses/"+goID+"/join", nil)
	s.post("/courses/"+rustID+"/join", nil)
	res, body = s.get("/my-courses")
	expectStatus(t, res, body, http.StatusOK, "Go Basics")
	expectStatus(t, res, body, http.StatusOK, "Rust Basics")
	if strings.Contains(content(body), "Java Basics") { // the header's Courses menu lists it
		t.Fatalf("My courses lists a Course Ada has not joined: %s", body)
	}
	if strings.Contains(body, "You have not joined any Courses yet.") {
		t.Fatalf("My courses shows the empty message to a Learner with Courses: %s", body)
	}

	s.leaveCourse(goID)
	res, body = s.get("/my-courses")
	if strings.Contains(content(body), "Go Basics") {
		t.Fatalf("My courses still lists a Course Ada left: %s", body)
	}
}

func TestVisitorIsSentToLogInFromMyCourses(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/my-courses?tab=all")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	loginURL := res.Header.Get("Location")
	if loginURL != "/login?next=%2Fmy-courses%3Ftab%3Dall" {
		t.Fatalf("Visitor My courses redirected to %q, want the login page with next", loginURL)
	}
	res, body = s.get(loginURL)
	expectStatus(t, res, body, http.StatusOK, `name="next" value="/my-courses?tab=all"`)

	const email, password = "ada@example.com", "correct horse battery"
	s.register("Ada Lovelace", email, password)
	s.get(s.lastVerifyPath(email))
	res, body = s.post("/login", url.Values{"email": {email}, "password": {password}, "next": {"/my-courses?tab=all"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/my-courses?tab=all" {
		t.Fatalf("login redirected to %q, want back to My courses", got)
	}
}

func TestVisitorPostingToALearnerPageIsSentToPlainLogIn(t *testing.T) {
	s := newSite(t)
	res, body := s.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {"Mine"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor POST redirected to %q, want /login", loc)
	}
}

func TestCourseWithEnrollmentsCannotBeDeleted(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")

	res, body := s.post("/admin/courses/"+id+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "This Course cannot be deleted while it has Lessons, Learners, Exam attempts or Certificates.")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Basics</h1>")
}
