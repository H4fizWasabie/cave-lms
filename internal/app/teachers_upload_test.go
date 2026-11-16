package app_test

import (
	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// uploadOwnDSKP posts the Teachers Portal's DSKP upload form as the logged-in Teacher.
func (s *site) uploadOwnDSKP(subject, year, language string, pdf []byte) (*http.Response, string) {
	s.t.Helper()
	return s.postFile("/teachers/dskps", "dskp.pdf", pdf, url.Values{"subject": {subject}, "year": {year}, "language": {language}})
}

// newProDSKPTeacher is Ada, a Teacher whose trial has ended and whose month of Teachers Portal Pro is running; s is the Admin.
func newProDSKPTeacher(s *site) *site {
	teacher := newDSKPTeacher(s)
	s.ageTrial(20)
	s.giveTeacherPro("ada@example.com", "month")
	return teacher
}

func TestATeacherOnTheTrialCannotUploadADSKPAndIsToldItNeedsPro(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newDSKPTeacher(s)

	_, body := teacher.get("/teachers/dskps")
	expectAll(t, body, "Uploading a DSKP needs Teachers Portal Pro")
	expectNone(t, body, `name="file"`)
	res, body := teacher.uploadOwnDSKP("Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusForbidden, "Uploading a DSKP needs Teachers Portal Pro")
	s.application.WaitForDSKPReads()
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("a trial Teacher's upload was read: %d requests", len(got))
	}
}

func TestAProTeacherUploadsAMissingDSKPAndOnlyTheyCanSeeItUnchecked(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newProDSKPTeacher(s)
	bob := s.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body := bob.saveTeacherProfile("Sejarah", "Y4")
	expectRedirect(t, res, body, "/teachers")

	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, `name="file"`, `action="/teachers/dskps"`, "Upload and read")
	expectNone(t, body, "needs Teachers Portal Pro")

	release := s.ai.HoldDSKPs()
	res, body = teacher.uploadOwnDSKP("Sejarah", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	p := res.Header.Get("Location")
	if !regexp.MustCompile(`^/teachers/dskps/\d+$`).MatchString(p) {
		t.Fatalf("upload led to %q, want the DSKP's page in the portal", p)
	}
	_, body = teacher.get(p)
	expectAll(t, body, "Sejarah", "Year 4", "The AI is reading this DSKP")
	release()
	s.application.WaitForDSKPReads()
	_, body = teacher.get(p)
	expectAll(t, body, "Unchecked", "Tema 1: Kajian Saintifik", "SK 1.1", "SP 1.1.1")
	expectNone(t, body, "Publish", "Published")
	_, body = teacher.get("/teachers/dskps")
	expectInOrder(t, body, "Your uploads", "Sejarah", "Unchecked")
	expectAll(t, body, `href="`+p+`"`)

	// Nobody else sees it, however they ask: not another Teacher, not the Admin's library.
	_, body = bob.get("/teachers/dskps")
	expectNone(t, body, "Sejarah", "Your uploads")
	res, body = bob.get(p)
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "No DSKP has been uploaded yet.")
	adminPath := "/admin/dskps/" + path.Base(p)
	res, body = s.get(adminPath)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.post(adminPath+"/publish", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
	if got := s.ai.DSKPRequests(); len(got) != 1 || got[0].Language != "ms" {
		t.Fatalf("the upload was read %d times, want once in Malay", len(got))
	}
}

// nextMonthStart is the first day of next month in Malaysia time, as the page names it ("1 December 2026").
func nextMonthStart() string {
	n := time.Now().In(malaysiaTime).AddDate(0, 1, 0)
	return "1 " + n.Month().String() + " " + strconv.Itoa(n.Year())
}

// thisMonthStart is when the current calendar month began in Malaysia time.
func thisMonthStart() time.Time {
	n := time.Now().In(malaysiaTime)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, malaysiaTime)
}

