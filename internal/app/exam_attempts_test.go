package app_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

const examPassword = "correct horse battery"

var answerField = regexp.MustCompile(`name="answer-(\d+)"`)

// examQuestionIDs lists the Exam question IDs of a Course in position order, Drafts included. Admin only.
func (s *site) examQuestionIDs(courseID string) []string {
	s.t.Helper()
	_, body := s.get("/admin/courses/" + courseID + "/exam")
	var ids []string
	for _, m := range examQuestionID.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// examCourse makes a Course with one text Lesson, an Exam with the given settings and one published question per
// prompt, then logs in as a Learner who has set her IC, joined and completed the Lesson. It returns the Course ID and the
// question IDs. Each question's correct option is "Right answer" (slot 0), its wrong option "Wrong answer" (slot 1).
func (s *site) examCourse(passMark, timeLimit, maxAttempts string, prompts ...string) (string, []string) {
	s.t.Helper()
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.saveExamSettings(id, passMark, timeLimit, maxAttempts)
	for _, p := range prompts {
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", p, [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	qids := s.examQuestionIDs(id)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.setICFor("ada@example.com")
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	return id, qids
}

// startExam starts an attempt and returns the response; on success Location is the attempt's path.
func (s *site) startExam(courseID string) (*http.Response, string) {
	return s.post("/courses/"+courseID+"/exam/attempts", nil)
}

// submitExam posts answers keyed by question ID; the value is the option slot picked. Any question left out is
// submitted blank, confirmed (submitting blanks is otherwise asked about first).
func (s *site) submitExam(attemptPath string, answers map[string]string) (*http.Response, string) {
	form := url.Values{"confirm": {"yes"}}
	for qid, slot := range answers {
		form.Set("answer-"+qid, slot)
	}
	return s.post(attemptPath+"/submit", form)
}

// attemptQuestionIDs returns the question IDs on an attempt page in the order shown.
func attemptQuestionIDs(body string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range answerField.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}

// ageAttempt moves an attempt's start back by the given seconds, straight in the database.
func (s *site) ageAttempt(attemptPath string, seconds int) {
	s.t.Helper()
	parts := strings.Split(attemptPath, "/")
	if _, err := s.db.Exec(`UPDATE exam_attempts SET started_at = now() - make_interval(secs => $2) WHERE id = $1`,
		parts[len(parts)-1], seconds); err != nil {
		s.t.Fatal(err)
	}
}

func TestExamIsRefusedUntilEveryLessonIsComplete(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.addTextLesson(id, "Functions", "", "Use func.", false)
	s.saveExamSettings(id, "50", "", "")
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "What is Go?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Complete every Lesson before you sit the Exam (1 of 2 complete).")
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "Complete every Lesson before you sit the Exam (1 of 2 complete).")

	s.markLesson(id, s.lessonIDByTitle("Functions"), true)
	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Start the Exam")
}

func TestExamPageShowsTimeLimitPassMarkAndAttemptsLeft(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("70", "30", "3", "What is Go?")

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Pass mark: 70%")
	expectStatus(t, res, body, http.StatusOK, "Time limit: 30 minutes")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 3")
	expectStatus(t, res, body, http.StatusOK, "Start the Exam")
}

func TestTheExamFactsShowEachValueOverItsBareLabelAndSayBothOnce(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("70", "30", "3", "What is Go?")

	_, body := s.getAsking("/courses/"+id+"/exam", "en")
	expectInOrder(t, mainOf(body),
		`<span aria-hidden="true">Pass mark</span><span class="sr">Pass mark: 70%</span>`,
		`<span aria-hidden="true">Time limit</span><span class="sr">Time limit: 30 minutes</span>`,
		`<span aria-hidden="true">Attempts left</span><span class="sr">Attempts left: 3</span>`,
		`<span aria-hidden="true">Best score</span><span class="sr">No score yet</span>`)
	_, body = s.getAsking("/courses/"+id+"/exam", "ms")
	expectInOrder(t, mainOf(body), `<span aria-hidden="true">Soalan</span>`, `<span aria-hidden="true">Markah lulus</span>`)
}

