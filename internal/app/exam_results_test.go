package app_test

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
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
	expectAll(t, body, "Your Result will be ready by 2 January 2099, 11:14", "emailed to you")
	expectNone(t, body, "Congratulations", "Score", "50%", "Passed", "Failed", "Right one", "Wrong one", "Right answer", "Wrong answer")

	res, body = s.get("/courses/" + id + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Submitted. Your Result will be ready by 2 January 2099, 11:14.")
	expectAll(t, body, "No score yet")
	expectNone(t, body, "Best score:", "Start the Exam", "You have passed") // the bare "Best score" label shows over "-"
}

func TestAFailedResultIsReleasedAMinuteAfterSubmittingOnceAndEmailedInTheLearnersLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("100", "", "", "Right one", "Wrong one")
	s.switchLanguage("ms", "/") // saved on Ada's account
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
	before := len(s.emailsTo("ada@example.com"))

	s.ageSubmit(attempt, 50)
	s.runReleaseJob()
	if n := len(s.emailsTo("ada@example.com")); n != before {
		t.Fatalf("%d emails before 1 minute, want %d", n, before)
	}
	_, body := s.getAsking(attempt, "en")
	expectAll(t, body, "Jawapan anda telah dihantar")
	expectNone(t, body, "Tahniah", "Markah: ")

	s.ageSubmit(attempt, 61)
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

// sitAndAge sits an Exam as a new Site would and moves its submit time back by seconds. It returns how many Result
// emails Ada has by then, after the release job has run.
func sitAndAge(t *testing.T, written bool, passMark string, seconds int) int {
	t.Helper()
	s := newSite(t)
	var attempt string
	if written {
		id := s.writtenCourse(passMark, "Explain :=")
		s.switchLanguage("en", "/")       // saved on Ada's account, so her Result emails say "Exam Result"
		attempt, _ = s.sitWritten(id, "") // a blank answer needs no marking and scores 0
		attempt = strings.TrimSuffix(attempt, "?format=written")
	} else {
		id, qids := s.examCourse(passMark, "", "", "Right one", "Wrong one")
		s.switchLanguage("en", "/")
		res, _ := s.startExam(id)
		attempt = res.Header.Get("Location")
		s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"}) // 50%
	}
	s.ageSubmit(attempt, seconds)
	s.runReleaseJob()
	return len(s.resultEmailsTo("ada@example.com"))
}

func TestOnlyAFailedMultipleChoiceResultIsReleasedAfterAMinuteEveryOtherAfterTenMinutes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		written  bool
		passMark string
		delay    int // seconds
	}{
		{"failed Multiple-choice", false, "100", 60},
		{"passed Multiple-choice", false, "50", 600},
		{"failed Written", true, "50", 600},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if n := sitAndAge(t, c.written, c.passMark, c.delay-10); n != 0 {
				t.Fatalf("%d Result emails 10 seconds before %d seconds, want 0", n, c.delay)
			}
			if n := sitAndAge(t, c.written, c.passMark, c.delay+1); n != 1 {
				t.Fatalf("%d Result emails after %d seconds, want 1", n, c.delay)
			}
		})
	}
}

func TestWhilePendingAPassedAndAFailedResultShowTheSameReadyTime(t *testing.T) {
	t.Parallel()
	for _, passMark := range []string{"50", "100"} { // the attempt scores 50%: a pass, then a fail
		s := newSite(t)
		id, qids := s.examCourse(passMark, "", "", "Right one", "Wrong one")
		res, _ := s.startExam(id)
		attempt := res.Header.Get("Location")
		s.submitExam(attempt, map[string]string{qids[0]: "0", qids[1]: "1"})
		s.setSubmitTime(attempt, "2099-01-02 03:04:00+00")
		_, body := s.getAsking("/courses/"+id+"/exam", "en")
		expectAll(t, body, "Submitted. Your Result will be ready by 2 January 2099, 11:14.")
		_, body = s.getAsking("/courses/"+id+"/exam", "ms")
		expectAll(t, body, "Telah dihantar. Keputusan anda akan sedia selewat-lewatnya pada ", "11:14.")
	}
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
	expectStatus(t, res, body, http.StatusConflict, "Submitted. Your Result will be ready by")
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
	s.ageAttempt(attempt, 100) // the tab was closed; the one-minute limit and its grace are over, but its minute to release is not

	s.runReleaseJob()
	res, body := s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Time ran out before your answers were sent, so none of them count.")
	expectAll(t, body, "Your Result will be ready by")
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
	admin.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 2, Reason: "Too vague."}})
	s.releaseResults() // the Written Exam is still to pass, so no Certificate yet

	_, body := s.getAsking(attempt, "en")
	main := content(body)
	expectAll(t, main, "Congratulations, you passed (pass mark 50%)")
	if n := strings.Count(main, "100%"); n != 1 {
		t.Fatalf("passed Result shows the score %d times, want once: %s", n, main)
	}
	expectNone(t, main, "Score: ", "cert-art", "/pdf", "PCC-")

	written, wqids := s.sitWritten(id, "My own words")
	s.releaseResults()
	admin.saveMarks(written, map[string]string{wqids[0]: "10"}) // the Certificate is issued now
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

