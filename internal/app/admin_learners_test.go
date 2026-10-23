package app_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
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

// adminCourseRow returns the table row on the Admin's Course page for the Learner with the given email.
func adminCourseRow(t *testing.T, body, email string) string {
	t.Helper()
	at := strings.Index(body, email)
	if at < 0 {
		t.Fatalf("Course page has no row for %s: %s", email, body)
	}
	start := strings.LastIndex(body[:at], "<tr>")
	end := strings.Index(body[at:], "</tr>")
	if start < 0 || end < 0 {
		t.Fatalf("no table row around %s: %s", email, body)
	}
	return body[start : at+end]
}

// expectCells checks that a row contains each cell, written exactly as the table renders it.
func expectCells(t *testing.T, row string, cells ...string) {
	t.Helper()
	for _, c := range cells {
		if !strings.Contains(row, c) {
			t.Fatalf("row lacks %s: %s", c, row)
		}
	}
}

func TestAdminSeesEachLearnersProgressBestScoreAndCertificate(t *testing.T) {
	const pw = "correct horse battery"
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", pw)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Body.", false)
	s.addTextLesson(id, "Functions", "", "Body.", false)
	s.saveExamSettings(id, "50", "", "")
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "What is a slice?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	qids := s.examQuestionIDs(id)
	variables := s.lessonIDByTitle("Variables")
	functions := s.lessonIDByTitle("Functions")

	// Ada finishes every Lesson and fails the Exam: 2 of 2, best 0%, no Certificate.
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", pw)
	ada.post("/courses/"+id+"/join", nil)
	ada.markLesson(id, variables, true)
	ada.markLesson(id, functions, true)
	res, _ := ada.startExam(id)
	ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})

	// Bea finishes every Lesson and passes: 2 of 2, best 100%, Certificate issued today.
	bea := s.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", pw)
	bea.setICFor("bea@example.com")
	bea.post("/courses/"+id+"/join", nil)
	bea.markLesson(id, variables, true)
	bea.markLesson(id, functions, true)
	res, _ = bea.startExam(id)
	bea.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	s.releaseResults()

	// Cara joined and did nothing: 0 of 2, no score, no Certificate.
	cara := s.second()
	cara.loginAsNewLearner("Cara Lamarr", "cara@example.com", pw)
	cara.post("/courses/"+id+"/join", nil)

	res, body := s.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<th>Progress</th>")
	today := time.Now().UTC().Format("2 January 2006")
	expectCells(t, adminCourseRow(t, body, "ada@example.com"), "<td>2 of 2</td>", "<td>0%</td>", "<td>None</td>")
	expectCells(t, adminCourseRow(t, body, "bea@example.com"), "<td>2 of 2</td>", "<td>100%</td>", "<td>Issued "+today+"</td>")
	expectCells(t, adminCourseRow(t, body, "cara@example.com"), "<td>0 of 2</td>", "<td>-</td>", "<td>None</td>")
}

func TestAdminSeesCertificateRevokedAndDashWhenCourseHasNoExam(t *testing.T) {
	const pw = "correct horse battery"
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", pw)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Body.", false)
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", pw)
	ada.setICFor("ada@example.com")
	ada.post("/courses/"+id+"/join", nil)
	ada.markLesson(id, s.lessonIDByTitle("Variables"), true)
	today := time.Now().UTC().Format("2 January 2006")

	_, body := s.get("/admin/courses/" + id)
	expectCells(t, adminCourseRow(t, body, "ada@example.com"), "<td>1 of 1</td>", "<td>-</td>", "<td>Issued "+today+"</td>")

	if _, err := s.db.Exec(`UPDATE certificates SET revoked_at = now()`); err != nil {
		t.Fatal(err)
	}
	_, body = s.get("/admin/courses/" + id)
	expectCells(t, adminCourseRow(t, body, "ada@example.com"), "<td>1 of 1</td>", "<td>-</td>", "<td>Revoked</td>")
}

func TestAdminCoursePageShowsIssuedOnceARevokedCertificateIsReplaced(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	id := s.onlyCourseID()
	s.post("/admin/certificates/"+certID+"/revoke", nil)
	res, body := s.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<td>Revoked</td>")

	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<td>Issued ")
	if strings.Contains(body, "Revoked") {
		t.Fatalf("Admin Course page still shows the replaced Certificate as revoked: %s", body)
	}
}