func TestDraftQuestionsAreNeverAsked(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Published question")
	s.logOut()
	s.login("admin@example.com", examPassword)
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Secret draft question", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	s.logOut()
	s.login("ada@example.com", examPassword)

	res, _ := s.startExam(id)
	res, body := s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Published question")
	if strings.Contains(body, "Secret draft question") {
		t.Fatalf("attempt asks a Draft question: %s", body)
	}
}

func TestAttemptAsksEveryPublishedQuestionWhenThereAreFewerThanTheDrawCount(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Q1", "Q2", "Q3", "Q4", "Q5", "Q6")

	res, _ := s.startExam(id)
	res, body := s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Q1")

	shown := attemptQuestionIDs(body)
	want := append([]string(nil), qids...)
	got := append([]string(nil), shown...)
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("attempt shows questions %v, want every published question %v", shown, qids)
	}
}

// drawExam is examCourse with ten published questions Q1 to Q10, one Draft and a draw count of 4. It returns the
// Course ID and the published question IDs.
func (s *site) drawExam() (string, []string) {
	s.t.Helper()
	id, qids := s.examCourse("100", "", "3", "Q1", "Q2", "Q3", "Q4", "Q5", "Q6", "Q7", "Q8", "Q9", "Q10")
	s.logOut()
	s.login("admin@example.com", examPassword)
	s.saveExamDraw(id, "100", "", "3", "4")
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Draft question", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	s.logOut()
	s.login("ada@example.com", examPassword)
	return id, qids
}

func TestFirstAttemptAsksTheDrawCountFromPublishedQuestionsOnly(t *testing.T) {
	s := newSite(t)
	id, qids := s.drawExam()

	res, _ := s.startExam(id)
	res, body := s.get(res.Header.Get("Location"))
	shown := attemptQuestionIDs(body)
	if len(shown) != 4 {
		t.Fatalf("attempt asks %d questions %v, want the draw count of 4", len(shown), shown)
	}
	published := strings.Join(qids, ",") + ","
	for _, q := range shown {
		if !strings.Contains(","+published, ","+q+",") {
			t.Fatalf("attempt asks question %s, which is not a published question %v", q, qids)
		}
	}
	if strings.Contains(body, "Draft question") {
		t.Fatalf("attempt asks a Draft question: %s", body)
	}
}

func TestLaterAttemptsAskTheFirstAttemptsQuestionsInTheSameOrder(t *testing.T) {
	s := newSite(t)
	id, _ := s.drawExam()
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	_, body := s.get(first)
	want := attemptQuestionIDs(body)
	s.submitExam(first, nil)
	s.releaseResults()

	for range 2 {
		res, _ = s.startExam(id)
		retry := res.Header.Get("Location")
		_, body = s.get(retry)
		if got := attemptQuestionIDs(body); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("later attempt asks %v, want the first attempt's %v", got, want)
		}
		s.submitExam(retry, nil)
		s.releaseResults()
	}
}

