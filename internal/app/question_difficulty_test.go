package app_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// generateWith posts the Exam page's Generate questions form with the extra fields.
func (s *site) generateWith(courseID, topic, count string, extra url.Values) (*http.Response, string) {
	form := url.Values{"topic": {topic}, "language": {"ms"}, "count": {count}}
	for k, v := range extra {
		form[k] = v
	}
	return s.post("/admin/courses/"+courseID+"/exam/generate", form)
}

const (
	multipleChoice = "multiple-choice"
	written        = "written"
)

// publishedBankQuestions puts the prompts into the Question bank as published questions of the format on topic, in
// Malay, at difficulty, by generating them on courseID's Exam and publishing its Drafts. A Multiple-choice question's
// options are "Right answer" and "Wrong answer"; a Written one's model answer is "Bank model answer".
func (s *site) publishedBankQuestions(courseID, format, topic, difficulty string, prompts ...string) {
	s.t.Helper()
	qs := make([]aiwriter.Question, len(prompts))
	for i, p := range prompts {
		qs[i] = aiwriter.Question{Prompt: p, Options: []aiwriter.Option{{Text: "Right answer", Correct: true}, {Text: "Wrong answer"}}}
		if format == written {
			qs[i] = aiwriter.Question{Prompt: p, ModelAnswer: "Bank model answer"}
		}
	}
	s.ai.Reply(testutil.FakeReply{Questions: qs})
	exam := "/admin/courses/" + courseID + "/exam"
	query := "?format=" + format
	res, body := s.post(exam+"/generate"+query, url.Values{"topic": {topic}, "language": {"ms"}, "count": {strconv.Itoa(len(prompts))}, "difficulty": {difficulty}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	res, body = s.post(exam+"/publish-drafts"+query, nil)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

func TestGenerationAsksTheAIForTheChosenDifficultyMediumByDefault(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()

	for _, c := range []struct{ posted, want string }{{"hard", "hard"}, {"easy", "easy"}, {"", "medium"}, {"extreme", "medium"}} {
		before := len(s.ai.Requests())
		var extra url.Values
		if c.posted != "" {
			extra = url.Values{"difficulty": {c.posted}}
		}
		res, body := s.generateWith(id, "Topic "+c.posted, "1", extra)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		got := s.ai.Requests()
		if len(got) != before+1 || got[before].Difficulty != c.want {
			t.Fatalf("posting difficulty %q asked the AI writer %#v, want Difficulty %q", c.posted, got[before:], c.want)
		}
	}
	_, body := s.get("/admin/courses/" + id + "/exam")
	expectAll(t, body, `<select id="gen-difficulty" name="difficulty">`, `<option value="medium" selected>Medium</option>`)
}

func TestAIQuestionsKeepTheirDifficultyInTheExamAndTheBankAndHandWrittenOnesHaveNone(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Hand written", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.generateWith(id, "Excel", "1", url.Values{"difficulty": {"hard"}})

	_, body := s.get("/admin/courses/" + id + "/exam")
	expectInOrder(t, body, "Hand written", "Published", "AI question 1", "Draft", `<span class="badge badge-difficulty">Hard</span>`)
	expectNone(t, body, `badge-difficulty">Easy`, `badge-difficulty">Medium`)
	if n := strings.Count(body, `badge-difficulty"`); n != 1 {
		t.Fatalf("the Exam shows %d Difficulties, want 1 (the hand-written question has none): %s", n, body)
	}
	_, body = s.get("/admin/question-bank")
	expectInOrder(t, body, "Excel · Multiple-choice · Malay</p>", "Hand written", "Excel · Multiple-choice · Malay · Hard</p>", "AI question 1")

	s.switchLanguage("ms", "/admin/courses/"+id+"/exam")
	_, body = s.get("/admin/courses/" + id + "/exam")
	expectAll(t, body, `badge-difficulty">Sukar`)
}

func TestBankFirstTakesOnlyPublishedBankQuestionsOfTheSameDifficulty(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	excel := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+excel+"/exam/questions", "Unset one", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	s.generateWith(excel, "Excel", "1", url.Values{"difficulty": {"hard"}})
	s.generateWith(excel, "Excel", "1", url.Values{"difficulty": {"easy"}})
	s.post("/admin/courses/"+excel+"/exam/publish-drafts", nil)
	s.generateWith(excel, "Excel", "1", url.Values{"difficulty": {"hard"}}) // stays a Draft
	s.createCourse("Sheets", "Another Course.", "")
	var sheets string
	for _, c := range s.courseIDs() {
		if c != excel {
			sheets = c
		}
	}
	before := len(s.ai.Requests())

	res, body := s.generateWith(sheets, "excel", "3", url.Values{"difficulty": {"hard"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	want := aiwriter.Request{Topic: "excel", Format: "multiple-choice", Language: "ms", Count: 2, Difficulty: "hard"}
	if got := s.ai.Requests()[before:]; len(got) != 1 || got[0] != want {
		t.Fatalf("the AI writer was asked %#v, want only %#v: one published Hard question was in the bank", got, want)
	}
	_, body = s.get("/admin/courses/" + sheets + "/exam")
	expectNone(t, body, "Unset one")
	expectInOrder(t, body, "AI question 1", "Published", "Hard", "AI question 1", "Draft", "Hard", "AI question 2", "Draft", "Hard")

	s.createCourse("Office", "A third Course.", "")
	var office string
	for _, c := range s.courseIDs() {
		if c != excel && c != sheets {
			office = c
		}
	}
	res, body = s.addFromBank(office, "Excel", "ms", "10")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := len(s.examQuestionIDs(office)); n != 3 {
		t.Fatalf("Add from bank took %d questions, want the 3 published ones whatever their Difficulty", n)
	}
	_, body = s.get("/admin/courses/" + office + "/exam")
	expectAll(t, body, "Unset one", `badge-difficulty">Easy`, `badge-difficulty">Hard`)
}

func TestLearnersNeverSeeAQuestionsDifficulty(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Hand written")
	s.logOut()
	s.login("admin@example.com", examPassword)
	s.generateWith(id, "Go", "1", url.Values{"difficulty": {"hard"}})
	s.post("/admin/courses/"+id+"/exam/publish-drafts", nil)
	s.logOut()
	s.login("ada@example.com", examPassword)

	res, body := s.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "AI question 1")
	expectNone(t, body, "badge-difficulty", ">Hard<")
}

func TestAdminSetsAndClearsADifficultyOnAnExamQuestion(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	exam := "/admin/courses/" + id + "/exam"
	_, body := s.get(exam + "/questions/new")
	expectAll(t, body, `<select id="difficulty" name="difficulty"><option value="" selected>Not set</option>`)
	res, body := s.post(exam+"/questions", url.Values{"prompt": {"Hand written"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}, "difficulty": {"easy"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	qid := s.examQuestionIDs(id)[0]
	_, body = s.get(exam)
	expectAll(t, body, `badge-difficulty">Easy`)
	_, body = s.get("/admin/question-bank")
	expectAll(t, body, "Excel · Multiple-choice · Malay · Easy")

	_, body = s.get(exam + "/questions/" + qid + "/edit")
	expectAll(t, body, `<option value="easy" selected>Easy</option>`)
	res, body = s.post(exam+"/questions/"+qid, url.Values{"prompt": {"Hand written"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}, "difficulty": {"hard"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(exam)
	expectAll(t, body, `badge-difficulty">Hard`)
	res, body = s.post(exam+"/questions/"+qid, url.Values{"prompt": {"Hand written"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}, "difficulty": {""}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get(exam)
	expectNone(t, body, `badge-difficulty"`)
	res, body = s.post(exam+"/questions/"+qid, url.Values{"prompt": {"Hand written"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}, "difficulty": {"extreme"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose Easy, Medium, Hard or Not set.")

	s.saveQuestion(exam+"/questions", "No Difficulty", [5]string{"Right", "Wrong", "", "", ""}, "0", false)
	_, body = s.get(exam)
	expectNone(t, body, `badge-difficulty"`)
	_, body = s.get("/admin/question-bank")
	expectAll(t, body, "Excel · Multiple-choice · Malay · Easy") // the bank question keeps its own
}

func TestAdminSetsClearsAndFiltersByADifficultyInTheQuestionBank(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "Excel unset", [5]string{"Right", "Wrong", "", "", ""}, "0", false)
	s.publishedBankQuestions(id, multipleChoice, "Excel", "hard", "Excel hard")
	s.publishedBankQuestions(id, multipleChoice, "Word", "hard", "Word hard")
	b := s.bankQuestionIDs("/admin/question-bank?topic=Excel")

	_, body := s.get("/admin/question-bank/" + b[0] + "/edit")
	expectAll(t, body, `<option value="" selected>Not set</option>`)
	res, body := s.saveBankQuestion(b[0], "Excel", "ms", "Excel unset", [5]string{"Right", "Wrong", "", "", ""}, "0")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/question-bank?difficulty=none")
	expectAll(t, body, "Excel unset")
	expectNone(t, body, "Excel hard", "Word hard")

	form := url.Values{"topic": {"Excel"}, "language": {"ms"}, "prompt": {"Excel unset"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}, "difficulty": {"easy"}}
	res, body = s.post("/admin/question-bank/"+b[0], form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/question-bank?difficulty=easy")
	expectAll(t, body, "Excel unset", `<option value="easy" selected>Easy</option>`)
	expectNone(t, body, "Excel hard", "Word hard")
	_, body = s.get("/admin/question-bank?difficulty=hard&topic=Excel&format=multiple-choice")
	expectAll(t, body, "Excel hard")
	expectNone(t, body, "Excel unset", "Word hard")
	_, body = s.get("/admin/question-bank/" + b[1] + "/edit?difficulty=hard&topic=Excel")
	expectAll(t, body, `action="/admin/question-bank/`+b[1]+`?difficulty=hard&amp;topic=Excel"`)

	form.Set("difficulty", "")
	res, body = s.post("/admin/question-bank/"+b[0], form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/question-bank?difficulty=easy")
	expectNone(t, body, "Excel unset")
	form.Set("difficulty", "extreme")
	res, body = s.post("/admin/question-bank/"+b[0], form)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose Easy, Medium, Hard or Not set.")
}
