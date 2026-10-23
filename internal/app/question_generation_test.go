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

// generate posts the Exam page's Generate questions form.
func (s *site) generate(courseID, topic, language, count string) (*http.Response, string) {
	return s.post("/admin/courses/"+courseID+"/exam/generate", url.Values{"topic": {topic}, "language": {language}, "count": {count}})
}

func TestTheAIWritesGeneratedQuestionsAsDraftsInTheBankAndThePool(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.generate(id, " Excel pivot tables ", "en", "2")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	want := aiwriter.Request{Topic: "Excel pivot tables", Format: "multiple-choice", Language: "en", Count: 2}
	if got := s.ai.Requests(); len(got) != 1 || got[0] != want {
		t.Fatalf("the AI writer was asked %#v, want only %#v", got, want)
	}
	res, body = s.get("/admin/courses/" + id + "/exam")
	expectInOrder(t, body, "AI question 1", "Draft", "AI right (correct)", "AI wrong", "AI question 2", "Draft")
	expectStatus(t, res, body, http.StatusOK, "Not usable yet")
	res, body = s.get("/admin/question-bank")
	expectInOrder(t, body, "Excel pivot tables · Multiple-choice · English", "AI question 1", "Draft", "AI right (correct)",
		"Excel pivot tables · Multiple-choice · English", "AI question 2", "Draft")
	if strings.Contains(body, "Go Basics ·") {
		t.Fatalf("the generated questions were filed under the Course title: %s", body)
	}
}

