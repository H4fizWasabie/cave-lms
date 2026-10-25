package app_test

import (
	"net/http"
	"net/url"
	"testing"
)

// examCourseForBea is examCourse with Bea, who has no IC, in Ada's place: joined, the Lesson done, logged in.
func (s *site) examCourseForBea() string {
	s.t.Helper()
	id, _ := s.examCourse("50", "", "", "Which is right?")
	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", examPassword)
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	return id
}

// setExamIC posts the IC step for the Exam at examPath, with the IC typed and typed again.
func (s *site) setExamIC(examPath, ic, again string) (*http.Response, string) {
	return s.post(examPath, url.Values{"ic": {ic}, "ic-confirm": {again}})
}

func TestALearnerWithoutAnICSetsItBeforeTheirFirstExamAttemptAndContinuesToTheExam(t *testing.T) {
	s := newSite(t)
	id := s.examCourseForBea()
	exam, step := "/courses/"+id+"/exam", "/courses/"+id+"/exam/ic"

	res, body := s.startExam(id)
	expectRedirect(t, res, body, step)
	_, body = s.getAsking(step, "en")
	expectInOrder(t, mainOf(body), "Go Basics", "IC or passport number", `action="`+step+`"`, `id="ic"`, `id="ic-confirm"`, "</button>")
	_, body = s.getAsking(step, "ms")
	expectInOrder(t, mainOf(body), "No. KP atau pasport", `id="ic-confirm"`)

	res, body = s.setExamIC(step, " 951206 - 08 5555 ", "951206085555")
	expectRedirect(t, res, body, exam)
	_, body = s.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5555", "Only the Admin can change it")

	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc == step {
		t.Fatalf("Bea was sent to the IC step again after setting her IC")
	}
	res, body = s.get(step) // set once: the step only leads on to the Exam
	expectRedirect(t, res, body, exam)
}

func TestTheExamICStepRefusesMismatchedInvalidOrTakenEntriesAndKeepsTheForm(t *testing.T) {
	s := newSite(t)
	id := s.examCourseForBea()
	step := "/courses/" + id + "/exam/ic"

	res, body := s.setExamIC(step, "951206-08-5555", "951206-08-5556")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectFieldError(t, body, "ic-confirm", "The two entries do not match", "</button>")
	expectInOrder(t, mainOf(body), `value="951206-08-5555"`, `value="951206-08-5556"`)
	_, body = s.postAsking(step, "ms", url.Values{"ic": {"951206085555"}, "ic-confirm": {"951206085556"}})
	expectFieldError(t, body, "ic-confirm", "Kedua-dua entri tidak sepadan", "</button>")

	res, body = s.setExamIC(step, "95/1206", "95/1206")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectFieldError(t, body, "ic", "letters and digits", `id="ic-confirm"`)

	cat := s.second()
	cat.loginAsNewLearner("Cat", "cat@example.com", examPassword)
	cat.setIC("951206085555")
	res, body = s.setExamIC(step, "951206085555", "951206 08 5555")
	expectStatus(t, res, body, http.StatusConflict, "")
	expectFieldError(t, body, "ic", "already used by another Learner", `id="ic-confirm"`)

	_, body = s.get("/my-profile")
	expectInOrder(t, content(body), `action="/my-profile/ic"`) // still unset
}

func TestTheCoursePageRemindsOnlyAnEnrolledLearnerWithoutAnICOnACourseWithAnExam(t *testing.T) {
	const reminder = "Set your IC or passport number before your first Exam attempt"
	s := newSite(t)
	id := s.examCourseForBea()
	_, body := s.getAsking("/courses/"+id, "en")
	expectInOrder(t, mainOf(body), reminder, `href="/my-profile"`)
	_, body = s.getAsking("/courses/"+id, "ms")
	expectInOrder(t, mainOf(body), "Tetapkan No. KP atau pasport anda sebelum cubaan Peperiksaan pertama anda")

	admin := s.adminSite()
	admin.createCourse("No Exam", "Just reading.", "")
	var noExam string
	if err := admin.db.QueryRow(`SELECT id FROM courses WHERE title = 'No Exam'`).Scan(&noExam); err != nil {
		t.Fatal(err)
	}
	s.post("/courses/"+noExam+"/join", nil)
	_, body = s.get("/courses/" + noExam)
	expectNone(t, mainOf(body), reminder)

	cat := s.second()
	cat.loginAsNewLearner("Cat", "cat@example.com", examPassword) // not joined
	_, body = cat.get("/courses/" + id)
	expectNone(t, mainOf(body), reminder)
	_, body = s.second().get("/courses/" + id) // a Visitor
	expectNone(t, mainOf(body), reminder)

	s.setIC("951206085555")
	_, body = s.get("/courses/" + id)
	expectNone(t, mainOf(body), reminder)
}

func TestAWrittenExamSendsALearnerWithoutAnICToItsOwnICStepAndBack(t *testing.T) {
	s := newSite(t)
	id := s.examCourseForBea()
	s.adminSite().saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	step := "/courses/" + id + "/exam/ic?format=written"

	res, body := s.post("/courses/"+id+"/exam/attempts?format=written", nil)
	expectRedirect(t, res, body, step)
	_, body = s.get(step)
	expectInOrder(t, mainOf(body), "Written Exam", `action="/courses/`+id+`/exam/ic?format=written"`)
	res, body = s.setExamIC(step, "A1234567", "a123 4567")
	expectRedirect(t, res, body, "/courses/"+id+"/exam?format=written")
}

func TestTheExamICStepSendsALearnerWhoCannotStartTheExamBackToTheCourseSavingNothing(t *testing.T) {
	s := newSite(t)
	id := s.examCourseForBea()
	admin := s.adminSite()
	admin.createCourse("No Exam", "Just reading.", "")
	var noExam string
	if err := admin.db.QueryRow(`SELECT id FROM courses WHERE title = 'No Exam'`).Scan(&noExam); err != nil {
		t.Fatal(err)
	}
	s.post("/courses/"+noExam+"/join", nil)
	cat := s.second()
	cat.loginAsNewLearner("Cat", "cat@example.com", examPassword) // not joined

	for _, c := range []struct {
		who    *site
		course string
	}{{s, noExam}, {cat, id}} {
		step := "/courses/" + c.course + "/exam/ic"
		res, body := c.who.get(step)
		expectRedirect(t, res, body, "/courses/"+c.course)
		res, body = c.who.setExamIC(step, "951206085555", "951206085555")
		expectRedirect(t, res, body, "/courses/"+c.course)
		_, body = c.who.get("/my-profile")
		expectInOrder(t, content(body), `action="/my-profile/ic"`) // still unset
	}
}
