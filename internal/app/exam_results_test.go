package app_test

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

// attemptID is the last part of an attempt's path.
func attemptID(attemptPath string) string {
	return attemptPath[strings.LastIndex(attemptPath, "/")+1:]
}

// setSubmitTime sets when an attempt was submitted, straight in the database. at is a Postgres timestamp.
func (s *site) setSubmitTime(attemptPath, at string) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE exam_attempts SET finished_at = $2 WHERE id = $1`, attemptID(attemptPath), at); err != nil {
		s.t.Fatal(err)
	}
}

// ageSubmit moves an attempt's submit time back by the given seconds, straight in the database.
func (s *site) ageSubmit(attemptPath string, seconds int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE exam_attempts SET finished_at = now() - make_interval(secs => $2) WHERE id = $1`,
		attemptID(attemptPath), seconds); err != nil {
		s.t.Fatal(err)
	}
}

// runReleaseJob runs the minutely Result release now.
func (s *site) runReleaseJob() {
	s.t.Helper()
	s.application.RunReleaseResults(context.Background())
}

// resultEmailsTo lists the English Exam Result emails sent to `to`, oldest first, leaving out the Certificate email
// a pass may also send.
func (s *site) resultEmailsTo(to string) []mailer.Message {
	var got []mailer.Message
	for _, m := range s.emailsTo(to) {
		if strings.Contains(m.Subject, "Exam Result") {
			got = append(got, m)
		}
	}
	return got
}

// emailsTo lists every email sent to `to`, oldest first.
func (s *site) emailsTo(to string) []mailer.Message {
	var got []mailer.Message
	for _, m := range s.mail.Messages() {
		if m.To == to {
			got = append(got, m)
		}
	}
	return got
}

func TestSubmittingSaysTheAnswersAreInAndWhenTheResultIsReadyButNoMarkAndNoCongratulations(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Right one", "Wrong one")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")

	res, body := s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != attempt {
		t.Fatalf("submit went to %q, want the attempt %q", loc, attempt)
	}
	// Far in the future, so the minutely release job never releases it during the test. Malaysia is UTC+8.
	s.setSubmitTime(attempt, "2099-01-02 03:04:00+00")

	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Your answers are in")
	expectAll(t, body, "Your Result is ready at 2 January 2099, 11:14", "emailed to you")
	expectNone(t, body, "Congratulations", "Score", "50%", "Passed", "Failed", "Right one", "Wrong one", "Right answer", "Wrong answer")

	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Submitted, your Result is ready at 2 January 2099, 11:14.")
	expectAll(t, body, "No score yet")
	expectNone(t, body, "Best score:", "Start the Exam", "You have passed") // the bare "Best score" label shows over "-"
}

func TestTheResultIsReleasedTenMinutesAfterSubmittingOnceAndEmailedInTheLearnersLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("100", "", "", "Right one", "Wrong one")
	s.switchLanguage("ms", "/") // saved on Ada's account
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	before := len(s.emailsTo("ada@example.com"))

	s.ageSubmit(attempt, 9*60+50)
	s.runReleaseJob()
	if n := len(s.emailsTo("ada@example.com")); n != before {
		t.Fatalf("%d emails before 10 minutes, want %d", n, before)
	}
	_, body := s.getAsking(attempt, "en")
	expectAll(t, body, "Jawapan anda telah dihantar")
	expectNone(t, body, "Tahniah", "Markah: ")

	s.ageSubmit(attempt, 10*60+1)
	s.runReleaseJob()
	s.runReleaseJob()
	emails := s.emailsTo("ada@example.com")[before:]
	if len(emails) != 1 {
		t.Fatalf("%d Result emails, want 1: %#v", len(emails), emails)
	}
	expectEmail(t, emails[0], "Keputusan Peperiksaan Objektif anda untuk Go Basics", "Markah: 50%")
	expectAll(t, emails[0].Body, "Gagal (markah lulus 100%)", "https://cave.example.invalid"+attempt)

	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, `class="x-score">50%<`)
	expectAll(t, body, "Gagal (markah lulus 100%)")
	for _, shown := range []string{content(body), emails[0].Body} {
		expectNone(t, shown, "Right one", "Wrong one", "Right answer", "Wrong answer", "Jawapan")
	}
	_, body = s.get("/courses/" + id + "/exam")
	expectAll(t, body, "Markah terbaik: 50%", "Percubaan berbaki: Tanpa had", "Mulakan Peperiksaan")
}

