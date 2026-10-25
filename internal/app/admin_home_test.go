package app_test

import (
	"net/url"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
)

func TestTheAdminHomeCountsSubmissionsToMark(t *testing.T) {
	admin, ada, path := publishedAssignment(t)
	_, body := admin.get("/admin")
	expectAll(t, body, "Nothing needs marking.")

	ada.postFiles(path+"/files", upload{"prompt.pdf", pdfHandIn})
	aid := admin.assignmentIDByTitle("Write a prompt")
	_, body = admin.get("/admin")
	expectAll(t, body, "1 Submission to mark", `href="/admin/assignments/`+aid+`"`, "Write a prompt")
	expectNone(t, body, "Nothing needs marking.")

	admin.post("/admin/assignments/"+aid+"/learners/"+admin.adaID()+"/comment", url.Values{"comment": {"Well done."}})
	_, body = admin.get("/admin")
	expectAll(t, body, "Nothing needs marking.")

	// A file handed in after the Admin's comment needs marking again.
	ada.postFiles(path+"/files", upload{"notes.docx", docxHandIn})
	_, body = admin.get("/admin")
	expectAll(t, body, "1 Submission to mark")
}

func TestTheAdminHomeCountsWrittenAnswersAwaitingMarkingAndListsRecentResults(t *testing.T) {
	s := newSite(t)
	s.ai.FailMarks(aiwriter.ErrUnavailable)
	id := s.writtenCourse("50", "Explain :=", "Explain go vet")
	attempt, qids := s.sitWritten(id, "My own words")
	admin := s.adminSite()

	_, body := admin.get("/admin")
	expectAll(t, body, "2 Written answers awaiting marking", `href="/admin/results/`+writtenAttemptID(attempt)+`"`, "Ada", "No Results released yet.")
	admin.saveMarks(attempt, map[string]string{qids[0]: "10"})
	_, body = admin.get("/admin")
	expectAll(t, body, "1 Written answer awaiting marking")
	admin.saveMarks(attempt, map[string]string{qids[1]: "5"})
	_, body = admin.get("/admin")
	expectAll(t, body, "Nothing needs marking.")

	s.releaseResults()
	_, body = admin.get("/admin")
	expectInOrder(t, body, "Recent Results", `href="/admin/results"`, "Ada", "Go Basics", "75%", "Passed")
	expectNone(t, body, "No Results released yet.")
}

func TestTheAdminHomeCountsLearnersCoursesAndValidCertificates(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	_, body := s.get("/admin")
	expectInOrder(t, body, `<dl class="dash-counts">`, "Learners", ">1<", "Courses", ">1<", "Certificates", ">1<", "</dl>")

	s.post("/admin/certificates/"+certID+"/revoke", confirming)
	_, body = s.get("/admin")
	expectInOrder(t, body, `<dl class="dash-counts">`, "Certificates", ">0<", "</dl>")
}
