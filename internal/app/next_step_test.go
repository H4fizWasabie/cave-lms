package app_test

import (
	"html"
	"net/http"
	"regexp"
	"testing"
)

var nextStepTag = regexp.MustCompile(`id="next-step"([^>]*)>\s*([^<]+?)\s*<`)
var hrefAttr = regexp.MustCompile(`href="([^"]*)"`)

// nextStepOn reads the Course page's next step: its label and where it leads, "" for a step that leads nowhere.
func (s *site) nextStepOn(courseID string) (label, href string) {
	s.t.Helper()
	res, body := s.get("/courses/" + courseID)
	expectStatus(s.t, res, body, http.StatusOK, "")
	m := nextStepTag.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no next step on the Course page: %s", content(body))
	}
	if h := hrefAttr.FindStringSubmatch(m[1]); h != nil {
		href = html.UnescapeString(h[1])
	}
	return m[2], href
}

// expectNextStep checks the Course page's next step.
func (s *site) expectNextStep(courseID, label, href string) {
	s.t.Helper()
	if gotLabel, gotHref := s.nextStepOn(courseID); gotLabel != label || gotHref != href {
		s.t.Fatalf("next step = %q to %q, want %q to %q", gotLabel, gotHref, label, href)
	}
}

// nextStepCourse is the Admin making "Go Basics" with the given published text Lessons and, when withExam, a
// Multiple-choice Exam of one question (right answer slot 0, pass mark 50, two attempts); then Ada, with her IC
// set, joining it. It returns the Course ID, leaving Ada logged in.
func (s *site) nextStepCourse(withExam bool, lessons ...string) string {
	s.t.Helper()
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	for _, l := range lessons {
		s.addTextLesson(id, l, "", "Read this.", false)
	}
	if withExam {
		s.saveExamSettings(id, "50", "", "2")
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Which is right?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.setICFor("ada@example.com")
	s.post("/courses/"+id+"/join", nil)
	return id
}

func TestTheNextStepIsTheFirstLessonNotDoneAndNeverTheExamWhileLessonsRemain(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables", "Functions", "Loops")
	lesson := func(title string) string { return "/courses/" + id + "/lessons/" + s.lessonIDByTitle(title) }

	s.expectNextStep(id, "Start Lesson 1", lesson("Variables"))

	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.expectNextStep(id, "Continue to Lesson 2", lesson("Functions"))

	s.markLesson(id, s.lessonIDByTitle("Loops"), true) // out of order: Functions is still the first not done
	s.expectNextStep(id, "Continue to Lesson 2", lesson("Functions"))
}

// answerAll submits the open attempt at attemptPath answering slot to every question.
func (s *site) answerAll(attemptPath, slot string) {
	s.t.Helper()
	_, body := s.get(attemptPath)
	answers := map[string]string{}
	for _, qid := range attemptQuestionIDs(body) {
		answers[qid] = slot
	}
	s.submitExam(attemptPath, answers)
}

func TestTheNextStepGoesFromTheExamToAwaitingTheResultToTheCertificate(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables")
	exam := "/courses/" + id + "/exam"
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.expectNextStep(id, "Take the Exam", exam)

	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.expectNextStep(id, "Continue the Exam", exam)

	s.answerAll(attempt, "1") // wrong
	s.expectNextStep(id, "Awaiting Result", exam)

	s.releaseResults() // failed, one attempt left
	s.expectNextStep(id, "Retake the Exam", exam)
	_, body := s.getAsking("/courses/"+id, "ms")
	expectAll(t, body, "Ambil semula Peperiksaan")

	s.sitExam(id, "0") // passed, and the Certificate is issued
	s.expectNextStep(id, "View your Certificate", "/my-certificates")
}

func TestTheNextStepSkipsAPendingExamForTheNextOneThenAwaitsItsResult(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables")
	s.adminSite().saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	res, _ := s.startExam(id)
	s.answerAll(res.Header.Get("Location"), "0") // Multiple-choice Result pending
	s.expectNextStep(id, "Take the Written Exam", "/courses/"+id+"/exam?format=written")

	s.sitWritten(id, "It declares and assigns.") // Written Result pending too
	s.expectNextStep(id, "Awaiting Result", "/courses/"+id+"/exam")
}

func TestACourseWithNoExamEndsAtTheCertificateOrCompleteWithoutAnIC(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(false, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.expectNextStep(id, "View your Certificate", "/my-certificates")

	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", examPassword) // no IC, so no Certificate
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.expectNextStep(id, "Course complete", "")
}
