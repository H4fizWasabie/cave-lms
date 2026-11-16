package app_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// uploadedDSKP uploads a Malay DSKP as the Admin and returns the page it leads to, /admin/dskps/<id>.
func (s *site) uploadedDSKP(subject, year string) string {
	s.t.Helper()
	res, body := s.uploadDSKP(subject, year, "ms", "dskp.pdf", pdfFile)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return res.Header.Get("Location")
}

// publishedDSKP uploads a DSKP, waits for the read and publishes it, returning its Admin page.
func (s *site) publishedDSKP(subject, year string) string {
	s.t.Helper()
	p := s.uploadedDSKP(subject, year)
	s.application.WaitForDSKPReads()
	res, body := s.post(p+"/publish", nil)
	expectRedirect(s.t, res, body, p)
	return p
}

// newDSKPTeacher is a Teacher, Ada, teaching Sains and Matematik in Years 4 and 5, logged in on a client of her own.
func newDSKPTeacher(s *site) *site {
	teacher := s.second()
	teacher.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	res, body := teacher.saveTeacherProfile("Sains, Matematik", "Y4", "Y5")
	expectRedirect(s.t, res, body, "/teachers")
	return teacher
}

func TestOnlyTheAdminOpensTheDSKPLibrary(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/admin/dskps")
	expectRedirect(t, res, body, "/login")
	teacher := newDSKPTeacher(s)
	res, body = teacher.get("/admin/dskps")
	expectStatus(t, res, body, http.StatusForbidden, "")
	res, body = teacher.uploadDSKP("Sains", "Y4", "ms", "sains.pdf", pdfFile)
	expectStatus(t, res, body, http.StatusForbidden, "")
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("a Teacher's upload was read: %d requests", len(got))
	}
}

func TestTheAdminUploadsAPDFWithSubjectYearAndLanguageAndNothingElse(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	_, body := s.get("/admin/dskps")
	expectAll(t, body, `enctype="multipart/form-data"`, `name="subject"`, `name="year"`, `name="language"`, `name="file"`, `value="F5"`, "No DSKP has been uploaded yet.")

	for _, c := range []struct {
		name                          string
		subject, year, language, file string
		pdf                           []byte
		status                        int
		want                          string
	}{
		{"not a PDF", "Sains", "Y4", "ms", "sains.pdf", jpegFile, http.StatusUnprocessableEntity, "Upload the DSKP as a PDF file."},
		{"a PDF in name only", "Sains", "Y4", "ms", "sains.pdf", []byte("<html>%PDF-</html>"), http.StatusUnprocessableEntity, "Upload the DSKP as a PDF file."},
		{"empty file", "Sains", "Y4", "ms", "sains.pdf", nil, http.StatusUnprocessableEntity, "Upload the DSKP as a PDF file."},
		{"no subject", " ", "Y4", "ms", "sains.pdf", pdfFile, http.StatusUnprocessableEntity, "Enter the subject"},
		{"a long subject", strings.Repeat("x", 101), "Y4", "ms", "sains.pdf", pdfFile, http.StatusUnprocessableEntity, "Enter the subject"},
		{"a year nobody teaches", "Sains", "Y7", "ms", "sains.pdf", pdfFile, http.StatusUnprocessableEntity, "Pick the year."},
		{"another language", "Sains", "Y4", "fr", "sains.pdf", pdfFile, http.StatusUnprocessableEntity, "Pick the language of the DSKP."},
		{"over 30 MB", "Sains", "Y4", "ms", "sains.pdf", append([]byte("%PDF-"), make([]byte, 30<<20)...), http.StatusRequestEntityTooLarge, "up to about 30 MB, and nothing was stored"},
	} {
		res, body := s.uploadDSKP(c.subject, c.year, c.language, c.file, c.pdf)
		if res.StatusCode != c.status || !strings.Contains(body, c.want) {
			t.Errorf("%s: status %d, want %d with %q; body:\n%s", c.name, res.StatusCode, c.status, c.want, body)
		}
	}
	s.application.WaitForDSKPReads()
	if got := s.ai.DSKPRequests(); len(got) != 0 {
		t.Fatalf("a refused upload was read: %d requests", len(got))
	}
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "No DSKP has been uploaded yet.")
	// A refused upload gives the form back as it was posted.
	_, body = s.uploadDSKP("Sains Tahun 4", "Y5", "en", "sains.pdf", jpegFile)
	expectAll(t, body, `value="Sains Tahun 4"`, `value="Y5" selected`, `value="en" selected`)
}

