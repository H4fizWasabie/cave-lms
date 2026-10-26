package app_test

import (
	"html"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
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

// liveNextStepCourse is the Admin making a published Live Course from start to end with the given text Lessons and, when
// withExam, a one-question Multiple-choice Exam; then Ada, with her IC set, joining it while the clock reads now.
// It returns the Course ID, leaving Ada logged in.
func (s *site) liveNextStepCourse(now *time.Time, start, end, days string, withExam bool, lessons ...string) string {
	s.t.Helper()
	s.application.SetClock(func() time.Time { return *now })
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	for _, l := range lessons {
		s.addTextLesson(id, l, "", "Read this.", false)
	}
	if withExam {
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Which is right?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	copyID := s.copiedCourseID(id, "Go for Acme", start, end, days)
	s.post("/admin/courses/"+copyID+"/publish", nil)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.setICFor("ada@example.com")
	s.post("/courses/"+copyID+"/join", nil)
	return copyID
}

func TestALiveCourseWithNothingToDoBeforeItsStartDateSaysWhenItStarts(t *testing.T) {
	now := time.Date(2027, 1, 31, 15, 59, 0, 0, time.UTC) // 23:59 the day before the start date in Malaysia
	s := newSite(t)
	id := s.liveNextStepCourse(&now, "2027-02-01", "2027-02-03", "2", false)

	s.expectNextStep(id, "Starts 1 February 2027", "")
	s.expectMyCourse(id, "", "Starts 1 February 2027", "")
	_, body := s.getAsking("/courses/"+id, "ms")
	expectAll(t, body, "Bermula 1 Februari 2027")
	expectNone(t, body, "Kursus selesai")
}

func TestALiveCourseWithNothingToDoBetweenItsDatesSaysWhenItRuns(t *testing.T) {
	now := time.Date(2027, 1, 31, 16, 0, 0, 0, time.UTC) // midnight on the start date in Malaysia
	s := newSite(t)
	id := s.liveNextStepCourse(&now, "2027-02-01", "2027-02-03", "2", false, "Variables")
	label, lesson := s.nextStepOn(id) // the copy's Lesson, not the original's
	if label != "Start Lesson 1" {
		t.Fatalf("next step = %q, want the Lesson first", label)
	}

	s.markLesson(id, lesson[strings.LastIndex(lesson, "/")+1:], true)
	s.expectNextStep(id, "Runs 1 February 2027–3 February 2027", "")
	_, body := s.get(lesson)
	expectAll(t, body, `id="next-step" class="mc-done step-upcoming">Runs 1 February 2027–3 February 2027<`)
	now = time.Date(2027, 2, 3, 15, 59, 0, 0, time.UTC) // 23:59 on the end date in Malaysia
	s.expectMyCourse(id, "1 of 1 Lesson", "Runs 1 February 2027–3 February 2027", "")
	_, body = s.getAsking("/courses/"+id, "ms")
	expectAll(t, body, "Berlangsung 1 Februari 2027–3 Februari 2027")
}

func TestASingleDayLiveCourseWithNothingToDoOnTheDayShowsThatDateOnly(t *testing.T) {
	now := time.Date(2027, 2, 1, 1, 0, 0, 0, time.UTC)
	s := newSite(t)
	id := s.liveNextStepCourse(&now, "2027-02-01", "2027-02-01", "1", false)

	s.expectNextStep(id, "Runs 1 February 2027", "")
	s.expectMyCourse(id, "", "Runs 1 February 2027", "")
}

func TestALiveCourseWithNothingToDoIsCompleteAfterItsEndDate(t *testing.T) {
	now := time.Date(2027, 2, 2, 9, 0, 0, 0, time.UTC)
	s := newSite(t)
	id := s.liveNextStepCourse(&now, "2027-02-01", "2027-02-03", "2", false)

	now = time.Date(2027, 2, 3, 16, 0, 0, 0, time.UTC) // midnight after the end date in Malaysia
	s.expectNextStep(id, "Course complete", "")
}

func TestALiveCourseWithAnOpenExamStillShowsTheExam(t *testing.T) {
	now := time.Date(2027, 1, 20, 9, 0, 0, 0, time.UTC)
	s := newSite(t)
	id := s.liveNextStepCourse(&now, "2027-02-01", "2027-02-03", "2", true)
	s.expectNextStep(id, "Starts 1 February 2027", "") // its Exam can't be taken before the start date

	now = time.Date(2027, 2, 2, 9, 0, 0, 0, time.UTC)
	s.expectNextStep(id, "Take the Exam", "/courses/"+id+"/exam")
	s.expectMyCourse(id, "", "Take the Exam", "/courses/"+id+"/exam")
}

var solidTag = regexp.MustCompile(`<(?:a|button)\b([^>]*\bclass="[^"]*\bbtn-solid\b[^"]*"[^>]*)>\s*([^<]+?)\s*<`)

// primaries are the primary buttons in a page's own content, each as "label > href", or the label alone for a button.
func primaries(body string) []string {
	var got []string
	for _, m := range solidTag.FindAllStringSubmatch(mainOf(body), -1) {
		p := m[2]
		if h := hrefAttr.FindStringSubmatch(m[1]); h != nil {
			p += " > " + html.UnescapeString(h[1])
		}
		got = append(got, p)
	}
	return got
}

// expectPrimaries checks a page's primary buttons.
func expectPrimaries(t *testing.T, body string, want ...string) {
	t.Helper()
	if got := primaries(body); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("primary buttons = %q, want %q: %s", got, want, mainOf(body))
	}
}

// sitExamAt is Ada sitting the Course's Exam answering slot to every question, its Result then released; it
// returns the attempt's path, where its Result shows.
func (s *site) sitExamAt(courseID, slot string) string {
	s.t.Helper()
	res, body := s.startExam(courseID)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	attempt := res.Header.Get("Location")
	s.answerAll(attempt, slot)
	s.releaseResults()
	return attempt
}

func TestAPassedResultLeadsToTheCertificate(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	attempt := s.sitExamAt(id, "0") // passed, and the Certificate is issued

	_, mine := s.get("/my-certificates")
	pdf := "/certificates/" + certificateIDs(mine)[0] + "/pdf"
	_, body := s.get(attempt)
	expectAll(t, body, "Congratulations, you passed (pass mark 50%)", "Back to the Exam")
	expectPrimaries(t, body, "View your Certificate > "+pdf)
	_, body = s.getAsking(attempt, "ms")
	expectPrimaries(t, body, "Lihat Sijil anda > "+pdf)
}

func TestAfterAPassTheExamPageLeadsToTheCertificateWithTheAnswerSheetSecond(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	s.sitExamAt(id, "0")

	_, body := s.get("/courses/" + id + "/exam")
	expectPrimaries(t, body, "View your Certificate > /my-certificates")
	expectAll(t, body, `class="btn btn-ghost btn-lg" href="/courses/`+id+`/exam/answer-sheet">Download your Answer sheet`)
}

func TestAFailedResultOffersTheRetakeWhileAttemptsRemainAndOtherwiseOnlyTheWayBack(t *testing.T) {
	s := newSite(t)
	id := s.nextStepCourse(true, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	_, body := s.get(s.sitExamAt(id, "1")) // failed, one attempt left
	expectAll(t, body, "Failed (pass mark 50%)", "Back to the Exam")
	expectPrimaries(t, body, "Retake the Exam > /courses/"+id+"/exam")

	_, body = s.get(s.sitExamAt(id, "1")) // failed, no attempts left
	expectAll(t, body, "Failed (pass mark 50%)", "Back to the Exam")
	expectPrimaries(t, body)
	expectNone(t, mainOf(body), "/my-certificates")
}
