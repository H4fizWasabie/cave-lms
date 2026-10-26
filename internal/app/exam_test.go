package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var examQuestionAction = regexp.MustCompile(`/exam/questions/(\d+)/`)

// saveExamSettings posts the Exam settings form for a Course, keeping the default draw count of 30.
func (s *site) saveExamSettings(courseID, passMark, timeLimit, maxAttempts string) (*http.Response, string) {
	return s.saveExamDraw(courseID, passMark, timeLimit, maxAttempts, "30")
}

// saveExamDraw posts the Exam settings form for a Course with a draw count.
func (s *site) saveExamDraw(courseID, passMark, timeLimit, maxAttempts, drawCount string) (*http.Response, string) {
	return s.post("/admin/courses/"+courseID+"/exam", url.Values{
		"pass_mark":    {passMark},
		"time_limit":   {timeLimit},
		"max_attempts": {maxAttempts},
		"draw_count":   {drawCount},
	})
}

// saveQuestion posts a new or edited Exam question; the form always sends five option fields.
func (s *site) saveQuestion(path, prompt string, options [5]string, correct string, draft bool) (*http.Response, string) {
	form := url.Values{"prompt": {prompt}, "correct": {correct}}
	for _, o := range options {
		form.Add("option", o)
	}
	if draft {
		form.Set("draft", "on")
	}
	return s.post(path, form)
}

func TestAdminSetsAndChangesTheExamSettings(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.saveExamSettings(id, "70", "30", "3")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Pass mark: 70%")
	expectStatus(t, res, body, http.StatusOK, "Time limit: 30 minutes")
	expectStatus(t, res, body, http.StatusOK, "Attempts allowed: 3")

	res, body = s.saveExamSettings(id, "80", "", "")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Pass mark: 80%")
	expectStatus(t, res, body, http.StatusOK, "No time limit")
	expectStatus(t, res, body, http.StatusOK, "Unlimited attempts")
	if strings.Contains(body, "Pass mark: 70%") {
		t.Fatalf("changed Exam still shows the old pass mark: %s", body)
	}
}

func TestAnExamAsks30QuestionsUnlessTheAdminChangesTheDrawCount(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Questions per attempt: 30")
	res, body = s.saveExamDraw(id, "50", "", "", "12")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Questions per attempt: 12")

	for _, bad := range []string{"0", "", "abc"} {
		res, body = s.saveExamDraw(id, "50", "", "", bad)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the questions per attempt as a whole number of at least 1.")
	}
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Questions per attempt: 12")
}

