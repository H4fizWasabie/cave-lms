package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// adminSite logs a second client in as the Admin made by examCourse.
func (s *site) adminSite() *site {
	s.t.Helper()
	admin := s.second()
	res, body := admin.login("admin@example.com", examPassword)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return admin
}

// resultRow returns the Results table row that links to the attempt.
func resultRow(t *testing.T, body, attemptPath string) string {
	t.Helper()
	at := strings.Index(body, `href="/admin/results/`+attemptID(attemptPath)+`"`)
	if at < 0 {
		t.Fatalf("Results has no row for attempt %s: %s", attemptPath, body)
	}
	start := strings.LastIndex(body[:at], "<tr>")
	end := strings.Index(body[at:], "</tr>")
	return body[start : at+end]
}

func TestAdminResultsListEveryAttemptWithItsMarkTimesAndWhoMarkedIt(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("100", "", "2", "Q1", "Q2")
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	s.submitExam(first, map[string]string{qids[0]: "0", qids[1]: "1"})
	s.releaseResults()
	if _, err := s.db.Exec(`UPDATE exam_attempts SET finished_at = '2099-01-02 03:04+00', released_at = '2099-01-02 03:14+00'
		WHERE id = $1`, attemptID(first)); err != nil {
		t.Fatal(err)
	}
	res, _ = s.startExam(id)
	second := res.Header.Get("Location")
	s.submitExam(second, map[string]string{qids[0]: "0", qids[1]: "0"})
	s.setSubmitTime(second, "2099-01-03 03:04:00+00") // never released during the test; Malaysia is UTC+8

	res, body := s.adminSite().get("/admin/results")
	expectStatus(t, res, body, http.StatusOK, "Results")
	// The Exam sits under its Course, who marked it under the Result, and each time under its date, so the table fits.
	expectCells(t, resultRow(t, body, first), "Ada", "ada@example.com", "Go Basics<small>Multiple-choice</small>", "<td>1</td>",
		"50%", "Failed<small>Marked by: Automatic</small>", "2 January 2099<small>11:04</small>", "2 January 2099<small>11:14</small>")
	row := resultRow(t, body, second)
	expectCells(t, row, "Ada", "<td>2</td>", "3 January 2099<small>11:04</small>", "Waiting for release")
	expectNone(t, row, "100%", "Passed", "Failed")
}

func TestAdminResultsFilterByCourseAndExam(t *testing.T) {
	s := newSite(t)
	goID, _ := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(goID)
	adaAttempt := res.Header.Get("Location")
	s.submitExam(adaAttempt, nil)
	admin := s.adminSite()
	sqlID := admin.examOnlyCourse("SQL Basics", "50")
	bea := s.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", examPassword)
	bea.setICFor("bea@example.com")
	bea.joinAndFinish(sqlID, "SQL Basics")
	res, _ = bea.startExam(sqlID)
	beaAttempt := res.Header.Get("Location")
	bea.submitExam(beaAttempt, nil)

	_, body := admin.get("/admin/results")
	resultRow(t, body, adaAttempt)
	resultRow(t, body, beaAttempt)
	expectAll(t, body, `<option value="`+goID+`">Go Basics</option>`, `<option value="`+sqlID+`">SQL Basics</option>`)

	res, body = admin.get("/admin/results?course=" + sqlID + "&format=multiple-choice")
	expectStatus(t, res, body, http.StatusOK, `<option value="`+sqlID+`" selected>SQL Basics</option>`)
	resultRow(t, body, beaAttempt)
	expectNone(t, content(body), "ada@example.com")

	_, body = admin.get("/admin/results?course=" + goID + "&format=written")
	expectAll(t, body, "No attempts.")
	expectNone(t, content(body), "ada@example.com", "bea@example.com")
}

