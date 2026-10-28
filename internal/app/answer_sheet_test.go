package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestOnlyThePassResultEmailCarriesTheAnswerSheetWithHeadingsInTheSavedLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "What does := do?", "What does go vet do?")
	s.switchLanguage("en", "/") // saved on Ada's account
	res, _ := s.startExam(id)
	first := res.Header.Get("Location")
	s.submitExam(first, map[string]string{qids[0]: "1", qids[1]: "1"})
	s.releaseResults()

	fail := s.lastEmailTo("ada@example.com")
	expectEmail(t, fail, "Your Multiple-choice Exam Result for Go Basics", "Failed (pass mark 50%)")
	if len(fail.Attachments) != 0 {
		t.Fatalf("the fail email carries %d attachments, want none", len(fail.Attachments))
	}

	res, _ = s.startExam(id)
	second := res.Header.Get("Location")
	s.submitExam(second, map[string]string{qids[0]: "0", qids[1]: "1"})
	s.releaseResults()

	pass := s.lastEmailTo("ada@example.com")
	expectEmail(t, pass, "Your Multiple-choice Exam Result for Go Basics", "Passed (pass mark 50%)")
	expectAll(t, pass.Body, "Your Answer sheet is attached.")
	if len(pass.Attachments) != 1 {
		t.Fatalf("the pass email carries %d attachments, want the Answer sheet", len(pass.Attachments))
	}
	sheet := pass.Attachments[0]
	if !strings.HasSuffix(sheet.Filename, ".pdf") || !strings.HasPrefix(string(sheet.Content), "%PDF") {
		t.Fatalf("attachment %q starts %.20q, want a PDF", sheet.Filename, sheet.Content)
	}
	expectAll(t, string(sheet.Content), "Answer sheet", "Ada", "Go Basics", "Exam: Multiple-choice", "Score: 50%",
		"What does := do?", "What does go vet do?", "Your answer: Right answer", "Your answer: Wrong answer",
		"Correct answer: Right answer")
	expectNone(t, string(sheet.Content), "Skema Jawapan", "Correct answer: Wrong answer")
}

func TestOnlyAPassedLearnerDownloadsTheirAnswerSheetFromTheExamPage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("50", "", "", "What does := do?")
	s.switchLanguage("ms", "/") // saved on Ada's account; the download still asks for English
	sheet := "/courses/" + id + "/exam/answer-sheet"
	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	s.submitExam(attempt, map[string]string{qids[0]: "0"})

	res, body := s.get(sheet) // submitted, its Result not yet released
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get("/courses/" + id + "/exam")
	expectNone(t, body, sheet)

	s.releaseResults()
	_, body = s.get("/courses/" + id + "/exam")
	expectAll(t, body, `href="`+sheet+`"`, "Muat turun Skema Jawapan anda")
	pdf := s.pdfBody(sheet)
	expectAll(t, pdf, "Skema Jawapan", "Pelajar: Ada", "Kursus: Go Basics", "Peperiksaan: Objektif", "Markah: 100%",
		"What does := do?", "Jawapan anda: Right answer", "Jawapan betul: Right answer")

	other := s.second()
	other.loginAsNewLearner("Bob", "bob@example.com", examPassword)
	other.post("/courses/"+id+"/join", nil)
	res, body = other.get(sheet)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestAFailedLearnerIsRefusedTheAnswerSheet(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id, qids := s.examCourse("100", "", "", "What does := do?")
	res, _ := s.startExam(id)
	s.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "1"})
	s.releaseResults()

	res, body := s.get("/courses/" + id + "/exam/answer-sheet")
	expectStatus(t, res, body, http.StatusNotFound, "")
	expectNone(t, body, "%PDF", "Right answer")
	_, body = s.get("/courses/" + id + "/exam")
	expectNone(t, body, "answer-sheet", "Skema Jawapan")
}
