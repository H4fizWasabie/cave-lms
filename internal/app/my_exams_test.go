package app_test

import (
	"net/http"
	"strings"
	"testing"
)

// examOnlyCourse is the Admin making an open Course with one text Lesson and, when passMark is set, an Exam with
// that pass mark and one published question whose right answer is slot 0. It returns the Course ID.
func (s *site) examOnlyCourse(title, passMark string) string {
	s.t.Helper()
	s.createCourse(title, "About "+title+".", "")
	id := s.courseIDByTitle(title)
	s.addTextLesson(id, title+" lesson", "", "Read this.", false)
	if passMark != "" {
		s.saveExamSettings(id, passMark, "", "")
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Which is right?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	return id
}

// joinAndFinish is the logged-in Learner joining a Course and completing its one Lesson.
func (s *site) joinAndFinish(courseID, title string) {
	s.t.Helper()
	s.post("/courses/"+courseID+"/join", nil)
	s.markLesson(courseID, s.lessonIDByTitle(title+" lesson"), true)
}

// sitExam is the logged-in Learner finishing one attempt, answering slot for every question, and its Result being
// released.
func (s *site) sitExam(courseID, slot string) {
	s.t.Helper()
	res, body := s.startExam(courseID)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	path := res.Header.Get("Location")
	_, body = s.get(path)
	answers := map[string]string{}
	for _, qid := range attemptQuestionIDs(body) {
		answers[qid] = slot
	}
	s.submitExam(path, answers)
	s.releaseResults()
}

func TestMyExamsShowsEachEnrolledCoursesExamWithItsStatus(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	fresh := s.examOnlyCourse("Fresh", "60")
	passed := s.examOnlyCourse("Passer", "60")
	failed := s.examOnlyCourse("Failer", "70")
	none := s.examOnlyCourse("Examless", "")
	s.examOnlyCourse("Unjoined", "50")
	s.logOut()

	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	for id, title := range map[string]string{fresh: "Fresh", passed: "Passer", failed: "Failer", none: "Examless"} {
		s.joinAndFinish(id, title)
	}
	s.sitExam(passed, "1")
	s.sitExam(passed, "0")
	s.sitExam(failed, "1")

	res, body := s.getAsking("/my-exams", "en")
	expectStatus(t, res, body, http.StatusOK, "My exams")
	main := content(body)
	expectNone(t, main, "Examless", "Unjoined")
	expectInOrder(t, main, ">Failer<", "Failed", "Best score: 0%", "Pass mark: 70%", `href="/courses/`+failed+`/exam"`)
	expectInOrder(t, main, ">Fresh<", "Not taken", "Pass mark: 60%", `href="/courses/`+fresh+`/exam"`)
	expectInOrder(t, main, ">Passer<", "Passed", "Best score: 100%", "Pass mark: 60%", `href="/courses/`+passed+`/exam"`)
	_, freshRow, _ := strings.Cut(main, ">Fresh<")
	freshRow, _, _ = strings.Cut(freshRow, ">Passer<")
	expectNone(t, freshRow, "Best score")

	_, body = s.getAsking("/my-exams", "ms")
	main = content(body)
	expectAll(t, main, "Peperiksaan saya", "Belum diambil", "Lulus", "Gagal", "Markah terbaik: 100%", "Markah lulus: 60%")
}

func TestMyExamsWithNoExamsSaysSoAndLinksToTheCourses(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	id := s.examOnlyCourse("Examless", "")
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.joinAndFinish(id, "Examless")

	_, body := s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), "None of your Courses has an Exam yet.", `href="/courses">Browse the Courses</a>`)
	expectNone(t, content(body), "Examless")
	_, body = s.getAsking("/my-exams", "ms")
	expectInOrder(t, content(body), "Belum ada Kursus anda yang mempunyai Peperiksaan.", `href="/courses">Lihat senarai Kursus</a>`)
}

func TestMyExamsIsInTheClassroomMenuAndSendsAVisitorToLogIn(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Classroom<", `href="/my-courses"`, `<a href="/my-exams">My exams</a>`, ">Pricing<")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, header(t, body), ">Kelas<", `<a href="/my-exams">Peperiksaan saya</a>`, ">Harga<")

	res, body := s.get("/my-exams")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login?next=%2Fmy-exams" {
		t.Fatalf("Visitor My exams redirected to %q, want /login?next=%%2Fmy-exams", loc)
	}
}
