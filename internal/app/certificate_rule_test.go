package app_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestACertificateNeedsEveryExamPassedAndNeverComesBeforeTheResult(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "Which is right?")
	admin := s.adminSite()
	admin.saveWrittenQuestion("/admin/courses/"+id+"/exam/questions?format=written", "Explain :=", "It declares and assigns.")
	s.setIC("951206085555")
	s.ai.ReplyMarks(testutil.FakeMark{Marking: aiwriter.Marking{Mark: 2, Reason: "Too vague."}})

	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	written, wqids := s.sitWritten(id, "My own words")
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.") // submitted, not released

	s.releaseResults() // Multiple-choice passed, Written failed
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")

	admin.saveMarks(written, map[string]string{wqids[0]: "10"})
	_, body = s.get("/my-certificates")
	if ids := certificateIDs(body); len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates after passing both Exams, want 1: %s", len(ids), body)
	}
}

func TestALearnerWithoutAnICIsToldWhyAndGetsTheCertificateOnSettingIt(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", certPassword)
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	_, body := s.getAsking("/my-certificates", "en")
	expectAll(t, content(body), "You have no Certificates yet.", "A Certificate can be issued only once you set your IC or passport number", `href="/my-profile"`)
	_, body = s.getAsking("/my-certificates", "ms")
	expectAll(t, content(body), "Sijil hanya boleh dikeluarkan selepas anda menetapkan No. KP atau pasport anda")

	s.setIC("951206085555")
	_, body = s.getAsking("/my-certificates", "en")
	if ids := certificateIDs(body); len(ids) != 1 {
		t.Fatalf("Bea has %d Certificates after setting her IC, want 1: %s", len(ids), body)
	}
	expectNone(t, body, "set your IC or passport number")
}

// liveDates are yesterday and tomorrow in Malaysia, so a Live Course on them is open and its Exams can be sat.
func liveDates() (start, end string) {
	today := time.Now().In(time.FixedZone("MYT", 8*3600))
	return today.AddDate(0, 0, -1).Format(time.DateOnly), today.AddDate(0, 0, 1).Format(time.DateOnly)
}

// liveCourse makes a published Live Course of the given days, running yesterday to tomorrow in Malaysia, copied
// from a Course with one Lesson and one Multiple-choice question. Ada joins it, sets her IC and completes the Lesson.
// It returns the Admin's and Ada's sites, the Course ID and its question IDs.
func liveCourse(t *testing.T, days string) (admin, ada *site, id string, qids []string) {
	t.Helper()
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	start, end := liveDates()
	id = admin.copiedCourseID(admin.courseWithContent(), "Go for Acme", start, end, days)
	admin.post("/admin/courses/"+id+"/publish", nil)
	qids = admin.examQuestionIDs(id)
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	ada.post("/courses/"+id+"/join", nil)
	ada.setIC("951206085555")
	var lesson string // the copy's Lesson: lessonIDByTitle would find the original's too
	if err := admin.db.QueryRow(`SELECT id FROM lessons WHERE course_id = $1`, id).Scan(&lesson); err != nil {
		t.Fatal(err)
	}
	ada.markLesson(id, lesson, true)
	return admin, ada, id, qids
}

func TestALiveCourseCertificateNeedsNoLessonsAndKeepsTheICAndPCCLevelFromIssue(t *testing.T) {
	admin, ada, id, qids := liveCourse(t, "2")
	res, body := ada.startExam(id)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.addTextLesson(id, "Functions", "", "Use func.", false) // a self-paced Course would now hold the Certificate

	ada.releaseResults()
	_, body = ada.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates after passing the Live Course's Exam, want 1: %s", len(ids), body)
	}
	certID := ids[0]
	expectAll(t, ada.pdfBody("/certificates/"+certID+"/pdf"), "951206-08-5555")
	_, body = ada.get("/certificates/" + certID)
	expectAll(t, body, "PCC Level: Foundation", `alt="PCC Foundation badge"`)

	_, body = admin.get("/admin/courses/" + id)
	correct := correctICLink.FindStringSubmatch(adminCourseRow(t, body, "ada@example.com"))[1]
	admin.post(correct, url.Values{"ic": {"951206-08-5556"}})
	start, end := liveDates()
	edit := url.Values{"title": {"Go for Acme"}, "description": {"For Acme."}, "start_date": {start}, "end_date": {end}, "days": {"4"}}
	res, body = admin.post("/admin/courses/"+id, edit)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	pdf := ada.pdfBody("/certificates/" + certID + "/pdf")
	expectAll(t, pdf, "951206-08-5555")
	expectNone(t, pdf, "951206-08-5556")
	_, body = ada.get("/certificates/" + certID)
	expectAll(t, body, "PCC Level: Foundation")

	admin.post("/admin/certificates/"+certID+"/revoke", confirming)
	res, body = admin.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/my-certificates")
	ids = certificateIDs(body)
	newID := ids[len(ids)-1]
	expectAll(t, ada.pdfBody("/certificates/"+newID+"/pdf"), "951206-08-5556", "Ada Lovelace")
	_, body = ada.get("/certificates/" + newID)
	expectAll(t, body, "PCC Level: Professional")
}