// twoLearnersWithResultsDue gives Ada and Bea each a passing attempt on the same Course, old enough to be released, and
// returns their attempt paths.
func twoLearnersWithResultsDue(s *site) (ada, bea string) {
	s.t.Helper()
	id, qids := s.examCourse("50", "", "", "Q1")
	res, _ := s.startExam(id)
	ada = res.Header.Get("Location")
	s.submitExam(ada, map[string]string{qids[0]: "0"})
	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", examPassword)
	s.setICFor("bea@example.com")
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	res, _ = s.startExam(id)
	bea = res.Header.Get("Location")
	s.submitExam(bea, map[string]string{qids[0]: "0"})
	s.ageSubmit(ada, 600)
	s.ageSubmit(bea, 600)
	return ada, bea
}

// emailSubjects lists the emails as "to: subject", oldest first.
func emailSubjects(msgs []mailer.Message) []string {
	var subjects []string
	for _, m := range msgs {
		subjects = append(subjects, m.To+": "+m.Subject)
	}
	return subjects
}

// Neither Learner has picked a language, so their emails are in Malay, and each one's Certificate comes before their Result.
var adaAndBeaEmails = []string{
	"ada@example.com: Sijil anda untuk Go Basics", "ada@example.com: Keputusan Peperiksaan Objektif anda untuk Go Basics",
	"bea@example.com: Sijil anda untuk Go Basics", "bea@example.com: Keputusan Peperiksaan Objektif anda untuk Go Basics",
}

func TestASlowMailServiceDoesNotDelayReleasingResultsAndTheirCertificates(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	ada, bea := twoLearnersWithResultsDue(s)

	before := len(s.mail.Messages())
	release := s.mail.HoldSends()
	t.Cleanup(release)
	released := make(chan struct{})
	go func() {
		s.application.ReleaseResultsWithoutWaiting(context.Background())
		close(released)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the release job is still waiting on the mail service after 5 seconds")
	}
	for _, p := range []struct{ email, attempt string }{{"bea@example.com", bea}, {"ada@example.com", ada}} {
		s.logOut()
		s.login(p.email, examPassword)
		res, body := s.get(p.attempt)
		expectStatus(t, res, body, http.StatusOK, `class="x-score">100%<`)
		_, body = s.get("/my-certificates")
		expectAll(t, body, "Go Basics")
	}
	if n := len(s.mail.Messages()); n != before {
		t.Fatalf("%d emails sent while the mail service was held, want %d", n, before)
	}

	release()
	s.application.WaitForResultEmails()
	if got := emailSubjects(s.mail.Messages()[before:]); !slices.Equal(got, adaAndBeaEmails) {
		t.Fatalf("emails after the mail service came back = %q, want %q", got, adaAndBeaEmails)
	}
}

// aRestartOf is a second site on the same database as s, with its own mail service, as the server is after a restart.
func aRestartOf(s *site) *site {
	s.t.Helper()
	return newSite(s.t, func(c *config.Config) { c.DatabaseURL = s.cfg.DatabaseURL })
}

func TestEmailsOwedWhenTheServerRestartedAreSentAfterTheNextStart(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	twoLearnersWithResultsDue(s)
	release := s.mail.HoldSends()
	t.Cleanup(release) // lets the stopped server's one stuck send end before it closes
	s.application.ReleaseResultsWithoutWaiting(context.Background())

	after := aRestartOf(s)
	var got []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got = emailSubjects(after.mail.Messages()); len(got) >= len(adaAndBeaEmails) {
			break
		}
	}
	after.application.WaitForResultEmails()
	if got = emailSubjects(after.mail.Messages()); !slices.Equal(got, adaAndBeaEmails) {
		t.Fatalf("emails sent after the restart = %q, want %q", got, adaAndBeaEmails)
	}
}

func TestAnEmailAlreadySentIsNotSentAgainOnALaterTickOrRestart(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	twoLearnersWithResultsDue(s)
	before := len(s.mail.Messages())
	s.runReleaseJob()
	if got := emailSubjects(s.mail.Messages()[before:]); !slices.Equal(got, adaAndBeaEmails) {
		t.Fatalf("emails sent by the release = %q, want %q", got, adaAndBeaEmails)
	}

	s.runReleaseJob()
	after := aRestartOf(s)
	after.runReleaseJob()
	if n := len(s.mail.Messages()) - before; n != len(adaAndBeaEmails) {
		t.Fatalf("%d emails after a later tick, want %d", n, len(adaAndBeaEmails))
	}
	if got := emailSubjects(after.mail.Messages()); len(got) != 0 {
		t.Fatalf("emails sent again after a restart = %q, want none", got)
	}
}
