package app_test

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// expectLoginLandsOn logs in and checks where the login sends the account.
func (s *site) expectLoginLandsOn(email, password, next, want string) {
	s.t.Helper()
	res, body := s.post("/login", url.Values{"email": {email}, "password": {password}, "next": {next}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != want {
		s.t.Fatalf("login of %s with next %q landed on %q, want %q", email, next, got, want)
	}
}

func TestLoggingInTakesALearnerOrAnAdminToMyCoursesUnlessNextSaysOtherwise(t *testing.T) {
	s := newSite(t)
	const password = "correct horse battery"
	s.loginAsNewLearner("Ada", "ada@example.com", password)
	s.logOut()
	if err := s.application.CreateAdmin(t.Context(), "Abah", "admin@example.com", password); err != nil {
		t.Fatal(err)
	}

	s.expectLoginLandsOn("ada@example.com", password, "", "/my-courses")
	s.logOut()
	s.expectLoginLandsOn("admin@example.com", password, "", "/my-courses")
	s.logOut()

	s.expectLoginLandsOn("ada@example.com", password, "/courses?level=1", "/courses?level=1")
	s.logOut()
	s.expectLoginLandsOn("admin@example.com", password, "/courses?level=1", "/courses?level=1")
	s.logOut()
	s.expectLoginLandsOn("admin@example.com", password, "//evil.example/", "/my-courses") // an off-site next is dropped
}

var myCourseNextStep = regexp.MustCompile(`data-next-step([^>]*)>\s*([^<]+?)\s*<`)

// myCourses is the main part of My courses, in English.
func (s *site) myCourses() string {
	s.t.Helper()
	res, body := s.get("/my-courses")
	expectStatus(s.t, res, body, http.StatusOK, "")
	return mainOf(body)
}

// myCourse is a Course's row on My courses.
func (s *site) myCourse(courseID string) string {
	s.t.Helper()
	body := s.myCourses()
	start := strings.Index(body, `id="my-course-`+courseID+`"`)
	if start < 0 {
		s.t.Fatalf("My courses has no row for Course %s: %s", courseID, body)
	}
	row := body[start:]
	return row[:strings.Index(row, "</article>")]
}

// expectMyCourse checks a Course's row on My courses: its progress line (no progress bar when ""), its next step,
// and that it says each of wants.
func (s *site) expectMyCourse(courseID, progress, label, href string, wants ...string) {
	s.t.Helper()
	row := s.myCourse(courseID)
	if progress == "" && strings.Contains(row, "<progress") || progress != "" && !strings.Contains(row, progress) {
		s.t.Fatalf("My courses progress, want %q: %s", progress, row)
	}
	m := myCourseNextStep.FindStringSubmatch(row)
	if m == nil {
		s.t.Fatalf("no next step on My courses: %s", row)
	}
	gotHref := ""
	if h := hrefAttr.FindStringSubmatch(m[1]); h != nil {
		gotHref = html.UnescapeString(h[1])
	}
	if m[2] != label || gotHref != href {
		s.t.Fatalf("My courses next step = %q to %q, want %q to %q", m[2], gotHref, label, href)
	}
	expectAll(s.t, row, wants...)
}

func TestMyCoursesShowsEachCoursesProgressNextStepAndExamStatus(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables", "Functions")
	lesson := func(title string) string { return "/courses/" + id + "/lessons/" + s.lessonIDByTitle(title) }
	exam := "/courses/" + id + "/exam"

	s.expectMyCourse(id, "0 of 2 Lessons", "Start Lesson 1", lesson("Variables"), "Go Basics", `>Not taken</span>`)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.expectMyCourse(id, "1 of 2 Lessons", "Continue to Lesson 2", lesson("Functions"))
	s.markLesson(id, s.lessonIDByTitle("Functions"), true)
	s.expectMyCourse(id, "2 of 2 Lessons", "Take the Exam", exam, `>Not taken</span>`)

	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.expectMyCourse(id, "2 of 2 Lessons", "Continue the Exam", exam, `>In progress</span>`)
	s.answerAll(attempt, "1") // wrong
	s.expectMyCourse(id, "2 of 2 Lessons", "Awaiting Result", exam, `>Awaiting Result</span>`)
	s.releaseResults()
	s.expectMyCourse(id, "2 of 2 Lessons", "Retake the Exam", exam, `>Failed</span>`)

	if strings.Contains(s.myCourses(), "Certificates earned") {
		t.Fatal("My courses lists Certificates earned before any is issued")
	}
	s.sitExam(id, "0") // passed, and the Certificate is issued
	s.expectMyCourse(id, "2 of 2 Lessons", "View your Certificate", "/my-certificates", `>Passed</span>`)
	expectInOrder(t, s.myCourses(), "Certificates earned", "Go Basics", "/certificates/")

	_, body := s.getAsking("/my-courses", "ms")
	expectAll(t, mainOf(body), ">Lulus</span>", "Sijil yang diperoleh")
}

func TestMyCoursesSaysCompleteForACourseWithNothingLeftAndShowsNoProgressBarWithoutLessons(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(false)
	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", examPassword) // no IC, so no Certificate
	s.post("/courses/"+id+"/join", nil)
	s.expectMyCourse(id, "", "Course complete", "")
}

func TestMyCoursesWithoutCoursesPointsToTheCatalogue(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	expectInOrder(t, s.myCourses(), `class="empty-state`, "You have not joined any Courses yet.",
		"Join a Course and", `href="/courses"`, "Browse the Courses")
	_, body := s.getAsking("/my-courses", "ms")
	expectAll(t, mainOf(body), "Anda belum menyertai mana-mana Kursus.", "Sertai Kursus")
}

// The Learner's own pages are named in sentence case everywhere, as the header and footer already name them.
func TestTheLearnersOwnPagesAreNamedInSentenceCase(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	for _, c := range []struct{ path, en, ms string }{
		{"/my-courses", "My courses", "Kursus saya"},
		{"/my-certificates", "My certificates", "Sijil saya"},
		{"/my-exams", "My exams", "Peperiksaan saya"},
	} {
		for lang, name := range map[string]string{"en": c.en, "ms": c.ms} {
			_, body := s.getAsking(c.path, lang)
			expectAll(t, body, "<title>"+name+" · CAVELMS</title>", "<h1>"+name+"</h1>")
		}
	}
	for lang, banned := range map[string][]string{"en": {"My Courses", "My Certificates"}, "ms": {"Kursus Saya", "Sijil Saya"}} {
		for _, path := range []string{"/my-courses", "/my-certificates"} {
			_, body := s.getAsking(path, lang)
			expectNone(t, body, banned...)
		}
	}
}