func TestARetryShowsPoolEditsAndStillAsksAQuestionDeletedFromThePool(t *testing.T) {
	s := newSite(t)
	id, qids := s.drawExam()
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	_, body := s.get(first)
	drawn := attemptQuestionIDs(body)
	s.submitExam(first, nil)
	s.releaseResults()
	deletedPrompt := "</span>Q" + strconv.Itoa(slices.Index(qids, drawn[1])+1) + "</legend>" // after its "Question n of N"

	s.logOut()
	s.login("admin@example.com", examPassword)
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions/"+drawn[0], "Edited prompt", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.post(examPath+"/questions/"+drawn[1]+"/delete", nil)
	s.logOut()
	s.login("ada@example.com", examPassword)

	res, _ = s.startExam(id)
	retry := res.Header.Get("Location")
	res, body = s.get(retry)
	expectStatus(t, res, body, http.StatusOK, "</span>Edited prompt</legend>")
	expectStatus(t, res, body, http.StatusOK, deletedPrompt)
	if got := attemptQuestionIDs(body); strings.Join(got, ",") != strings.Join(drawn, ",") {
		t.Fatalf("retry asks %v, want the first attempt's %v", got, drawn)
	}

	answers := map[string]string{}
	for _, q := range drawn {
		answers[q] = "0"
	}
	s.submitExam(retry, answers)
	s.releaseResults()
	res, body = s.get(retry)
	expectStatus(t, res, body, http.StatusOK, "Score: 100%")
}

func TestAReleasedResultShowsOnlyTheMarkAndPassOrFail(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Right one", "Wrong one")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")

	res, body := s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.releaseResults()
	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 50%")
	expectAll(t, body, "Passed (pass mark 50%)")
	expectNone(t, content(body), "Right one", "Wrong one", "Right answer", "Wrong answer", "Your answer")
}

func TestTimeLimitIsEnforcedOnTheServer(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "1", "", "Q1")

	// Two minutes in, past the one-minute limit and its grace period: the answers do not count.
	res, _ := s.startExam(id)
	late := res.Header.Get("Location")
	s.ageAttempt(late, 120)
	res, body := s.submitExam(late, map[string]string{qids[0]: "0"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(late)
	expectStatus(t, res, body, http.StatusOK, "Time ran out before your answers were sent, so none of them count.")
	s.releaseResults()
	res, body = s.get(late)
	expectStatus(t, res, body, http.StatusOK, "Score: 0%")

	// 70 seconds in is inside the grace period after the one-minute limit: the answers count.
	res, _ = s.startExam(id)
	inGrace := res.Header.Get("Location")
	s.ageAttempt(inGrace, 70)
	s.submitExam(inGrace, map[string]string{qids[0]: "0"})
	s.releaseResults()
	res, body = s.get(inGrace)
	expectStatus(t, res, body, http.StatusOK, "Score: 100%")
}

func TestFailedLearnerRetakesWhileAttemptsRemainAndAttemptsAreLimited(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "", "2", "Q1")

	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	s.submitExam(first, map[string]string{qids[0]: "1"})
	s.releaseResults()

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 1")
	res, _ = s.startExam(id)
	second := res.Header.Get("Location")
	if second == "" || second == first {
		t.Fatalf("retake did not start a new attempt: status %d, Location %q", res.StatusCode, second)
	}
	s.submitExam(second, map[string]string{qids[0]: "1"})
	s.releaseResults()

	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 0")
	expectStatus(t, res, body, http.StatusOK, "You have no attempts left.")
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "You have no attempts left.")
}

func TestUnlimitedAttemptsWhenNoMaximumIsSet(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "", "", "Q1")
	for i := 0; i < 3; i++ {
		res, _ := s.startExam(id)
		s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})
		s.releaseResults()
	}
	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: Unlimited")
	expectStatus(t, res, body, http.StatusOK, "Start the Exam")
}

func TestPassedLearnerCannotRetakeAndSeesTheirBestScore(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	s.releaseResults()

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "You have passed this Exam.")
	expectStatus(t, res, body, http.StatusOK, "Best score: 100%")
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "You have passed this Exam.")
}

