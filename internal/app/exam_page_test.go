package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// expectRedirect fails unless res is a See Other to want.
func expectRedirect(t *testing.T, res *http.Response, body, want string) {
	t.Helper()
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != want {
		t.Fatalf("%s %s went to %q, want %q", res.Request.Method, res.Request.URL.Path, loc, want)
	}
}

func TestEachActionOnAnExamQuestionReturnsToItsCard(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	for _, p := range []string{"First", "Second", "Third"} {
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", p, [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	}
	q := s.examQuestionIDs(id)
	exam := "/admin/courses/" + id + "/exam"
	_, body := s.get(exam)
	expectAll(t, body, `id="questions"`, `id="q-`+q[0]+`"`, `id="q-`+q[2]+`"`)

	res, body := s.post(exam+"/questions/"+q[1]+"/publish", nil)
	expectRedirect(t, res, body, exam+"#q-"+q[1])
	res, body = s.saveQuestion(exam+"/questions/"+q[1], "Second, edited", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	expectRedirect(t, res, body, exam+"#q-"+q[1])
	res, body = s.post(exam+"/questions/"+q[1]+"/up", nil)
	expectRedirect(t, res, body, exam+"#q-"+q[1])
	res, body = s.post(exam+"/questions/"+q[1]+"/down", nil)
	expectRedirect(t, res, body, exam+"#q-"+q[1])

	res, body = s.post(exam+"/questions/"+q[1]+"/delete", nil)
	expectRedirect(t, res, body, exam+"#q-"+q[2])
	res, body = s.post(exam+"/questions/"+q[2]+"/delete", nil)
	expectRedirect(t, res, body, exam+"#questions")
}

func TestActionsOnAWrittenExamQuestionKeepTheFormat(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	_, body := s.get(writtenExam(id))
	qid := examQuestionID.FindStringSubmatch(body)[1]

	res, body := s.post("/admin/courses/"+id+"/exam/questions/"+qid+"/up?format=written", nil)
	expectRedirect(t, res, body, writtenExam(id)+"#q-"+qid)
}

func TestEachActionOnABankQuestionReturnsToItsCardUnderTheSameFilters(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	for _, p := range []string{"Excel one", "Excel two", "Excel three"} {
		s.saveQuestion("/admin/courses/"+id+"/exam/questions", p, [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", true)
	}
	filtered := "/admin/question-bank?format=multiple-choice&topic=Excel"
	b := s.bankQuestionIDs(filtered)
	_, body := s.get(filtered)
	expectAll(t, body, `id="questions"`, `id="q-`+b[0]+`"`,
		`href="/admin/question-bank/`+b[0]+`/edit?format=multiple-choice&amp;topic=Excel"`,
		`action="/admin/question-bank/`+b[0]+`/publish?format=multiple-choice&amp;topic=Excel"`)
	_, body = s.get("/admin/question-bank/" + b[0] + "/edit?format=multiple-choice&topic=Excel")
	expectAll(t, body, `action="/admin/question-bank/`+b[0]+`?format=multiple-choice&amp;topic=Excel"`,
		`href="/admin/question-bank?format=multiple-choice&amp;topic=Excel#q-`+b[0]+`"`)

	res, body := s.post("/admin/question-bank/"+b[0]+"/publish?format=multiple-choice&topic=Excel", nil)
	expectRedirect(t, res, body, filtered+"#q-"+b[0])
	form := url.Values{"topic": {"Excel"}, "language": {"ms"}, "prompt": {"Excel one, edited"}, "correct": {"0"}, "option": {"Right", "Wrong", "", "", ""}}
	res, body = s.post("/admin/question-bank/"+b[0]+"?format=multiple-choice&topic=Excel", form)
	expectRedirect(t, res, body, filtered+"#q-"+b[0])
	res, body = s.post("/admin/question-bank/"+b[1]+"/delete?format=multiple-choice&topic=Excel", nil)
	expectRedirect(t, res, body, filtered+"#q-"+b[2])
	res, body = s.post("/admin/question-bank/"+b[2]+"/delete?format=multiple-choice&topic=Excel", nil)
	expectRedirect(t, res, body, filtered+"#questions")
	res, body = s.post("/admin/question-bank/"+b[0]+"/delete?evil=https://example.com", nil)
	expectRedirect(t, res, body, "/admin/question-bank#questions")
}

func TestPublishAllDraftsPublishesEveryDraftOnTheExamAndItsBankQuestions(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	id := s.onlyCourseID()
	exam := "/admin/courses/" + id + "/exam"
	s.saveQuestion(exam+"/questions", "Already published", [5]string{"Right answer", "Wrong answer", "", "", ""}, "0", false)
	_, body := s.get(exam)
	expectNone(t, body, "Publish all Drafts")

	s.generate(id, "Excel", "ms", "2")
	_, body = s.get(exam)
	expectAll(t, body, "Publish all Drafts (2)", `data-confirm="Publish all 2 Drafts? Learners can be asked them from then on."`)

	res, body := s.post(exam+"/publish-drafts", nil)
	expectRedirect(t, res, body, exam+"#questions")
	_, body = s.get(exam)
	expectNone(t, body, "Publish all Drafts", ">Draft<")
	_, body = s.get("/admin/question-bank")
	expectNone(t, body, ">Draft<")

	_, body = s.get(exam + "?format=written")
	expectNone(t, body, "Publish all Drafts")
}

func TestLearnersAreRefusedPublishAllDrafts(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "What is Go?")
	res, body := s.post("/admin/courses/"+id+"/exam/publish-drafts", nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
}

func TestGenerateHasItsOwnCardAboveAddFromTheBankWithABusyIndicator(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", examPassword)
	s.createCourse("Excel", "A Course.", "")
	_, body := s.get("/admin/courses/" + s.onlyCourseID() + "/exam")
	expectInOrder(t, body, "Generate questions with AI", `action="/admin/courses/`, `/exam/generate"`, "data-busy", "cave-loader",
		"up to a minute", "Add from the Question bank", `/exam/from-bank"`)
	if strings.Count(body, `formaction=`) != 0 {
		t.Fatalf("Generate still shares the bank form: %s", body)
	}
	expectAll(t, body, `src="/static/forms.js`)
}

func TestADraftCoursesExamIsNotFoundToLearnersButOpenToTheAdmin(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "30", "3", "Q1")
	exam := "/courses/" + id + "/exam"
	if res, body := s.get(exam); res.StatusCode != http.StatusOK || !strings.Contains(body, "Go Basics") {
		t.Fatalf("published Course's Exam page = %d, want 200 with the title", res.StatusCode)
	}
	if _, err := s.db.Exec(`UPDATE courses SET published = false`); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{exam, exam + "?format=written"} {
		res, body := s.get(p)
		expectStatus(t, res, body, http.StatusNotFound, "")
		if strings.Contains(body, "Go Basics") {
			t.Fatalf("GET %s leaked the draft Course's title", p)
		}
	}
	s.logOut()
	s.login("admin@example.com", examPassword)
	if res, body := s.get(exam); res.StatusCode != http.StatusOK || !strings.Contains(body, "Go Basics") {
		t.Fatalf("Admin's view of a draft Course's Exam = %d, want 200", res.StatusCode)
	}
}