// questionBlock returns the part of an attempt's Admin page about the question with the prompt.
func questionBlock(t *testing.T, body, prompt string) string {
	t.Helper()
	at := strings.Index(body, prompt)
	end := strings.Index(body[max(at, 0):], "</section>")
	if at < 0 || end < 0 {
		t.Fatalf("the attempt page has no question %q: %s", prompt, body)
	}
	return body[at : at+end]
}

func TestAnAttemptOpensToShowItsQuestionsTheLearnersAnswersAndTheCorrectOptions(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Got it right", "Got it wrong", "Left blank")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	s.releaseResults()
	admin := s.adminSite()

	res, body := admin.get("/admin/results/" + attemptID(attempt))
	expectStatus(t, res, body, http.StatusOK, "Ada")
	expectAll(t, body, "ada@example.com", "Go Basics", "Multiple-choice", "Attempt 1", "33%", "Failed")
	expectInOrder(t, questionBlock(t, body, "Got it right"), "Right answer (correct) (Learner&#39;s answer)", "Wrong answer")
	expectInOrder(t, questionBlock(t, body, "Got it wrong"), "Right answer (correct)", "Wrong answer (Learner&#39;s answer)")
	block := questionBlock(t, body, "Left blank")
	expectAll(t, block, "Not answered", "Right answer (correct)")
	expectNone(t, block, "Learner&#39;s answer")

	res, body = admin.get("/admin/results/999999")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestGiveAnotherAttemptLetsALearnerWhoUsedEveryAttemptStartOnceMoreWithTheSameQuestions(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("100", "", "1", "Q1", "Q2", "Q3", "Q4")
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	_, body := s.get(first)
	asked := attemptQuestionIDs(body)
	s.submitExam(first, nil)
	admin := s.adminSite()
	detail := "/admin/results/" + attemptID(first)
	another := detail + "/another-attempt"

	// Not before the Result is released and failed.
	_, body = admin.get(detail)
	expectNone(t, body, another)
	res, body = admin.post(another, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.releaseResults()
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "You have no attempts left.")

	_, body = admin.get(detail)
	expectAll(t, body, `action="`+another+`"`, "Give another attempt")
	res, body = admin.post(another, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != detail {
		t.Fatalf("Give another attempt went to %q, want %q", loc, detail)
	}
	_, body = admin.get(detail)
	expectNone(t, body, another)
	admin.post(another, nil) // a second click while the extra attempt is unused adds nothing

	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Attempts left: 1")
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	second := res.Header.Get("Location")
	_, body = s.get(second)
	if got := attemptQuestionIDs(body); !slices.Equal(got, asked) {
		t.Fatalf("the extra attempt asks %v, want the first attempt's %v", got, asked)
	}
	s.submitExam(second, nil)
	s.releaseResults()
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "You have no attempts left.")
}

func TestResultsPagesAndGiveAnotherAttemptRefuseVisitorsAndLearners(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("100", "", "1", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, nil)
	s.releaseResults()
	detail := "/admin/results/" + attemptID(attempt)
	pages := []struct{ method, path string }{
		{http.MethodGet, "/admin/results"},
		{http.MethodGet, detail},
		{http.MethodPost, detail + "/another-attempt"},
		{http.MethodPost, detail + "/marks"},
	}
	check := func(who *site, role string, want int, wantText string) {
		t.Helper()
		for _, p := range pages {
			var res *http.Response
			var body string
			if p.method == http.MethodPost {
				res, body = who.post(p.path, nil)
			} else {
				res, body = who.get(p.path)
			}
			if res.StatusCode != want || !strings.Contains(body, wantText) {
				t.Fatalf("%s %s as %s = %d, want %d containing %q", p.method, p.path, role, res.StatusCode, want, wantText)
			}
		}
	}
	visitor := s.second()
	check(visitor, "a Visitor", http.StatusSeeOther, "")
	check(s, "a Learner", http.StatusForbidden, "Only the Admin can open this page")

	res, body := s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "You have no attempts left.")
}
