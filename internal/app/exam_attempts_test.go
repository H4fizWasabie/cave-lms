package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
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
// prompt, then logs in as a Learner who has joined and completed the Lesson. It returns the Course ID and the
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
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	return id, qids
}

// startExam starts an attempt and returns the response; on success Location is the attempt's path.
func (s *site) startExam(courseID string) (*http.Response, string) {
	return s.post("/courses/"+courseID+"/exam/attempts", nil)
}

// submitExam posts answers keyed by question ID; the value is the option slot picked.
func (s *site) submitExam(attemptPath string, answers map[string]string) (*http.Response, string) {
	form := url.Values{}
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

func TestAttemptAsksEveryPublishedQuestionInAShuffledOrder(t *testing.T) {
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

func TestSubmittedAttemptShowsScoreAndWrongQuestionsButNeverTheCorrectAnswers(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Right one", "Wrong one")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")

	res, body := s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 50%")
	expectStatus(t, res, body, http.StatusOK, "Passed")
	expectStatus(t, res, body, http.StatusOK, "Wrong one")
	expectStatus(t, res, body, http.StatusOK, "Your answer: Wrong answer")

	wrong := body[strings.Index(body, "Wrong answers"):]
	wrong = wrong[:strings.Index(wrong, "</ol>")]
	if strings.Contains(wrong, "Right one") {
		t.Fatalf("right question listed as wrong: %s", wrong)
	}
	if strings.Contains(body, "Right answer") {
		t.Fatalf("result shows the correct option text: %s", body)
	}
}

func TestTimeLimitIsEnforcedOnTheServer(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "1", "", "Q1")

	// Two minutes in, past the one-minute limit and its grace period: the answers are not marked.
	res, _ := s.startExam(id)
	late := res.Header.Get("Location")
	s.ageAttempt(late, 120)
	res, body := s.submitExam(late, map[string]string{qids[0]: "0"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(late)
	expectStatus(t, res, body, http.StatusOK, "Time ran out. This attempt was not marked.")
	if strings.Contains(body, "Score:") {
		t.Fatalf("late attempt was marked: %s", body)
	}

	// 70 seconds in is inside the grace period after the one-minute limit: the answers count.
	res, _ = s.startExam(id)
	inGrace := res.Header.Get("Location")
	s.ageAttempt(inGrace, 70)
	res, body = s.submitExam(inGrace, map[string]string{qids[0]: "0"})
	res, body = s.get(inGrace)
	expectStatus(t, res, body, http.StatusOK, "Score: 100%")
}

func TestFailedLearnerRetakesWhileAttemptsRemainAndAttemptsAreLimited(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "", "2", "Q1")

	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	s.submitExam(first, map[string]string{qids[0]: "1"})

	res, body := s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 1")
	res, _ = s.startExam(id)
	second := res.Header.Get("Location")
	if second == "" || second == first {
		t.Fatalf("retake did not start a new attempt: status %d, Location %q", res.StatusCode, second)
	}
	s.submitExam(second, map[string]string{qids[0]: "1"})

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

	s.logOut()
	s.login("admin@example.com", examPassword)
	examPath := "/admin/courses/" + id + "/exam"
	s.saveExamSettings(id, "0", "", "")
	s.saveQuestion(examPath+"/questions/"+qids[0], "Edited prompt", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.saveQuestion(examPath+"/questions", "Added later", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	s.post(examPath+"/questions/"+s.examQuestionIDs(id)[1]+"/publish", nil)
	s.logOut()
	s.login("ada@example.com", examPassword)

	res, body := s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 0%")
	expectStatus(t, res, body, http.StatusOK, "Failed (pass mark 50%)")
	expectStatus(t, res, body, http.StatusOK, "Original prompt")
	if strings.Contains(body, "Edited prompt") || strings.Contains(body, "Added later") {
		t.Fatalf("past attempt shows later Exam edits: %s", body)
	}
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
	s.post("/courses/"+id+"/leave", nil)
	s.loginAdmin()
	s.post("/admin/lessons/"+s.lessonIDByTitle("Variables")+"/delete", nil)

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
