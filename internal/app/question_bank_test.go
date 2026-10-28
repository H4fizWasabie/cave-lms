package app_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

var bankQuestionAction = regexp.MustCompile(`/admin/question-bank/(\d+)/`)

// bankQuestionIDs lists the question IDs on the Question bank page, in order.
func (s *site) bankQuestionIDs(path string) []string {
	s.t.Helper()
	_, body := s.get(path)
	var ids []string
	for _, m := range bankQuestionAction.FindAllStringSubmatch(body, -1) {
		if len(ids) == 0 || ids[len(ids)-1] != m[1] {
			ids = append(ids, m[1])
		}
	}
	return ids
}

// courseIDs reads the IDs of the Courses on the Admin's Course list.
func (s *site) courseIDs() []string {
	s.t.Helper()
	_, body := s.get("/admin/courses")
	var ids []string
	for _, m := range courseEditLink.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// saveBankQuestion posts the Question bank's edit form.
func (s *site) saveBankQuestion(id, topic, language, prompt string, options [5]string, correct string) (*http.Response, string) {
	form := url.Values{"topic": {topic}, "language": {language}, "prompt": {prompt}, "correct": {correct}}
	for _, o := range options {
		form.Add("option", o)
	}
	return s.post("/admin/question-bank/"+id, form)
}

func TestAQuestionWrittenInAnExamIsSavedToTheQuestionBank(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)

	res, body := s.get("/admin/question-bank")
	expectStatus(t, res, body, http.StatusOK, "What is Go?")
	expectInOrder(t, body, "Go Basics · Multiple-choice · Malay", "What is Go?", "A language (correct)", "A drink")
}

func TestExistingExamQuestionsAreInTheQuestionBankAfterTheMigration(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.examCourse("50", "", "", "First question text", "Second question text")
	if err := app.MigrateDownTo(context.Background(), s.db, 78); err != nil {
		t.Fatal(err)
	}
	s.restart()

	s.logOut()
	s.login("admin@example.com", examPassword)
	res, body := s.get("/admin/question-bank")
	expectStatus(t, res, body, http.StatusOK, "")
	expectInOrder(t, body, "Go Basics · Multiple-choice · Malay", "First question text", "Right answer (correct)", "Wrong answer",
		"Go Basics · Multiple-choice · Malay", "Second question text")
	if n := strings.Count(body, "First question text"); n != 1 {
		t.Fatalf("the bank lists the first question %d times, want once: %s", n, body)
	}
}