func TestAnUploadedDSKPIsReadInTheBackgroundThenWaitsAsADraftUntilPublished(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newDSKPTeacher(s)
	release := s.ai.HoldDSKPs()

	p := s.uploadedDSKP("Sains", "Y4") // answers without waiting for the read
	_, body := s.get(p)
	expectAll(t, body, "Sains", "Year 4", "Malay", "Reading", "The AI is reading this DSKP")
	expectNone(t, body, "Publish", "Tema 1")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "Sains", "Reading")

	release()
	s.application.WaitForDSKPReads()
	_, body = s.get(p)
	expectAll(t, body, "Draft", "This DSKP is a draft: Teachers cannot see it.", `action="`+p+`/publish"`,
		"Tema 1: Kajian Saintifik", "Kemahiran Saintifik", "Penyiasatan", "SK 1.1", "Memahami kemahiran saintifik", "SP 1.1.1", "Mengenal pasti kemahiran proses sains", "SP 1.2.1", "Membuat hipotesis")
	if n := strings.Count(body, "Tema 1: Kajian Saintifik"); n != 1 {
		t.Errorf("the tema is shown %d times, want once, with both its SKs under it", n)
	}
	expectInOrder(t, body, "Tema 1: Kajian Saintifik", "Kemahiran Saintifik", "SK 1.1", "SP 1.1.1", "SP 1.1.2", "Penyiasatan", "SK 1.2", "SP 1.2.1")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "Draft", "2 SK, 3 SP")

	// A Teacher sees nothing of it, however they ask.
	id := path.Base(p)
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "No DSKP is published yet.")
	expectNone(t, body, "Sains")
	res, body := teacher.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = s.post(p+"/publish", nil)
	expectRedirect(t, res, body, p)
	_, body = s.get(p)
	expectAll(t, body, "Published", "Teachers can see this DSKP.")
	expectNone(t, body, `/publish"`)
	res, body = s.post(p+"/publish", nil) // again changes nothing
	expectRedirect(t, res, body, p)
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "Sains", `href="/teachers/dskps/`+id+`"`)
	res, body = teacher.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusOK, "Tema 1: Kajian Saintifik")
	expectAll(t, body, "Sains", "Year 4", "SK 1.1", "SP 1.1.2", "Menyatakan kemahiran manipulatif", "SP 1.2.1")
	expectInOrder(t, body, "Tema 1: Kajian Saintifik", "Kemahiran Saintifik", "SK 1.1", "SP 1.1.1", "Penyiasatan", "SK 1.2")
	expectNone(t, body, "Publish")

	// Only a draft publishes: nothing else, and nothing that is not there.
	res, body = s.post("/admin/dskps/999/publish", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestTeachersSeeTheDSKPsOfTheirSubjectsAndYearsFirst(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	s.publishedDSKP("Sejarah", "F1")
	s.publishedDSKP("sains", "Y4") // the subject matches whatever its case
	s.publishedDSKP("Matematik", "Y5")
	s.publishedDSKP("Sains", "Y6") // her subject, but not her year
	s.uploadedDSKP("Pendidikan Seni", "Y4")
	s.application.WaitForDSKPReads()
	teacher := newDSKPTeacher(s)

	_, body := teacher.get("/teachers/dskps")
	expectInOrder(t, body, "Your DSKPs", "Matematik", "sains", "Other DSKPs", "Sains</a> · Year 6", "Sejarah")
	expectNone(t, body, "Pendidikan Seni", "Teachers cannot see")
	expectAll(t, body, `href="/teachers/dskps"`) // the portal's own navigation
}

func TestTheTeachersPortalLetsInOnlyWhoseTrialRunsAndTellsTheRestWhereToGo(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	id := path.Base(s.publishedDSKP("Sains", "Y4"))

	visitor := s.second()
	res, body := visitor.get("/teachers/dskps")
	expectRedirect(t, res, body, "/login?next=%2Fteachers%2Fdskps")

	noProfile := s.second()
	noProfile.loginAsNewLearner("Bob", "bob@example.com", teachersPassword)
	res, body = noProfile.get("/teachers/dskps/" + id)
	expectRedirect(t, res, body, "/teachers/profile")

	teacher := newDSKPTeacher(s)
	res, body = teacher.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusOK, "Sains")
	s.ageTrial(14)
	res, body = teacher.get("/teachers/dskps")
	expectRedirect(t, res, body, "/teachers")
	res, body = teacher.get("/teachers/dskps/" + id)
	expectRedirect(t, res, body, "/teachers")
	res, body = teacher.get("/teachers/dskps/abc")
	expectRedirect(t, res, body, "/teachers")

	// The Admin passes without a profile.
	res, body = s.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusOK, "Sains")
	res, body = s.get("/teachers/dskps/999")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestABadlyShapedReadingIsAskedForAgainOnceThenFails(t *testing.T) {
	t.Parallel()
	noSP := []aiwriter.SK{{Grouping: "Tema 1", Code: "1.1", Text: "Memahami"}}
	s := newDSKPAdmin(t)

	// Wrong once, then right: a draft, after two requests.
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{SKs: noSP})
	p := s.uploadedDSKP("Sains", "Y4")
	s.application.WaitForDSKPReads()
	_, body := s.get(p)
	expectAll(t, body, "Draft", "SP 1.1.1")
	if got := len(s.ai.DSKPRequests()); got != 2 {
		t.Fatalf("a bad reading was asked for %d times in all, want 2", got)
	}

	// Wrong twice: failed, with nothing kept.
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{SKs: noSP}, testutil.FakeDSKPReply{})
	p = s.uploadedDSKP("Sejarah", "Y5")
	s.application.WaitForDSKPReads()
	_, body = s.get(p)
	expectAll(t, body, "Failed", "The AI could not read this DSKP", "no standards in the right shape", "Upload it again.")
	expectNone(t, body, "Tema 1", "Publish")
	if got := len(s.ai.DSKPRequests()); got != 4 {
		t.Fatalf("after a second bad reading the writer was asked %d times in all, want 4", got)
	}
	res, body := s.post(p+"/publish", nil) // a failed DSKP cannot be published
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, "Failed")
}

