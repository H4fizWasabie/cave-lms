package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTheStartPageListsTheQuestionCountAttemptsUsedAndLeftAndWhatStartingUses(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "30", "3", "Q1", "Q2")

	res, body := s.getAsking("/courses/"+id+"/exam", "en")
	expectStatus(t, res, body, http.StatusOK, "Start the Exam")
	expectInOrder(t, content(body), "Questions: 2", "Time limit: 30 minutes", "Attempts left: 3",
		"Starting uses 1 of your 3 attempts.", "Start the Exam")
	_, body = s.getAsking("/courses/"+id+"/exam", "ms")
	expectAll(t, content(body), "Soalan: 2", "Memulakan Peperiksaan menggunakan 1 daripada 3 percubaan anda.")

	res, _ = s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1", qids[1]: "1"})
	s.releaseResults()
	_, body = s.getAsking("/courses/"+id+"/exam", "en")
	expectInOrder(t, content(body), "Attempts left: 2", "You have used 1 of 3 attempts. Starting uses 1 of the 2 left.", "Start the Exam")

	res, _ = s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1", qids[1]: "1"})
	s.releaseResults()
	_, body = s.getAsking("/courses/"+id+"/exam", "en")
	expectInOrder(t, content(body), "Attempts left: 1", "You have used 2 of 3 attempts. This is your last one.", "Start the Exam")
	_, body = s.getAsking("/courses/"+id+"/exam", "ms")
	expectAll(t, content(body), "Anda telah menggunakan 2 daripada 3 percubaan. Ini percubaan terakhir anda.")
}

func TestTheStartPageOfAnExamWithUnlimitedAttemptsCountsTheAttemptsUsed(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "", "", "Q1")
	_, body := s.getAsking("/courses/"+id+"/exam", "en")
	expectNone(t, content(body), "Starting uses", "You have used")

	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})
	s.releaseResults()
	_, body = s.getAsking("/courses/"+id+"/exam", "en")
	expectInOrder(t, content(body), "Attempts left: Unlimited", "You have used 1 attempt so far.", "Start the Exam")
}