// moveDSKPUploads moves the start of every DSKP upload to at.
func (s *site) moveDSKPUploads(at time.Time) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE teacher_dskp_uploads SET started_at = $1`, at); err != nil {
		s.t.Fatal(err)
	}
}

func TestATeacherMayUploadFiveDSKPsAMonthAndTheSixthIsRefusedWithWhenItResets(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newProDSKPTeacher(s)
	_, body := teacher.get("/teachers/dskps")
	expectAll(t, body, "5 of 5 uploads left this month")

	// An upload that is refused is not one started.
	res, body := teacher.uploadOwnDSKP("Sains", "Y4", "ms", jpegFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Upload the DSKP as a PDF file.")
	for i := 1; i <= 5; i++ {
		res, body = teacher.uploadOwnDSKP("Sains "+strconv.Itoa(i), "Y4", "ms", pdfFile)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		_, body = teacher.get("/teachers/dskps")
		expectAll(t, body, strconv.Itoa(5-i)+" of 5 uploads left this month")
	}
	s.application.WaitForDSKPReads()

	res, body = teacher.uploadOwnDSKP("Sains 6", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusTooManyRequests, "You have used your 5 DSKP uploads for this month. You can upload again from "+nextMonthStart()+".")
	_, body = teacher.get("/teachers/dskps")
	expectNone(t, body, `name="file"`)
	expectAll(t, body, "You can upload again from "+nextMonthStart())
	s.application.WaitForDSKPReads()
	if got := s.ai.DSKPRequests(); len(got) != 5 {
		t.Fatalf("the writer was asked %d times, want 5: the sixth upload was refused", len(got))
	}
	_, body = teacher.getAsking("/teachers/dskps", "ms")
	expectLanguage(t, body, "ms", "0 daripada 5 muat naik tinggal bulan ini.")
	expectAll(t, body, "Anda boleh memuat naik semula mulai 1 ")
}

// The month is the calendar month in Malaysia time: eight hours ahead of UTC, so an upload an hour after Malaysia's
// midnight is already this month's although UTC is still in the month before.
func TestTheMonthlyUploadLimitResetsAtMalaysiaMidnightOnTheFirst(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newProDSKPTeacher(s)
	for i := 1; i <= 5; i++ {
		res, body := teacher.uploadOwnDSKP("Sains "+strconv.Itoa(i), "Y4", "ms", pdfFile)
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	s.application.WaitForDSKPReads()

	s.moveDSKPUploads(thisMonthStart().Add(time.Hour))
	res, body := teacher.uploadOwnDSKP("Sains 6", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
	s.moveDSKPUploads(thisMonthStart())
	res, body = teacher.uploadOwnDSKP("Sains 6", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusTooManyRequests, "")

	s.moveDSKPUploads(thisMonthStart().Add(-time.Minute)) // last month's, a minute before Malaysia's midnight
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "5 of 5 uploads left this month")
	res, body = teacher.uploadOwnDSKP("Sains 6", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "4 of 5 uploads left this month")
}

func TestATeacherDeletesTheirOwnUploadAndNobodyElsesAndItStillCountsForTheMonth(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	library := path.Base(s.publishedDSKP("Matematik", "Y5"))
	teacher := newProDSKPTeacher(s)
	bob := s.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body := bob.saveTeacherProfile("Sejarah", "Y4")
	expectRedirect(t, res, body, "/teachers")

	res, body = teacher.uploadOwnDSKP("Sejarah", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	p := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	_, body = teacher.get(p)
	expectAll(t, body, `action="`+p+`/delete"`)

	// Not another Teacher's, and not the library's.
	res, body = bob.post(p+"/delete", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = teacher.post("/teachers/dskps/"+library+"/delete", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = teacher.get("/teachers/dskps/" + library)
	expectAll(t, body, "Matematik")
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "Sejarah", "4 of 5 uploads left this month")

	res, body = teacher.post(p+"/delete", nil)
	expectRedirect(t, res, body, "/teachers/dskps")
	_, body = teacher.get("/teachers/dskps")
	expectNone(t, body, "Sejarah", "Your uploads")
	expectAll(t, body, "4 of 5 uploads left this month") // deleting gives no upload back
	res, body = teacher.get(p)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = teacher.post(p+"/delete", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")

	// Deleted in the middle of its read, it stays deleted.
	release := s.ai.HoldDSKPs()
	res, body = teacher.uploadOwnDSKP("Geografi", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = teacher.post(res.Header.Get("Location")+"/delete", nil)
	expectRedirect(t, res, body, "/teachers/dskps")
	release()
	s.application.WaitForDSKPReads()
	_, body = teacher.get("/teachers/dskps")
	expectNone(t, body, "Geografi")
}

func TestTheAdminSeesWhichDSKPsTeachersUploadedForThemselvesAndHowManyTeachersDid(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	ada := newDSKPTeacher(s)
	bob := s.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body := bob.saveTeacherProfile("Sejarah", "Y4")
	expectRedirect(t, res, body, "/teachers")
	s.ageTrial(20)
	s.giveTeacherPro("ada@example.com", "month")
	s.giveTeacherPro("bob@example.com", "month")

	_, body = s.get("/admin/dskps/teacher-uploads")
	expectAll(t, body, "No Teacher has uploaded a DSKP for themselves.")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, `href="/admin/dskps/teacher-uploads"`)

	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{Err: aiwriter.ErrUnavailable})
	res, body = ada.uploadOwnDSKP("Fizik", "F4", "ms", pdfFile) // fails to read: nothing to add to the library
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.application.WaitForDSKPReads()
	for _, c := range []struct {
		teacher       *site
		subject, year string
	}{{ada, "Sejarah", "Y4"}, {bob, "sejarah", "Y4"}, {ada, "Sejarah", "Y4"}, {ada, "Geografi", "Y5"}, {bob, "Seni", "Y6"}} {
		res, body = c.teacher.uploadOwnDSKP(c.subject, c.year, "ms", pdfFile)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		s.application.WaitForDSKPReads()
		if c.subject == "Seni" { // deleted again
			bob.post(res.Header.Get("Location")+"/delete", nil)
		}
	}

	_, body = s.get("/admin/dskps/teacher-uploads")
	expectAll(t, body, "DSKPs uploaded by Teachers", "Sejarah", "Year 4", "<td>2</td>", "Geografi", "Year 5", "<td>1</td>")
	expectInOrder(t, body, "Sejarah", "Year 4", "<td>2</td>", "Geografi", "Year 5", "<td>1</td>")
	expectNone(t, body, "Fizik", "Seni", "ada@example.com", "Ada", "Bob", "/publish")
	if n := strings.Count(body, "Year 4"); n != 1 {
		t.Errorf("Sejarah of Year 4 is listed %d times, want once with both Teachers counted", n)
	}
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "No DSKP has been uploaded yet.") // none of them is in the library

	res, body = ada.get("/admin/dskps/teacher-uploads")
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = s.getAsking("/admin/dskps/teacher-uploads", "ms")
	expectLanguage(t, body, "ms", "DSKP dimuat naik oleh Guru")
}

func TestTheAdminMayUploadAtThePortalWithoutAProfileAndItIsNotInTheLibraryOrTheTeachersList(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	res, body := s.uploadOwnDSKP("Sejarah", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	p := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	_, body = s.get(p)
	expectAll(t, body, "Unchecked", "SK 1.1")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "No DSKP has been uploaded yet.")
	_, body = s.get("/admin/dskps/teacher-uploads")
	expectAll(t, body, "No Teacher has uploaded a DSKP for themselves.")
}

func TestATeacherWhoseProHasEndedCannotUpload(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newProDSKPTeacher(s)
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET starts_at = starts_at - interval '60 days', ends_at = ends_at - interval '60 days'`); err != nil {
		t.Fatal(err)
	}
	res, body := teacher.uploadOwnDSKP("Sains", "Y4", "ms", pdfFile)
	expectRedirect(t, res, body, "/teachers")
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("a lapsed Teacher's upload was read: %d requests", len(got))
	}
}

func TestUploadingAtThePortalIsRefusedWhileDSKPReadingIsOff(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newProDSKPTeacher(s)
	s.ai.DSKPOff()
	_, body := teacher.get("/teachers/dskps")
	expectAll(t, body, "DSKP reading is off")
	expectNone(t, body, `name="file"`)
	res, body := teacher.uploadOwnDSKP("Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusServiceUnavailable, "DSKP reading is off")
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("an upload while reading is off was read: %d requests", len(got))
	}
}