func TestAReadThatCannotBeDoneIsFailedWithItsReason(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		err  error
		want string
	}{
		{aiwriter.ErrUnavailable, "the AI service is unavailable."},
		{aiwriter.ErrOverLimit, "the AI key is over its spending limit."},
		{aiwriter.ErrNotConfigured, "the AI is not configured."},
	} {
		s := newDSKPAdmin(t)
		s.ai.ReplyDSKPs(testutil.FakeDSKPReply{Err: c.err})
		p := s.uploadedDSKP("Sains", "Y4")
		s.application.WaitForDSKPReads()
		_, body := s.get(p)
		expectAll(t, body, "Failed", c.want)
		if got := len(s.ai.DSKPRequests()); got != 1 {
			t.Errorf("%v: the writer was asked %d times, want 1: only a bad shape is asked again", c.err, got)
		}
	}
}

func TestARestartInTheMiddleOfAReadMarksItFailedAndItsLateAnswerChangesNothing(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	release := s.ai.HoldDSKPs()
	p := s.uploadedDSKP("Sains", "Y4")
	_, body := s.get(p)
	expectAll(t, body, "Reading")

	if err := s.application.FailInterruptedDSKPReads(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, body = s.get(p)
	expectAll(t, body, "Failed", "the site restarted while it was being read.", "Upload it again.")

	release()
	s.application.WaitForDSKPReads()
	_, body = s.get(p)
	expectAll(t, body, "Failed")
	expectNone(t, body, "Tema 1", "Publish")
}

func TestThePDFIsHandedToTheAIAndNothingOfTheFileIsStored(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("dskp secret page\n"), 2000)...)
	res, body := s.uploadDSKP("Sains", "Y4", "en", "sains.pdf", pdf)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.application.WaitForDSKPReads()

	got := s.ai.DSKPRequests()
	if len(got) != 1 || !bytes.Equal(got[0].PDF, pdf) || got[0].Language != "en" {
		t.Fatalf("the AI was asked %d times; want the PDF and its language, once", len(got))
	}
	// Not on the disk, wherever the app keeps files.
	err := fs.WalkDir(os.DirFS(s.storageDir), ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := fs.ReadFile(os.DirFS(s.storageDir), name)
			if bytes.Contains(data, []byte("dskp secret page")) {
				t.Errorf("the PDF is stored in %s", name)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestThePrivacyNoticeSaysDSKPsGoToAnAIServiceAndAreNotKept(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/privacy", "en")
	expectAll(t, body, "DSKP PDFs the Admin or a Teacher uploads go to an AI service through OpenRouter", "are not kept", "no Teacher&#39;s name, email or other details are sent")
	_, body = s.getAsking("/privacy", "ms")
	expectAll(t, body, "PDF DSKP yang dimuat naik oleh Pentadbir atau Guru dihantar kepada perkhidmatan AI melalui OpenRouter", "tidak disimpan")
}

func TestTheDSKPLibraryIsInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	p := s.publishedDSKP("Sains", "Y4")
	teacher := newDSKPTeacher(s)
	for _, c := range []struct {
		site  *site
		path  string
		wants []string
	}{
		{s, "/admin/dskps", []string{"Perpustakaan DSKP", "Muat naik DSKP", "Diterbitkan", "Tahun 4"}},
		{s, p, []string{"Diterbitkan", "Guru boleh melihat DSKP ini."}},
		{teacher, "/teachers/dskps", []string{"DSKP anda", "Tahun 4"}},
		{teacher, "/teachers/dskps/" + path.Base(p), []string{"Semua DSKP", "Tahun 4"}},
	} {
		_, body := c.site.getAsking(c.path, "ms")
		expectLanguage(t, body, "ms", c.wants[0])
		expectAll(t, body, c.wants...)
	}
	_, body := teacher.getAsking("/teachers/dskps", "ms")
	expectNone(t, body, "Pelajar")
}

func TestAReadThatFailsBecauseTheServiceFailsIsLoggedWithTheServicesReason(t *testing.T) {
	t.Parallel()
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" })
	s.loginAsNewAdmin("Abah", "admin@example.com", adminPassword)
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{Err: errors.Join(aiwriter.ErrUnavailable, errors.New("openrouter: 502"))})
	s.uploadedDSKP("Sains", "Y4")
	s.application.WaitForDSKPReads()
	res, body := s.get("/admin/log")
	expectStatus(t, res, body, http.StatusOK, "read DSKP 1")
	expectAll(t, body, "openrouter: 502")
}