func TestSubmittingWithBlankAnswersAsksFirstAndKeepsTheAnswersGiven(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Q1", "Q2", "Q3")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	_, body := s.get(attempt)
	qids := attemptQuestionIDs(body)
	expectInOrder(t, content(body), "Question 1 of 3", "Question 2 of 3", "Question 3 of 3")
	expectNone(t, content(body), "Submit anyway", "Not answered")

	res, body = s.post(attempt+"/submit", url.Values{"answer-" + qids[1]: {"1"}})
	expectStatus(t, res, body, http.StatusOK, "You have not answered 2 of 3 questions.")
	main := content(body)
	// "Submit anyway" comes after "Submit answers", so Enter in the form never confirms.
	expectInOrder(t, main, `role="alert"`, `href="#q-1"`, `id="q-1"`, "Not answered", `id="q-2"`,
		`name="answer-`+qids[1]+`" value="1" checked`, `id="q-3"`, "Not answered", "Submit answers", `name="confirm" value="yes"`, "Submit anyway")
	if n := strings.Count(main, "Not answered"); n != 2 {
		t.Fatalf("%d questions are marked Not answered, want the 2 blank ones", n)
	}
	_, body = s.get(attempt) // not submitted
	expectAll(t, content(body), "Submit answers")
	expectNone(t, content(body), "Congratulations")

	_, body = s.postAsking(attempt+"/submit", "ms", url.Values{"answer-" + qids[1]: {"1"}})
	expectAll(t, content(body), "Anda belum menjawab 2 daripada 3 soalan.", "Hantar juga", "Belum dijawab")

	res, body = s.post(attempt+"/submit", url.Values{"answer-" + qids[1]: {"1"}, "confirm": {"yes"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.releaseResults()
	_, body = s.get(attempt)
	expectAll(t, body, "Score: 0%")
}

func TestAnsweringEveryQuestionSubmitsWithoutAsking(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Q1", "Q2")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	_, body := s.get(attempt)
	form := url.Values{}
	for _, qid := range attemptQuestionIDs(body) {
		form.Set("answer-"+qid, "0")
	}
	res, body = s.post(attempt+"/submit", form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestABlankWrittenAnswerAsksFirstAndKeepsTheWordsTyped(t *testing.T) {
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=", "Explain func")
	res, _ := s.post("/courses/"+id+"/exam/attempts?format=written", nil)
	attempt := res.Header.Get("Location")
	_, body := s.get(attempt)
	qids := attemptQuestionIDs(body)
	submit := strings.Replace(attempt, "?", "/submit?", 1)

	res, body = s.post(submit, url.Values{"answer-" + qids[0]: {"  "}, "answer-" + qids[1]: {"It <declares> a function."}})
	expectStatus(t, res, body, http.StatusOK, "You have not answered 1 of 2 questions.")
	expectAll(t, content(body), "It &lt;declares&gt; a function.</textarea>", `action="`+strings.ReplaceAll(submit, "&", "&amp;")+`"`)
}

func TestWhenTimeIsUpBlankAnswersAreSubmittedWithoutAsking(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "1", "", "Q1", "Q2")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	_, body := s.get(attempt)
	qids := attemptQuestionIDs(body)
	s.ageAttempt(attempt, 65) // past the one-minute limit, inside the grace period: the timer submits at zero

	res, body = s.post(attempt+"/submit", url.Values{"answer-" + qids[0]: {"0"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.releaseResults()
	_, body = s.get(attempt)
	expectAll(t, body, "Score: 50%")
}

// Asking about blank answers costs a round trip; with under a minute left before the server stops taking the
// submission (the time limit plus its 30 s grace), the answers are submitted as they are instead.
func TestWithUnderAMinuteLeftBlankAnswersAreSubmittedWithoutAsking(t *testing.T) {
	for _, c := range []struct {
		age    int // seconds into the one-minute limit
		status int
	}{{20, http.StatusOK}, {40, http.StatusSeeOther}} { // 70 s left, then 50 s, counting the grace
		s := newSite(t)
		id, _ := s.examCourse("50", "1", "", "Q1", "Q2")
		res, _ := s.startExam(id)
		attempt := res.Header.Get("Location")
		_, body := s.get(attempt)
		qids := attemptQuestionIDs(body)
		s.ageAttempt(attempt, c.age)

		res, body = s.post(attempt+"/submit", url.Values{"answer-" + qids[0]: {"0"}})
		if res.StatusCode != c.status {
			t.Fatalf("%d s in, a submission with a blank answer answers %d, want %d: %s", c.age, res.StatusCode, c.status, body)
		}
		if c.status == http.StatusOK {
			expectAll(t, body, "You have not answered 1 of 2 questions.")
		}
	}
}

func TestTheAttemptPageWarnsAtFiveAndOneMinutesInAPoliteLiveRegionAndCountsTheAnswers(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "30", "", "Q1", "Q2")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")

	_, body := s.getAsking(attempt, "en")
	main := content(body)
	expectAll(t, main, `data-answered="{n} of 2 answered"`, `aria-live="polite"`,
		`data-warn-5="Less than 5 minutes left. Check your answers, then submit."`,
		`data-warn-1="Less than 1 minute left. Your answers are sent automatically when time runs out."`)
	_, body = s.getAsking(attempt, "ms")
	expectAll(t, content(body), `data-answered="{n} daripada 2 dijawab"`, `data-warn-5="Kurang daripada 5 minit lagi.`, `data-warn-1="Kurang daripada 1 minit lagi.`)

	src := timerScript.FindStringSubmatch(body)
	if src == nil {
		t.Fatalf("the attempt page does not load exam-timer.js: %s", body)
	}
	_, js := s.get(src[1])
	expectAll(t, js, "data-answered", "warn5", "warn1", `"confirm"`)
}

func TestAnAttemptWithNoTimeLimitStillCountsTheAnswersButHasNoTimer(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(id)
	_, body := s.get(res.Header.Get("Location"))
	expectAll(t, content(body), `data-answered="{n} of 1 answered"`)
	expectNone(t, content(body), "data-seconds-left", "aria-live")
	if timerScript.FindStringSubmatch(body) == nil {
		t.Fatalf("the attempt page does not load exam-timer.js for the answered count: %s", body)
	}
}

func TestTheSubmittedPageSaysWhenTheResultIsReleasedAndWhereToFindIt(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0"})
	s.setSubmitTime(attempt, "2099-01-02 03:04:00+00")

	_, body := s.getAsking(attempt, "en")
	expectInOrder(t, content(body), "Your Result is ready at 2 January 2099, 11:14",
		"Once it is released, you will find it on this page and in My exams.", `href="/my-exams"`)
	expectNone(t, content(body), "Written answers are marked first")
	_, body = s.getAsking(attempt, "ms")
	expectAll(t, content(body), "Setelah dikeluarkan, anda boleh melihatnya di halaman ini dan di Peperiksaan saya.")
}

func TestTheSubmittedPageOfAWrittenExamSaysMarkingCanDelayTheResult(t *testing.T) {
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=")
	attempt, _ := s.sitWritten(id, "It declares and assigns.")
	_, body := s.getAsking(attempt, "en")
	expectInOrder(t, content(body), "Your Result is ready at", "Written answers are marked first", `href="/my-exams"`)
}

func TestMyExamsShowsAwaitingResultBetweenSubmittingAndTheRelease(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Q1")
	_, body := s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Not taken")

	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})
	_, body = s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Awaiting Result", "Pass mark: 50%")
	expectNone(t, content(body), "Not taken", "Failed", "Best score")
	_, body = s.getAsking("/my-exams", "ms")
	expectAll(t, content(body), "Menunggu Keputusan")
	expectNone(t, content(body), "Belum diambil")

	s.releaseResults()
	_, body = s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Failed", "Best score: 0%")
	expectNone(t, content(body), "Awaiting Result")

	res, _ = s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	_, body = s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Awaiting Result", "Best score: 0%")
	s.releaseResults()
	_, body = s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Passed", "Best score: 100%")
	expectNone(t, content(body), "Awaiting Result")
}

func TestMyExamsShowsAwaitingResultForAnAttemptLeftOpenPastItsTimeLimit(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "1", "", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	_, body := s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Not taken")

	s.ageAttempt(attempt, 120)
	_, body = s.getAsking("/my-exams", "en")
	expectInOrder(t, content(body), ">Go Basics<", "Awaiting Result")
}