func TestLaterExamEditsDoNotChangePastAttempts(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Original prompt")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "1"})

	// The edits land before the Result is released, and still do not change how it is marked.
	s.logOut()
	s.login("admin@example.com", examPassword)
	examPath := "/admin/courses/" + id + "/exam"
	s.saveExamSettings(id, "0", "", "")
	s.saveQuestion(examPath+"/questions/"+qids[0], "Edited prompt", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.saveQuestion(examPath+"/questions", "Added later", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	s.post(examPath+"/questions/"+s.examQuestionIDs(id)[1]+"/publish", nil)
	s.logOut()
	s.login("ada@example.com", examPassword)
	s.releaseResults()

	res, body := s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 0%")
	expectStatus(t, res, body, http.StatusOK, "Failed (pass mark 50%)")
}

func TestExamIsForEnrolledLearnersAndTheirOwnAttempts(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")

	other := s.second()
	other.loginAsNewLearner("Bob", "bob@example.com", examPassword)
	res, body := other.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Join this Course to sit its Exam.")
	res, body = other.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "Join this Course to sit its Exam.")
	res, body = other.get(attempt)
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam attempt at this address.")
	res, body = other.submitExam(attempt, map[string]string{qids[0]: "0"})
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam attempt at this address.")

	s.logOut()
	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if want := "/login?next=" + url.QueryEscape("/courses/"+id+"/exam"); res.Header.Get("Location") != want {
		t.Fatalf("visitor redirected to %q, want %q", res.Header.Get("Location"), want)
	}
}

func TestStartingAgainResumesTheOpenAttempt(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "1", "Q1")
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")

	res, _ = s.startExam(id)
	if res.Header.Get("Location") != first {
		t.Fatalf("second start went to %q, want the open attempt %q", res.Header.Get("Location"), first)
	}
	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 0")
	expectStatus(t, res, body, http.StatusOK, "Continue your attempt")

	s.submitExam(first, map[string]string{qids[0]: "1"})
	s.releaseResults()
	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "You have no attempts left.")
}

func TestStartingAnExamNeedsOnlyOneDatabaseConnection(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Q1")
	s.application.SetMaxDBConns(1)
	s.client.Timeout = 5 * time.Second
	res, body := s.startExam(id)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("start status = %d, want %d: %s", res.StatusCode, http.StatusSeeOther, body)
	}
}

func TestCourseWithExamAttemptsCannotBeDeleted(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "What is a slice?")
	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"}) // failed, so no Certificate
	s.leaveCourse(id)
	s.loginAdmin()
	s.post("/admin/lessons/"+s.lessonIDByTitle("Variables")+"/delete", confirming)

	res, body := s.post("/admin/courses/"+id+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict,
		"This Course cannot be deleted while it has Lessons, Learners, Exam attempts or Certificates.")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Basics</h1>")
}

func TestExamWithNoPublishedQuestionsRefusesAStart(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "")
	s.loginAdmin()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Draft only?", [5]string{"Yes", "No", "", "", ""}, "0", true)
	s.logOut()
	s.login("ada@example.com", examPassword)

	res, body := s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "This Exam is not ready yet.")
}

func TestTodaysExamsBecomeMultipleChoiceWithTheirSettingsQuestionsAndAttempts(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("70", "30", "3", "First question text", "Second question text")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	s.releaseResults()
	if err := app.MigrateDownTo(context.Background(), s.db, 75); err != nil {
		t.Fatal(err)
	}
	s.restart()

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Best score: 50%")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 2")
	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 50%")
	var format string
	if err := s.db.QueryRow(`SELECT format FROM exams WHERE course_id = $1`, id).Scan(&format); err != nil || format != "multiple-choice" {
		t.Fatalf("the Exam's format is %q (%v), want multiple-choice", format, err)
	}
	if _, err := s.db.Exec(`INSERT INTO exams (course_id, format) VALUES ($1, 'multiple-choice')`, id); err == nil {
		t.Fatal("a Course took a second Multiple-choice Exam")
	}

	s.logOut()
	s.login("admin@example.com", examPassword)
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Pass mark: 70%")
	expectStatus(t, res, body, http.StatusOK, "Time limit: 30 minutes")
	expectStatus(t, res, body, http.StatusOK, "Attempts allowed: 3")
	expectInOrder(t, body, "First question text", "Second question text")
}