func TestExamSettingsRejectInvalidInput(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.saveExamSettings(id, "101", "", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the pass mark as a whole number from 0 to 100.")
	res, body = s.saveExamSettings(id, "50", "abc", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the time limit in minutes, or leave it empty.")
	res, body = s.saveExamSettings(id, "50", "", "0")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the maximum attempts as a whole number, or leave it empty.")

	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Pass mark: 50%")
	expectStatus(t, res, body, http.StatusOK, "No time limit")
}

func TestAdminAddsAQuestionThatMakesTheExamUsable(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"

	res, body := s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Not usable yet")

	res, body = s.saveQuestion(examPath+"/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "What is Go?")
	expectStatus(t, res, body, http.StatusOK, "A language (correct)")
	expectStatus(t, res, body, http.StatusOK, "A drink")
	expectStatus(t, res, body, http.StatusOK, "Ready for Learners")
}

func TestQuestionFormRejectsBadQuestions(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	questions := "/admin/courses/" + id + "/exam/questions"

	res, body := s.saveQuestion(questions, "  ", [5]string{"A", "B", "", "", ""}, "0", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the question.")
	res, body = s.saveQuestion(questions, "What is Go?", [5]string{"A", "", "", "", ""}, "0", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter between 2 and 5 options.")
	res, body = s.saveQuestion(questions, "What is Go?", [5]string{"", "", "", "", ""}, "0", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter between 2 and 5 options.")
	res, body = s.saveQuestion(questions, "What is Go?", [5]string{"A", "B", "", "", ""}, "", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Mark the correct option, and make sure it has text.")
	res, body = s.saveQuestion(questions, "What is Go?", [5]string{"A", "B", "", "", ""}, "2", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Mark the correct option, and make sure it has text.")

	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "No questions yet.")
	expectStatus(t, res, body, http.StatusOK, "Not usable yet")
}

func TestDraftQuestionsAreNotUsableUntilPublished(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"

	res, body := s.saveQuestion(examPath+"/questions", "Draft question text", [5]string{"Yes", "No", "", "", ""}, "1", true)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Draft question text")
	expectStatus(t, res, body, http.StatusOK, "Draft")
	expectStatus(t, res, body, http.StatusOK, "Not usable yet")

	m := examQuestionAction.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no question action on the Exam page: %s", body)
	}
	res, body = s.post(examPath+"/questions/"+m[1]+"/publish", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Published")
	expectStatus(t, res, body, http.StatusOK, "Ready for Learners")
}

func TestAdminEditsAQuestionAndItsCorrectAnswer(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	_, body := s.get(examPath)
	m := examQuestionAction.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no question on the Exam page: %s", body)
	}
	questionPath := examPath + "/questions/" + m[1]

	res, body := s.get(questionPath + "/edit")
	expectStatus(t, res, body, http.StatusOK, "What is Go?")
	res, body = s.saveQuestion(questionPath, "What is Go, really?", [5]string{"A language", "A drink", "", "", ""}, "1", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "What is Go, really?")
	expectStatus(t, res, body, http.StatusOK, "A drink (correct)")
	if strings.Contains(body, "What is Go?") || strings.Contains(body, "A language (correct)") {
		t.Fatalf("edited question still shows its old text or answer: %s", body)
	}
}

var examQuestionID = regexp.MustCompile(`/exam/questions/(\d+)/edit`)

func TestAdminReordersQuestionsWithUpAndDown(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "First question text", [5]string{"A", "B", "", "", ""}, "0", false)
	s.saveQuestion(examPath+"/questions", "Second question text", [5]string{"A", "B", "", "", ""}, "0", false)
	_, body := s.get(examPath)
	ids := examQuestionID.FindAllStringSubmatch(body, -1)
	if len(ids) != 2 {
		t.Fatalf("want 2 questions on the Exam page: %s", body)
	}

	res, body := s.post(examPath+"/questions/"+ids[1][1]+"/up", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Second question text")
	if strings.Index(body, "Second question text") > strings.Index(body, "First question text") {
		t.Fatalf("moved question did not go up: %s", body)
	}

	res, body = s.post(examPath+"/questions/"+ids[1][1]+"/down", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	if strings.Index(body, "First question text") > strings.Index(body, "Second question text") {
		t.Fatalf("moved question did not go back down: %s", body)
	}
}

func TestAdminDeletesAQuestionOnlyAfterConfirming(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "Doomed question text", [5]string{"A", "B", "", "", ""}, "0", false)
	s.saveQuestion(examPath+"/questions", "Kept question text", [5]string{"A", "B", "", "", ""}, "0", false)
	_, body := s.get(examPath)
	ids := examQuestionID.FindAllStringSubmatch(body, -1)
	if len(ids) != 2 {
		t.Fatalf("want 2 questions on the Exam page: %s", body)
	}
	confirmPage := examPath + "/questions/" + ids[0][1] + "/delete"
	expectAll(t, body, `href="`+confirmPage+`"`)
	res, body := s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "Delete the question “Doomed question text” from the Exam for “Go Basics”? This cannot be undone.")
	expectAll(t, body, `href="`+examPath+`#q-`+ids[0][1]+`"`, `name="confirm" value="yes"`)

	res, body = s.post(confirmPage, nil)
	expectRedirect(t, res, body, confirmPage)
	_, body = s.get(examPath)
	expectAll(t, body, "Doomed question text")

	res, body = s.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Kept question text")
	if strings.Contains(body, "Doomed question text") {
		t.Fatalf("deleted question still listed: %s", body)
	}
}

func TestExamAdminPagesRefuseVisitorsAndLearners(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "Protected question text", [5]string{"A", "B", "", "", ""}, "0", false)
	_, body := s.get(examPath)
	m := examQuestionID.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no question on the Exam page: %s", body)
	}
	qid := m[1]
	s.logOut()

	pages := []struct{ method, path string }{
		{http.MethodGet, examPath},
		{http.MethodPost, examPath},
		{http.MethodGet, examPath + "/questions/new"},
		{http.MethodPost, examPath + "/questions"},
		{http.MethodGet, examPath + "/questions/" + qid + "/edit"},
		{http.MethodPost, examPath + "/questions/" + qid},
		{http.MethodPost, examPath + "/questions/" + qid + "/publish"},
		{http.MethodPost, examPath + "/questions/" + qid + "/up"},
		{http.MethodPost, examPath + "/questions/" + qid + "/down"},
		{http.MethodPost, examPath + "/questions/" + qid + "/delete"},
	}
	check := func(role string, wantStatus int, wantText string) {
		t.Helper()
		for _, p := range pages {
			var res *http.Response
			var body string
			if p.method == http.MethodPost {
				res, body = s.post(p.path, url.Values{"prompt": {"Hacked"}, "option": {"A", "B"}, "correct": {"0"}, "pass_mark": {"0"}})
			} else {
				res, body = s.get(p.path)
			}
			if res.StatusCode != wantStatus || !strings.Contains(body, wantText) {
				t.Fatalf("%s %s as %s = %d, want %d containing %q", p.method, p.path, role, res.StatusCode, wantStatus, wantText)
			}
		}
	}

	check("a Visitor", http.StatusSeeOther, "")
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	check("a Learner", http.StatusForbidden, "Only the Admin can open this page")

	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Protected question text")
	if strings.Contains(body, "Hacked") {
		t.Fatalf("refused request still changed the Exam: %s", body)
	}
}

func TestUnknownQuestionIsNotFound(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	res, body := s.get("/admin/courses/" + id + "/exam/questions/999/edit")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam question at this address.")
	res, body = s.post("/admin/courses/"+id+"/exam/questions/999/publish", nil)
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam question at this address.")
	res, body = s.get("/admin/courses/" + id + "/exam/questions/abc/edit")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam question at this address.")
	res, body = s.get("/admin/courses/" + id + "/exam/questions/0/edit")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Exam question at this address.")
}

func TestDeletingACourseRemovesItsExam(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Going question text", [5]string{"A", "B", "", "", ""}, "0", false)

	res, body := s.post("/admin/courses/"+id+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Course at this address.")
}

func TestAdminReachesTheExamFromTheCourseList(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, `href="/admin/courses/`+id+`/exam"`)
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Exam for Go Basics")
}

func TestANewExamAllowsTwoAttemptsUntilTheAdminChangesIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"

	res, body := s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Attempts allowed: 2")
	expectStatus(t, res, body, http.StatusOK, `name="max_attempts" type="number" min="1" value="2"`)
	s.saveQuestion(examPath+"/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Attempts allowed: 2")

	s.saveExamSettings(id, "50", "", "5")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Attempts allowed: 5")
}

func TestAnExamOfAnUnknownFormatIsNotFound(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses/" + id + "/exam?format=multiple-choice")
	expectStatus(t, res, body, http.StatusOK, "Exam for Go Basics")
	for _, path := range []string{"/admin/courses/" + id + "/exam?format=essay", "/courses/" + id + "/exam?format=essay"} {
		res, body = s.get(path)
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
}

func TestExamSettingsWarnWhenTheDrawCountIsMoreThanThePublishedQuestions(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	for _, p := range []string{"Q1", "Q2"} {
		s.saveQuestion(examPath+"/questions", p, [5]string{"Yes", "No", "", "", ""}, "0", false)
	}
	s.saveQuestion(examPath+"/questions", "Draft question", [5]string{"Yes", "No", "", "", ""}, "0", true)

	res, body := s.saveExamDraw(id, "50", "", "", "30")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Questions per attempt: 30")
	expectStatus(t, res, body, http.StatusOK, "Only 2 questions are published, so each attempt asks those 2, not 30.")
	s.switchLanguage("ms", examPath)
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Hanya 2 soalan diterbitkan, jadi setiap percubaan bertanya 2 soalan itu, bukan 30.")
	s.switchLanguage("en", examPath)

	s.saveExamDraw(id, "50", "", "", "2")
	_, body = s.get(examPath)
	if strings.Contains(body, "Only 2 questions are published") {
		t.Fatalf("Exam warns although the draw count equals the published questions: %s", body)
	}
}

func TestTheDrawWarningForOnePublishedQuestionIsSingular(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "Q1", [5]string{"Yes", "No", "", "", ""}, "0", false)
	s.saveExamDraw(id, "50", "", "", "30")

	_, body := s.getAsking(examPath, "en")
	expectAll(t, mainOf(body), "Only 1 question is published, so each attempt asks that 1, not 30.")
	_, body = s.getAsking(examPath, "ms")
	expectAll(t, mainOf(body), "Hanya 1 soalan diterbitkan, jadi setiap percubaan bertanya 1 soalan itu sahaja, bukan 30.")
}