func TestBankQuestionsAndExamCopiesChangeIndependently(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	examPath := "/admin/courses/" + id + "/exam"
	s.saveQuestion(examPath+"/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	examQ := s.examQuestionIDs(id)[0]
	bankQ := s.bankQuestionIDs("/admin/question-bank")[0]

	res, body := s.saveQuestion(examPath+"/questions/"+examQ, "What is Go, in the Exam?", [5]string{"A language", "A drink", "A car", "", ""}, "0", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/question-bank/" + bankQ + "/edit")
	expectStatus(t, res, body, http.StatusOK, "What is Go?</textarea>")
	if strings.Contains(body, "A car") {
		t.Fatalf("editing the Exam question changed the bank question: %s", body)
	}

	res, body = s.saveBankQuestion(bankQ, "Golang", "en", "What is Go, in the bank?", [5]string{"A drink", "A language", "", "", ""}, "1")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/question-bank")
	expectInOrder(t, body, "Golang · Multiple-choice · English", "What is Go, in the bank?", "A drink", "A language (correct)")
	res, body = s.get(examPath)
	expectInOrder(t, body, "What is Go, in the Exam?", "A language (correct)", "A drink", "A car")

	confirmPage := "/admin/question-bank/" + bankQ + "/delete"
	_, body = s.get("/admin/question-bank")
	expectAll(t, body, `href="`+confirmPage+`"`)
	res, body = s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "Delete “What is Go, in the bank?” from the Question bank? Exam copies of it stay. This cannot be undone.")
	res, body = s.post(confirmPage, nil)
	expectRedirect(t, res, body, confirmPage)
	_, body = s.get("/admin/question-bank")
	expectAll(t, body, "What is Go, in the bank?")
	res, body = s.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/question-bank")
	expectStatus(t, res, body, http.StatusOK, "No questions in the Question bank.")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "What is Go, in the Exam?")

	s.saveQuestion(examPath+"/questions", "What is a goroutine?", [5]string{"A thread", "A fish", "", "", ""}, "0", false)
	ids := s.examQuestionIDs(id)
	res, body = s.post(examPath+"/questions/"+ids[len(ids)-1]+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/question-bank")
	expectStatus(t, res, body, http.StatusOK, "What is a goroutine?")
}

func TestAdminBrowsesTheQuestionBankByTopicAndFormat(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	for _, title := range []string{"Excel", "Word"} {
		s.createCourse(title, "A Course.", "")
	}
	for _, c := range s.courseIDs() {
		_, body := s.get("/admin/courses/" + c)
		prompt := "A Word question"
		if strings.Contains(body, "<h1>Excel") {
			prompt = "An Excel question"
		}
		s.saveQuestion("/admin/courses/"+c+"/exam/questions", prompt, [5]string{"Yes", "No", "", "", ""}, "0", false)
	}

	res, body := s.get("/admin/question-bank?topic=EXCEL")
	expectStatus(t, res, body, http.StatusOK, "An Excel question")
	if strings.Contains(body, "A Word question") {
		t.Fatalf("the Excel topic lists a Word question: %s", body)
	}
	res, body = s.get("/admin/question-bank?format=multiple-choice")
	expectInOrder(t, body, "An Excel question", "A Word question")
	res, body = s.get("/admin/question-bank?format=written")
	expectStatus(t, res, body, http.StatusOK, "No questions in the Question bank.")
}

// addFromBank posts the Exam page's Add from the Question bank form.
func (s *site) addFromBank(courseID, topic, language, count string) (*http.Response, string) {
	return s.post("/admin/courses/"+courseID+"/exam/from-bank", url.Values{"topic": {topic}, "language": {language}, "count": {count}})
}

func TestAdminAddsMatchingPublishedBankQuestionsNotAlreadyInThePool(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	excel := s.onlyCourseID()
	for _, p := range []string{"Excel one", "Excel two", "Excel three"} {
		s.saveQuestion("/admin/courses/"+excel+"/exam/questions", p, [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	s.saveQuestion("/admin/courses/"+excel+"/exam/questions", "Excel draft", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	bank := s.bankQuestionIDs("/admin/question-bank")
	s.saveBankQuestion(bank[2], "Excel", "en", "Excel three", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0")

	res, body := s.addFromBank(excel, "excel", "ms", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "No more published questions in the Question bank match.")

	s.createCourse("Spreadsheets", "Another Course.", "")
	var sheets string
	for _, c := range s.courseIDs() {
		if c != excel {
			sheets = c
		}
	}
	examPath := "/admin/courses/" + sheets + "/exam"
	res, body = s.addFromBank(sheets, "EXCEL", "ms", "1")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := len(s.examQuestionIDs(sheets)); n != 1 {
		t.Fatalf("the Exam has %d questions after adding 1 from the bank, want 1", n)
	}
	res, body = s.addFromBank(sheets, "Excel", "ms", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Ready for Learners")
	for _, p := range []string{"Excel one", "Excel two"} {
		if n := strings.Count(body, p); n != 1 {
			t.Fatalf("the Exam shows %q %d times, want once: %s", p, n, body)
		}
	}
	expectStatus(t, res, body, http.StatusOK, "Right answer (correct)")
	if strings.Contains(body, "Excel three") || strings.Contains(body, "Excel draft") {
		t.Fatalf("the Exam took an English or Draft bank question: %s", body)
	}
	res, body = s.addFromBank(sheets, "excel", "ms", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "No more published questions in the Question bank match.")

	res, body = s.addFromBank(sheets, "excel", "en", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.post("/admin/question-bank/"+bank[3]+"/publish", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.addFromBank(sheets, "excel", "ms", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(examPath)
	expectStatus(t, res, body, http.StatusOK, "Excel three")
	expectStatus(t, res, body, http.StatusOK, "Excel draft")
	if n := len(s.examQuestionIDs(sheets)); n != 4 {
		t.Fatalf("the Exam has %d questions, want 4", n)
	}
}

func TestAddFromTheQuestionBankRejectsABadForm(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()

	res, body := s.addFromBank(id, " ", "ms", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the topic.")
	res, body = s.addFromBank(id, "Excel", "fr", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose Malay or English.")
	res, body = s.addFromBank(id, "Excel", "ms", "0")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter how many questions to add, as a whole number from 1.")
}

func TestLearnersAreRefusedTheQuestionBankAndItsActions(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "What is Go?")
	s.logOut()
	s.login("admin@example.com", examPassword)
	bankQ := s.bankQuestionIDs("/admin/question-bank")[0]
	s.logOut()
	s.loginAsNewLearner("Mallory", "mallory@example.com", examPassword)

	res, body := s.get("/admin/question-bank")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	res, body = s.get("/admin/question-bank/" + bankQ + "/edit")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	for _, path := range []string{"/admin/question-bank/" + bankQ + "/publish", "/admin/question-bank/" + bankQ + "/delete"} {
		res, body = s.post(path, confirming)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	res, body = s.saveBankQuestion(bankQ, "Hacked", "en", "Hacked?", [5]string{"Yes", "No", "", "", ""}, "0")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	res, body = s.addFromBank(id, "Go Basics", "ms", "5")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")

	s.logOut()
	s.login("admin@example.com", examPassword)
	res, body = s.get("/admin/question-bank")
	expectInOrder(t, body, "Go Basics · Multiple-choice · Malay", "What is Go?")
	if n := len(s.examQuestionIDs(id)); n != 1 {
		t.Fatalf("the Exam has %d questions, want 1", n)
	}
}

func TestTheBankQuestionFormRejectsBadInput(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	bankQ := s.bankQuestionIDs("/admin/question-bank")[0]
	options := [5]string{"A language", "A drink", "", "", ""}

	res, body := s.saveBankQuestion(bankQ, " ", "ms", "What is Go?", options, "0")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the topic.")
	res, body = s.saveBankQuestion(bankQ, "Go", "fr", "What is Go?", options, "0")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose Malay or English.")
	res, body = s.saveBankQuestion(bankQ, "Go", "ms", "What is Go?", options, "3")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Mark the correct option, and make sure it has text.")
	res, body = s.get("/admin/question-bank/999999/edit")
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = s.get("/admin/question-bank")
	expectInOrder(t, body, "Go Basics · Multiple-choice · Malay", "What is Go?")
}