func TestGenerationTakesMatchingBankQuestionsFirstAndAsksTheAIOnlyForTheRest(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	excel := s.onlyCourseID()
	for _, p := range []string{"Bank one", "Bank two", "Bank three"} {
		s.saveQuestion("/admin/courses/"+excel+"/exam/questions", p, [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	}
	s.createCourse("Spreadsheets", "Another Course.", "")
	var sheets string
	for _, c := range s.courseIDs() {
		if c != excel {
			sheets = c
		}
	}
	s.addFromBank(sheets, "excel", "ms", "1")

	res, body := s.generate(sheets, "EXCEL", "ms", "4")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	want := aiwriter.Request{Topic: "EXCEL", Format: "multiple-choice", Language: "ms", Count: 2}
	if got := s.ai.Requests(); len(got) != 1 || got[0] != want {
		t.Fatalf("the AI writer was asked %#v, want only %#v", got, want)
	}
	res, body = s.get("/admin/courses/" + sheets + "/exam")
	for _, p := range []string{"Bank one", "Bank two", "Bank three"} {
		if n := strings.Count(body, p); n != 1 {
			t.Fatalf("the Exam shows %q %d times, want once: %s", p, n, body)
		}
	}
	expectInOrder(t, body, "AI question 1", "Draft", "AI question 2", "Draft")
	if n := len(s.examQuestionIDs(sheets)); n != 5 {
		t.Fatalf("the Exam has %d questions, want 5", n)
	}

	s.createCourse("Office", "A third Course.", "")
	var office string
	for _, c := range s.courseIDs() {
		if c != excel && c != sheets {
			office = c
		}
	}
	res, body = s.generate(office, "Excel", "ms", "3")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := len(s.ai.Requests()); n != 1 {
		t.Fatalf("the AI writer was asked %d times, want once: the bank had enough", n)
	}
}

func TestAIOutputInTheWrongShapeIsRetriedOnceThenReportedAndNothingIsSaved(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Bank one", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.createCourse("Spreadsheets", "Another Course.", "")
	var sheets string
	for _, c := range s.courseIDs() {
		if c != id {
			sheets = c
		}
	}
	right, wrong := aiwriter.Option{Text: "Right", Correct: true}, aiwriter.Option{Text: "Wrong"}
	bad := []aiwriter.Question{
		{Prompt: "One option", Options: []aiwriter.Option{right}},
		{Prompt: "Six options", Options: []aiwriter.Option{right, wrong, wrong, wrong, wrong, wrong}},
		{Prompt: "Two correct", Options: []aiwriter.Option{right, right}},
		{Prompt: "None correct", Options: []aiwriter.Option{wrong, wrong}},
		{Prompt: " ", Options: []aiwriter.Option{right, wrong}},
		{Prompt: "Blank option", Options: []aiwriter.Option{right, {Text: " "}}},
	}
	for _, q := range bad {
		s.ai.Reply(testutil.FakeReply{Questions: []aiwriter.Question{q}}, testutil.FakeReply{Err: aiwriter.ErrBadShape})
		res, body := s.generate(sheets, "Excel", "ms", "2")
		expectStatus(t, res, body, http.StatusServiceUnavailable, "The AI&#39;s questions came back in the wrong shape twice, so none were saved.")
	}
	s.ai.Reply(testutil.FakeReply{Questions: []aiwriter.Question{{Prompt: "Too few", Options: []aiwriter.Option{right, wrong}}, {Prompt: "Too many", Options: []aiwriter.Option{right, wrong}}}},
		testutil.FakeReply{Questions: nil})
	res, body := s.generate(sheets, "Excel", "ms", "2")
	expectStatus(t, res, body, http.StatusServiceUnavailable, "wrong shape twice")
	if n := len(s.ai.Requests()); n != 2*len(bad)+2 {
		t.Fatalf("the AI writer was asked %d times, want twice per generation", n)
	}
	if n := len(s.examQuestionIDs(sheets)); n != 0 {
		t.Fatalf("the Exam has %d questions after failed generations, want none", n)
	}
	if n := len(s.bankQuestionIDs("/admin/question-bank")); n != 1 {
		t.Fatalf("the bank has %d questions after failed generations, want 1", n)
	}

	s.ai.Reply(testutil.FakeReply{Questions: bad[:1]})
	res, body = s.generate(sheets, "Excel", "ms", "2")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/courses/" + sheets + "/exam")
	expectInOrder(t, body, "Bank one", "AI question 1", "Draft")
	if strings.Contains(body, "One option") {
		t.Fatalf("the Exam saved a question in the wrong shape: %s", body)
	}
}

func TestGenerationExplainsWhenTheAIIsNotConfiguredUnavailableOrOverItsLimitAndSavesNothing(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Bank one", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.createCourse("Spreadsheets", "Another Course.", "")
	var sheets string
	for _, c := range s.courseIDs() {
		if c != id {
			sheets = c
		}
	}
	for err, want := range map[error]string{
		aiwriter.ErrNotConfigured:                          "The AI writer is not configured yet, so no questions were added.",
		fmt.Errorf("%w: timeout", aiwriter.ErrUnavailable): "The AI service is unavailable right now, so no questions were added.",
		fmt.Errorf("%w: 402", aiwriter.ErrOverLimit):       "The AI key is over its spending limit, so no questions were added.",
	} {
		s.ai.Reply(testutil.FakeReply{Err: err})
		res, body := s.generate(sheets, "Excel", "ms", "3")
		expectStatus(t, res, body, http.StatusServiceUnavailable, want)
	}
	if n := len(s.examQuestionIDs(sheets)); n != 0 {
		t.Fatalf("the Exam has %d questions after failed generations, want none", n)
	}

	res, body := s.generate(sheets, "Excel", "ms", "51")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Generate at most 50 questions at a time.")
	res, body = s.generate(sheets, " ", "ms", "5")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the topic.")
}

func TestLearnersAreRefusedGeneration(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "What is Go?")
	s.logOut()
	s.loginAsNewLearner("Mallory", "mallory@example.com", examPassword)

	res, body := s.generate(id, "Go", "ms", "5")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	if n := len(s.ai.Requests()); n != 0 {
		t.Fatalf("a Learner's generation asked the AI writer %d times, want none", n)
	}
}
