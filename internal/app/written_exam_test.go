package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// writtenExam is the address of a Course's Written Exam Admin page.
func writtenExam(courseID string) string {
	return "/admin/courses/" + courseID + "/exam?format=written"
}

// saveWrittenQuestion posts a new or edited Written question to path, which names the Written Exam.
func (s *site) saveWrittenQuestion(path, prompt, modelAnswer string) (*http.Response, string) {
	return s.post(path, url.Values{"prompt": {prompt}, "model_answer": {modelAnswer}})
}

func TestACourseHasAWrittenExamWithHandWrittenQuestionsThatGoToTheBank(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Which is right?", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)

	_, body := s.get("/admin/courses/" + id)
	expectAll(t, body, `href="/admin/courses/`+id+`/exam?format=written"`)
	res, body := s.get(writtenExam(id))
	expectStatus(t, res, body, http.StatusOK, "Questions per attempt: 10")
	expectAll(t, body, "Written")
	expectNone(t, body, "Which is right?")

	res, body = s.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the model answer.")
	res, body = s.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != writtenExam(id) {
		t.Fatalf("saving a Written question went to %q, want %q", loc, writtenExam(id))
	}
	res, body = s.get(writtenExam(id))
	expectStatus(t, res, body, http.StatusOK, "Explain :=")
	expectAll(t, body, "It declares and assigns.", "Ready for Learners.")
	_, body = s.get("/admin/courses/" + id + "/exam")
	expectNone(t, body, "Explain :=")

	_, body = s.get("/admin/question-bank?format=written")
	expectAll(t, body, "Explain :=", "It declares and assigns.", "Go Basics")
	expectNone(t, body, "Which is right?")

	// Editing either copy keeps its model answer apart from the other's.
	bankID := bankQuestionAction.FindStringSubmatch(body)[1]
	_, body = s.get("/admin/question-bank/" + bankID + "/edit")
	expectAll(t, body, `name="model_answer"`, "It declares and assigns.")
	res, body = s.post("/admin/question-bank/"+bankID, url.Values{"topic": {"Go"}, "language": {"en"}, "prompt": {"Explain := in Go"}, "model_answer": {"Short declaration."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(writtenExam(id))
	qid := examQuestionID.FindStringSubmatch(body)[1]
	expectAll(t, body, "It declares and assigns.")
	expectNone(t, body, "Short declaration.")
	res, body = s.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions/"+qid+"?format=written", "Explain :=", "Declares a variable.")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(writtenExam(id))
	expectAll(t, body, "Declares a variable.")
	_, body = s.get("/admin/question-bank?format=written")
	expectAll(t, body, "Explain := in Go", "Short declaration.")

	// Saving the Written Exam's settings keeps it apart from the Multiple-choice one.
	res, body = s.post(writtenExam(id), url.Values{"pass_mark": {"60"}, "time_limit": {""}, "max_attempts": {""}, "draw_count": {"10"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(writtenExam(id))
	expectAll(t, body, "Pass mark: 60%")
	_, body = s.get("/admin/courses/" + id + "/exam")
	expectAll(t, body, "Pass mark: 50%", "Questions per attempt: 30")
}

// writtenCourse makes a Course with one text Lesson and a Written Exam with the pass mark and one published question
// per prompt, whose model answer is "Model answer to " and the prompt, then logs in as Ada, a Learner who has set her
// IC, joined and completed the Lesson. It returns the Course ID.
func (s *site) writtenCourse(passMark string, prompts ...string) string {
	s.t.Helper()
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.post(writtenExam(id), url.Values{"pass_mark": {passMark}, "time_limit": {""}, "max_attempts": {""}, "draw_count": {"10"}})
	for _, p := range prompts {
		res, body := s.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", p, "Model answer to "+p)
		expectStatus(s.t, res, body, http.StatusSeeOther, "")
	}
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", examPassword)
	s.setICFor("ada@example.com")
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	return id
}

// sitWritten starts the logged-in Learner's attempt at the Written Exam and submits answer to every question. It
// returns the attempt's path and its question IDs in the order asked.
func (s *site) sitWritten(courseID, answer string) (string, []string) {
	s.t.Helper()
	res, body := s.post("/courses/"+courseID+"/exam/attempts?format=written", nil)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	attempt := res.Header.Get("Location")
	_, body = s.get(attempt)
	qids := attemptQuestionIDs(body)
	form := url.Values{"confirm": {"yes"}} // a blank answer is submitted without asking
	for _, qid := range qids {
		form.Set("answer-"+qid, answer)
	}
	res, body = s.post(strings.Replace(attempt, "?", "/submit?", 1), form)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return attempt, qids
}

func TestAWrittenAttemptDrawsTenQuestionsAndTakesTextAnswersWithoutShowingModelAnswers(t *testing.T) {
	s := newSite(t)
	var prompts []string
	for i := 1; i <= 12; i++ {
		prompts = append(prompts, fmt.Sprintf("Written question %02d", i))
	}
	id := s.writtenCourse("50", prompts...)

	_, body := s.get("/my-exams")
	expectAll(t, body, `href="/courses/`+id+`/exam?format=written"`, "Written")
	_, body = s.get("/courses/" + id)
	expectAll(t, body, `href="/courses/`+id+`/exam?format=written"`)
	res, body := s.post("/courses/"+id+"/exam/attempts?format=written", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	attempt := res.Header.Get("Location")
	if !strings.HasSuffix(attempt, "?format=written") {
		t.Fatalf("the attempt is at %q, want the Written Exam's", attempt)
	}
	_, body = s.get(attempt)
	qids := attemptQuestionIDs(body)
	if len(qids) != 10 || strings.Count(body, "<textarea") != 10 {
		t.Fatalf("a Written attempt asks %d questions in %d text boxes, want 10: %s", len(qids), strings.Count(body, "<textarea"), body)
	}
	expectNone(t, body, "Model answer", `type="radio"`)

	form := url.Values{}
	for _, qid := range qids {
		form.Set("answer-"+qid, "My own words for "+qid)
	}
	res, body = s.post(strings.Replace(attempt, "?", "/submit?", 1), form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(attempt)
	expectAll(t, body, "Your answers are in")
	expectNone(t, body, "Model answer", "My own words")

	_, body = s.adminSite().get("/admin/results/" + attemptID(strings.TrimSuffix(attempt, "?format=written")))
	expectAll(t, body, "Written", "My own words for "+qids[0], "Model answer to Written question")
}

// saveMarks is the Admin posting marks on an attempt, keyed by question ID.
func (s *site) saveMarks(attempt string, marks map[string]string) (*http.Response, string) {
	form := url.Values{}
	for qid, m := range marks {
		form.Set("mark-"+qid, m)
	}
	return s.post("/admin/results/"+writtenAttemptID(attempt)+"/marks", form)
}

// writtenAttemptID is the ID in a Written attempt's path.
func writtenAttemptID(attempt string) string {
	return attemptID(strings.TrimSuffix(attempt, "?format=written"))
}

func TestWithTheAIDownAWrittenResultIsReleasedOnlyOnceTheAdminHasMarkedEveryAnswer(t *testing.T) {
	s := newSite(t)
	s.ai.FailMarks(aiwriter.ErrUnavailable)
	id := s.writtenCourse("70", "Explain :=", "Explain go vet")
	s.switchLanguage("en", "/") // saved on Ada's account
	attempt, qids := s.sitWritten(id, "My own words")
	before := len(s.resultEmailsTo("ada@example.com"))
	admin := s.adminSite()
	detail := "/admin/results/" + writtenAttemptID(attempt)

	s.releaseResults()
	if n := len(s.resultEmailsTo("ada@example.com")); n != before {
		t.Fatalf("%d emails before any answer was marked, want %d", n, before)
	}
	_, body := admin.get(detail)
	expectAll(t, body, `name="mark-`+qids[0]+`"`, `name="mark-`+qids[1]+`"`, "Waiting for release")

	for _, bad := range []string{"11", "-1", "7.5", "ten"} {
		res, body := admin.saveMarks(attempt, map[string]string{qids[0]: bad})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter each mark as a whole number from 0 to 10.")
	}
	res, body := admin.saveMarks(attempt, map[string]string{qids[0]: "10"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != detail {
		t.Fatalf("saving marks went to %q, want %q", loc, detail)
	}
	s.releaseResults()
	if n := len(s.resultEmailsTo("ada@example.com")); n != before {
		t.Fatalf("%d emails with one answer still unmarked, want %d", n, before)
	}
	_, body = s.get(attempt)
	expectAll(t, body, "Your Result is ready at")
	expectNone(t, body, "Score")

	admin.saveMarks(attempt, map[string]string{qids[1]: "5"})
	s.runReleaseJob() // the 10 minutes have already passed
	emails := s.resultEmailsTo("ada@example.com")[before:]
	if len(emails) != 1 {
		t.Fatalf("%d Result emails, want 1", len(emails))
	}
	expectEmail(t, emails[0], "Your Written Exam Result for Go Basics", "Score: 75%")
	expectAll(t, emails[0].Body, "Passed (pass mark 70%)")
	if len(emails[0].Attachments) != 1 {
		t.Fatalf("the pass email carries %d attachments, want the Answer sheet", len(emails[0].Attachments))
	}
	expectAll(t, string(emails[0].Attachments[0].Content), "Exam: Written", "Score: 75%", "Explain :=",
		"Your answer: My own words", "Model answer: Model answer to Explain :=", "Mark: 10 / 10", "Mark: 5 / 10")
	expectNone(t, emails[0].Body, "Model answer", "My own words")

	_, body = admin.get("/admin/results")
	expectCells(t, resultRow(t, body, writtenAttemptID(attempt)), "Written", "75%", "Passed", "Admin")
	_, body = s.get("/courses/" + id + "/exam?format=written")
	expectAll(t, body, `href="/courses/`+id+`/exam/answer-sheet?format=written"`)
}

func TestAWrittenAttemptWithNoAnswersNeedsNoMarking(t *testing.T) {
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=")
	attempt, _ := s.sitWritten(id, "")
	s.releaseResults()
	res, body := s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, `class="x-score">0%<`)
	expectAll(t, body, "Failed (pass mark 50%)")
	expectNone(t, body, "Model answer")
	expectNone(t, s.lastEmailTo("ada@example.com").Body, "Model answer", "Jawapan contoh")
	res, body = s.get("/courses/" + id + "/exam/answer-sheet?format=written")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestChangingAMarkAfterReleaseRecalculatesTheResultAndEmailsOnlyWhenPassOrFailFlips(t *testing.T) {
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=", "Explain go vet")
	s.switchLanguage("en", "/") // saved on Ada's account
	attempt, qids := s.sitWritten(id, "My own words")
	admin := s.adminSite()
	admin.saveMarks(attempt, map[string]string{qids[0]: "2", qids[1]: "4"})
	s.releaseResults()
	expectEmail(t, s.lastEmailTo("ada@example.com"), "Your Written Exam Result for Go Basics", "Score: 30%")
	sent := len(s.resultEmailsTo("ada@example.com"))

	admin.saveMarks(attempt, map[string]string{qids[0]: "3"}) // still a fail
	_, body := s.get(attempt)
	expectAll(t, body, `class="x-score">35%<`, "Failed (pass mark 50%)")
	if n := len(s.resultEmailsTo("ada@example.com")); n != sent {
		t.Fatalf("%d emails after a change that kept the fail, want %d", n, sent)
	}

	admin.saveMarks(attempt, map[string]string{qids[0]: "10"}) // now a pass
	emails := s.resultEmailsTo("ada@example.com")[sent:]
	if len(emails) != 1 {
		t.Fatalf("%d emails after the fail became a pass, want 1", len(emails))
	}
	expectEmail(t, emails[0], "Your corrected Written Exam Result for Go Basics", "Score: 70%")
	expectAll(t, emails[0].Body, "Passed (pass mark 50%)")
	if len(emails[0].Attachments) != 1 {
		t.Fatalf("the corrected pass email carries %d attachments, want the Answer sheet", len(emails[0].Attachments))
	}
	res, body := s.get("/courses/" + id + "/exam/answer-sheet?format=written")
	expectStatus(t, res, body, http.StatusOK, "Mark: 10 / 10")

	admin.saveMarks(attempt, map[string]string{qids[0]: "0"}) // back to a fail
	emails = s.resultEmailsTo("ada@example.com")[sent:]
	if len(emails) != 2 {
		t.Fatalf("%d emails after the pass became a fail, want 2", len(emails))
	}
	expectEmail(t, emails[1], "Your corrected Written Exam Result for Go Basics", "Score: 20%")
	if len(emails[1].Attachments) != 0 {
		t.Fatalf("the corrected fail email carries %d attachments, want none", len(emails[1].Attachments))
	}
	_, body = admin.get("/admin/results")
	expectCells(t, resultRow(t, body, writtenAttemptID(attempt)), "20%", "Failed", "Admin")
}

func TestAnAdminMarkThatTurnsAFailIntoAPassIssuesTheCertificateItEarns(t *testing.T) {
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=")
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 2, Reason: "Too vague."}})
	attempt, qids := s.sitWritten(id, "My own words")
	s.releaseResults()
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")

	s.adminSite().saveMarks(attempt, map[string]string{qids[0]: "10"})
	_, body = s.get("/my-certificates")
	if ids := certificateIDs(body); len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates after the pass, want 1: %s", len(ids), body)
	}
}
