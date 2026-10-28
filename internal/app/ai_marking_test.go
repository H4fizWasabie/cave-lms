package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestTheAIMarksEachWrittenAnswerSentWithOnlyItsQuestionAndModelAnswer(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id := s.writtenCourse("70", "Explain :=", "Explain go vet")
	s.switchLanguage("en", "/") // saved on Ada's account
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 10, Reason: "Says it declares and assigns."}},
		testutil.FakeMark{Marking: aiwriter.Marking{Mark: 5, Reason: "Misses what vet reports."}})
	attempt, _ := s.sitWritten(id, "My own words")

	s.releaseResults()
	got := map[aiwriter.MarkRequest]int{}
	for _, r := range s.ai.MarkRequests() {
		got[r]++
	}
	for _, p := range []string{"Explain :=", "Explain go vet"} {
		if want := (aiwriter.MarkRequest{Question: p, ModelAnswer: "Model answer to " + p, Answer: "My own words"}); got[want] != 1 || len(got) != 2 {
			t.Fatalf("the AI writer was asked to mark %#v, want each answer once with only its question and model answer", got)
		}
	}
	email := s.lastEmailTo("ada@example.com")
	expectEmail(t, email, "Your Written Exam Result for Go Basics", "Score: 75%")
	expectAll(t, email.Body, "Passed (pass mark 70%)")
	expectNone(t, email.Body, "Says it declares", "Misses what vet")
	if len(email.Attachments) != 1 {
		t.Fatalf("the pass email carries %d attachments, want the Answer sheet", len(email.Attachments))
	}
	expectAll(t, string(email.Attachments[0].Content), "Model answer: Model answer to Explain :=", "Mark: 10 / 10",
		"Reason: Says it declares and assigns.", "Mark: 5 / 10", "Reason: Misses what vet reports.")

	admin := s.adminSite()
	_, body := admin.get("/admin/results")
	expectCells(t, resultRow(t, body, writtenAttemptID(attempt)), "75%", "Passed", "AI")
	res, body := admin.get("/admin/results/" + writtenAttemptID(attempt))
	expectStatus(t, res, body, http.StatusOK, "Marked by: AI")
	expectAll(t, body, "Reason: Says it declares and assigns.", "Mark: 10 / 10")
}