// releaseResults moves every submitted or timed-out, unreleased attempt back 10 minutes, as if the Learner had
// waited, then runs the release job.
func (s *site) releaseResults() {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE exam_attempts SET started_at = started_at - interval '10 minutes',
		finished_at = finished_at - interval '10 minutes' WHERE released_at IS NULL
		AND (finished_at IS NOT NULL OR started_at + make_interval(mins => time_limit_minutes) < now())`); err != nil {
		s.t.Fatal(err)
	}
	s.runReleaseJob()
}

func TestAttemptTwoOpensOnlyOnceAttemptOneIsReleasedAndFailed(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("100", "", "2", "Q1")
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	s.submitExam(first, map[string]string{qids[0]: "1"})

	res, body := s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "Submitted, your Result is ready at")
	expectAll(t, body, "Attempts left: 1")

	s.releaseResults()
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if second := res.Header.Get("Location"); second == first {
		t.Fatalf("a second start after a released fail went back to %q", first)
	}
}

func TestTheReleaseJobSubmitsAnAttemptLeftOpenPastItsTimeLimitWithNoAnswers(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "1", "", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.ageAttempt(attempt, 120) // the tab was closed; the one-minute limit and its grace are over

	s.runReleaseJob()
	res, body := s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Time ran out before your answers were sent, so none of them count.")
	expectAll(t, body, "Your Result is ready at")
	expectNone(t, body, "Score")
	res, body = s.submitExam(attempt, map[string]string{qids[0]: "0"}) // too late to count
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.releaseResults()
	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, `class="x-score">0%<`)
	expectAll(t, body, "Failed (pass mark 50%)")
	// Ada never picked a language, so the email is in Malay, the default.
	expectEmail(t, s.lastEmailTo("ada@example.com"), "Keputusan Peperiksaan Objektif anda untuk Go Basics", "Markah: 0%")
}

var secondsLeft = regexp.MustCompile(`data-seconds-left="(\d+)"`)

func TestTheAttemptPageCountsDownAndSubmitsTheFormAtZero(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, _ := s.examCourse("50", "1", "", "Q1")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.ageAttempt(attempt, 20)

	_, body := s.get(attempt)
	m := secondsLeft.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the attempt page has no countdown: %s", body)
	}
	if left, _ := strconv.Atoi(m[1]); left < 35 || left > 40 {
		t.Fatalf("countdown starts at %d seconds, want about 40 of the one-minute limit", left)
	}
	// The site's CSP refuses inline scripts, so the timer comes from a static file.
	src := timerScript.FindStringSubmatch(body)
	if src == nil || strings.Contains(body, "form.submit()") {
		t.Fatalf("the attempt page should load the timer from exam-timer.js, not inline: %s", body)
	}
	_, js := s.get(src[1])
	expectAll(t, js, "data-seconds-left", "form.submit()")
}

var timerScript = regexp.MustCompile(`<script src="(/static/exam-timer\.js[^"]*)" defer></script>`)