// ageMarkingTry moves back by the given seconds when the AI was last asked to mark the attempt, straight in the
// database.
func (s *site) ageMarkingTry(attempt string, seconds int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE exam_attempts SET marking_tried_at = now() - make_interval(secs => $2) WHERE id = $1`,
		writtenAttemptID(attempt), seconds); err != nil {
		s.t.Fatal(err)
	}
}

const markingLate = "still has unmarked Written answers an hour after its release time"

func TestAFailedMarkIsRetriedEveryFiveMinutesAndStillUnmarkedAnHourPastReleaseWritesASystemLogLine(t *testing.T) {
	t.Parallel()
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" })
	id := s.writtenCourse("50", "Explain :=")
	s.switchLanguage("en", "/") // saved on Ada's account
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 11, Reason: "Too generous."}},
		testutil.FakeMark{Marking: aiwriter.Marking{Mark: 3}})
	attempt, _ := s.sitWritten(id, "My own words")
	before := len(s.emailsTo("ada@example.com"))
	admin := s.adminSite()
	asked := func(want int) {
		t.Helper()
		if n := len(s.ai.MarkRequests()); n != want {
			t.Fatalf("the AI writer was asked to mark %d times, want %d", n, want)
		}
	}

	s.releaseResults()
	asked(2) // the wrong shape is asked once more, then left
	if n := len(s.emailsTo("ada@example.com")); n != before {
		t.Fatalf("%d emails with the answer unmarked, want %d", n, before)
	}
	_, body := s.get(attempt)
	expectAll(t, body, "Your Result will be ready by")

	s.ai.FailMarks(aiwriter.ErrUnavailable)
	s.ageMarkingTry(attempt, 4*60+50)
	s.runReleaseJob()
	asked(2)
	s.ageMarkingTry(attempt, 5*60+1)
	s.runReleaseJob()
	asked(3)
	_, body = admin.get("/admin/log")
	expectNone(t, body, markingLate)

	s.ageSubmit(strings.TrimSuffix(attempt, "?format=written"), 69*60+50) // the Result was due 59 minutes ago
	s.runReleaseJob()
	_, body = admin.get("/admin/log")
	expectNone(t, body, markingLate)
	s.ageSubmit(strings.TrimSuffix(attempt, "?format=written"), 70*60+1)
	s.runReleaseJob()
	s.runReleaseJob()
	_, body = admin.get("/admin/log")
	if n := strings.Count(body, markingLate); n != 1 {
		t.Fatalf("the System log says %d times that the attempt is still unmarked, want once: %s", n, body)
	}
	expectAll(t, body, "/admin/results/"+writtenAttemptID(attempt))
	asked(3)

	s.ai.FailMarks(nil)
	s.ageMarkingTry(attempt, 5*60+1)
	s.runReleaseJob()
	asked(4)
	expectEmail(t, s.lastEmailTo("ada@example.com"), "Your Written Exam Result for Go Basics", "Score: 70%")
}

func TestAnAnswerWhoseMarkComesBackInTheWrongShapeThreeRunsIsLeftForTheAdmin(t *testing.T) {
	t.Parallel()
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" })
	id := s.writtenCourse("50", "Explain :=")
	s.switchLanguage("en", "/") // saved on Ada's account
	bad := testutil.FakeMark{Marking: aiwriter.Marking{Mark: 99, Reason: "Out of range."}}
	attempt, qids := s.sitWritten(id, "My own words")
	asked := func(want int) {
		t.Helper()
		if n := len(s.ai.MarkRequests()); n != want {
			t.Fatalf("the AI writer was asked to mark %d times, want %d", n, want)
		}
	}
	retry := func() {
		s.ageMarkingTry(attempt, 5*60+1)
		s.runReleaseJob()
	}

	s.ai.ReplyMarks(bad, bad)
	s.releaseResults()
	asked(2)
	s.ai.FailMarks(aiwriter.ErrUnavailable) // an outage is no wrong shape, so it does not count
	retry()
	asked(3)
	s.ai.FailMarks(nil)
	for _, want := range []int{5, 7} {
		s.ai.ReplyMarks(bad, bad)
		retry()
		asked(want)
	}
	retry() // the third wrong shape was the last: the fake would now give FakeMarking, but is not asked
	retry()
	asked(7)
	s.ageSubmit(strings.TrimSuffix(attempt, "?format=written"), 70*60+1) // the Result was due an hour ago
	s.runReleaseJob()
	asked(7)
	admin := s.adminSite()
	if _, body := admin.get("/admin/log"); strings.Count(body, markingLate) != 1 {
		t.Fatalf("the System log does not say once that the attempt is still unmarked: %s", body)
	}

	admin.saveMarks(attempt, map[string]string{qids[0]: "6"})
	s.runReleaseJob()
	expectEmail(t, s.lastEmailTo("ada@example.com"), "Your Written Exam Result for Go Basics", "Score: 60%")
}

func TestAnAdminsMarkWinsOverTheAIsBeforeAndAfterRelease(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id := s.writtenCourse("50", "Explain :=", "Explain go vet")
	s.switchLanguage("en", "/") // saved on Ada's account
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 2, Reason: "Too vague."}})
	attempt, qids := s.sitWritten(id, "My own words")
	admin := s.adminSite()
	admin.saveMarks(attempt, map[string]string{qids[0]: "4"})

	s.releaseResults()
	if n := len(s.ai.MarkRequests()); n != 1 {
		t.Fatalf("the AI writer was asked to mark %d answers, want only the one the Admin left", n)
	}
	expectEmail(t, s.lastEmailTo("ada@example.com"), "Your Written Exam Result for Go Basics", "Score: 30%")
	sent := len(s.resultEmailsTo("ada@example.com"))

	admin.saveMarks(attempt, map[string]string{qids[1]: "10"}) // overrides the AI's 2, now a pass
	emails := s.resultEmailsTo("ada@example.com")[sent:]
	if len(emails) != 1 || len(emails[0].Attachments) != 1 {
		t.Fatalf("emails after the override made a pass = %#v, want one with the Answer sheet", emails)
	}
	expectEmail(t, emails[0], "Your corrected Written Exam Result for Go Basics", "Score: 70%")
	sheet := string(emails[0].Attachments[0].Content)
	expectAll(t, sheet, "Mark: 4 / 10", "Mark: 10 / 10")
	expectNone(t, sheet, "Too vague.", "Mark: 2 / 10")
	_, body := admin.get("/admin/results")
	expectCells(t, resultRow(t, body, writtenAttemptID(attempt)), "70%", "Passed", "Admin")
}

func TestALaterAttemptStartsWithNoneOfTheAIsReasonsFromTheFirst(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id := s.writtenCourse("100", "Explain :=")
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 2, Reason: "Too vague."}})
	s.sitWritten(id, "My own words")
	s.releaseResults()

	s.ai.FailMarks(aiwriter.ErrUnavailable)
	second, _ := s.sitWritten(id, "Other words")
	s.releaseResults()
	res, body := s.adminSite().get("/admin/results/" + writtenAttemptID(second))
	expectStatus(t, res, body, http.StatusOK, "Not marked yet")
	expectNone(t, body, "Too vague.")
}