func TestALearnerWhoSitsBothExamsGetsResultsThatNameEachExamInTheirLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Which is right?")
	admin := s.adminSite()
	admin.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	s.switchLanguage("ms", "/") // saved on Ada's account
	res, _ := s.startExam(id)
	choice := res.Header.Get("Location")
	s.submitExam(choice, map[string]string{qids[0]: "0"})
	written, wqids := s.sitWritten(id, "My own words")
	before := len(s.emailsTo("ada@example.com"))

	s.releaseResults()
	subjects := map[string]string{}
	for _, m := range s.emailsTo("ada@example.com")[before:] {
		if strings.HasPrefix(m.Subject, "Keputusan") { // passing both also issues her Certificate, emailed too
			subjects[m.Subject] = m.Body
		}
	}
	for subject, body := range map[string]string{
		"Keputusan Peperiksaan Objektif anda untuk Go Basics":  "Keputusan Peperiksaan Objektif anda untuk Go Basics sudah sedia.",
		"Keputusan Peperiksaan Subjektif anda untuk Go Basics": "Keputusan Peperiksaan Subjektif anda untuk Go Basics sudah sedia.",
	} {
		if !strings.Contains(subjects[subject], body) || len(subjects) != 2 {
			t.Fatalf("Result emails = %#v, want one titled %q saying %q", subjects, subject, body)
		}
	}
	_, body := s.get(choice)
	expectAll(t, body, "<h1>Peperiksaan Objektif untuk Go Basics</h1>")
	_, body = s.get(written)
	expectAll(t, body, "<h1>Peperiksaan Subjektif untuk Go Basics</h1>")

	s.switchLanguage("en", "/")
	_, body = s.get(choice)
	expectAll(t, body, "<h1>Multiple-choice Exam for Go Basics</h1>")
	_, body = s.get(written)
	expectAll(t, body, "<h1>Written Exam for Go Basics</h1>")
	admin.saveMarks(written, map[string]string{wqids[0]: "0"}) // the AI's pass becomes a fail
	email := s.lastEmailTo("ada@example.com")
	expectEmail(t, email, "Your corrected Written Exam Result for Go Basics", "Your Result for the Written Exam for Go Basics is ready.")
}

// A passed Result is the peak: it congratulates the Learner, shows the score once and, once the Course's Certificate
// is issued to them, leads to it with its number and a thumbnail. Until it is issued, or once it is revoked, there
// is no Certificate to show.
func TestAPassedResultShowsTheScoreOnceAndLeadsToTheIssuedCertificate(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Right one", "Wrong one")
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "0"})
	admin := s.adminSite()
	admin.addTextLesson(id, "Loops", "", "Use `for`.", false) // Ada has not finished it, so no Certificate yet
	s.releaseResults()

	_, body := s.getAsking(attempt, "en")
	main := content(body)
	expectAll(t, main, "Congratulations, you passed (pass mark 50%)")
	if n := strings.Count(main, "100%"); n != 1 {
		t.Fatalf("passed Result shows the score %d times, want once: %s", n, main)
	}
	expectNone(t, main, "Score: ", "cert-art", "/pdf", "PCC-")

	s.markLesson(id, s.lessonIDByTitle("Loops"), true) // the Certificate is issued now
	_, mine := s.get("/my-certificates")
	certID := certificateIDs(mine)[0]
	for lang, want := range map[string][]string{
		"en": {"Congratulations, you passed", "Certificate No. PCC-", "View your Certificate"},
		"ms": {"Tahniah, anda lulus", "No. Sijil PCC-", "Lihat Sijil anda"},
	} {
		_, body = s.getAsking(attempt, lang)
		main = content(body)
		expectInOrder(t, main, append([]string{`class="x-score"`, "x-verdict", "cert-art"}, want[1:]...)...)
		expectAll(t, main, want[0], `href="/certificates/`+certID+`/pdf"`)
		if n := strings.Count(main, "btn-solid"); n != 1 {
			t.Errorf("%s passed Result has %d primary buttons, want only the Certificate's", lang, n)
		}
	}

	admin.post("/admin/certificates/"+certID+"/revoke", confirming)
	_, body = s.getAsking(attempt, "en")
	main = content(body)
	expectAll(t, main, "Congratulations, you passed")
	expectNone(t, main, "cert-art", certID)
}
