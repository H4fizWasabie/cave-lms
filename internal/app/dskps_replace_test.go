package app_test

import (
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// replaceDSKP uploads a new edition of the DSKP at p, as the Admin does from its page.
func (s *site) replaceDSKP(p, subject, year, language string, pdf []byte) (*http.Response, string) {
	s.t.Helper()
	return s.postFile("/admin/dskps?replaces="+path.Base(p), "edition.pdf", pdf, url.Values{"subject": {subject}, "year": {year}, "language": {language}})
}

// newEdition is a reading of one SK with one SP, 9.9, that the fake writer gives next.
func newEdition(s *site) {
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{SKs: []aiwriter.SK{
		{Grouping: "Tema Baharu", Topic: "Tajuk Baharu", Code: "9.9", Text: "Kandungan edisi baharu", SPs: []aiwriter.SP{{Code: "9.9.1", Text: "Standard edisi baharu"}}},
	}})
}

func TestTheAdminReplacesAPublishedDSKPWithANewEditionThatStaysADraftWhileTeachersKeepTheOldOne(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newDSKPTeacher(s)
	old := s.publishedDSKP("Sains", "Y4")
	_, body := s.get(old)
	expectAll(t, body, `action="/admin/dskps?replaces=`+path.Base(old)+`"`, `enctype="multipart/form-data"`, `value="Sains"`, `value="Y4" selected`, `value="ms" selected`, `name="file"`, "Replace with a new edition")

	newEdition(s)
	release := s.ai.HoldDSKPs()
	res, body := s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	fresh := res.Header.Get("Location")
	if fresh == "" || fresh == old {
		t.Fatalf("the new edition went to %q, want a page of its own", fresh)
	}
	_, body = s.get(fresh)
	expectAll(t, body, "Reading", "The AI is reading this DSKP")
	_, body = s.get(old)
	expectAll(t, body, "Published", `href="`+fresh+`"`, "A new edition is being prepared")
	expectNone(t, body, `name="file"`) // one new edition at a time

	release()
	s.application.WaitForDSKPReads()
	_, body = s.get(fresh)
	expectAll(t, body, "Draft", "This is a new edition of", `href="`+old+`"`, "Teachers keep seeing", "Kandungan edisi baharu", `action="`+fresh+`/publish"`)
	expectNone(t, body, "Kandungan 1.1")

	// Teachers see the published one, as it was, until the new edition is published.
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "Sains", `href="/teachers/dskps/`+path.Base(old)+`"`)
	if n := strings.Count(body, "Sains"); n != 1 {
		t.Errorf("the Teacher is offered Sains %d times, want once", n)
	}
	res, body = teacher.get("/teachers/dskps/" + path.Base(fresh))
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = teacher.get("/teachers/dskps/" + path.Base(old))
	expectStatus(t, res, body, http.StatusOK, "Memahami kemahiran saintifik")
	expectNone(t, body, "Kandungan edisi baharu")
	_, body = s.get(old)
	expectAll(t, body, "A new edition is waiting", "Memahami kemahiran saintifik")

	// Publishing it hands Teachers the new edition and takes the old one away.
	res, body = s.post(fresh+"/publish", nil)
	expectRedirect(t, res, body, fresh)
	_, body = s.get(fresh)
	expectAll(t, body, "Published", "Kandungan edisi baharu")
	expectNone(t, body, "This is a new edition of")
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, `href="/teachers/dskps/`+path.Base(fresh)+`"`)
	expectNone(t, body, `href="/teachers/dskps/`+path.Base(old)+`"`)
	res, body = teacher.get("/teachers/dskps/" + path.Base(fresh))
	expectStatus(t, res, body, http.StatusOK, "Kandungan edisi baharu")
	res, body = teacher.get("/teachers/dskps/" + path.Base(old))
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get(old)
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get("/admin/dskps")
	expectAll(t, body, `href="/admin/dskps/`+path.Base(fresh)+`"`)
	expectNone(t, body, `href="/admin/dskps/`+path.Base(old)+`"`)
}

func TestANewEditionOfAnUnpublishedDSKPReplacesItToo(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	old := s.publishedDSKP("Sains", "Y4")
	res, body := s.post(old+"/unpublish", nil)
	expectRedirect(t, res, body, old)
	newEdition(s)
	res, body = s.replaceDSKP(old, "Sains Tahun 4", "Y4", "en", pdfFile) // a new edition may fix the subject and language too
	expectStatus(t, res, body, http.StatusSeeOther, "")
	fresh := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	res, body = s.post(fresh+"/publish", nil)
	expectRedirect(t, res, body, fresh)
	_, body = s.get(fresh)
	expectAll(t, body, "Sains Tahun 4", "English", "Published")
	res, body = s.get(old)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestOnlyOneNewEditionIsPreparedAtATimeAndADiscardedOrFailedOneIsLetGo(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	old := s.publishedDSKP("Sains", "Y4")
	before := len(s.ai.DSKPRequests())

	newEdition(s)
	res, body := s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	fresh := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	res, body = s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusConflict, "A new edition of this DSKP is already being prepared")
	s.application.WaitForDSKPReads()
	if got := len(s.ai.DSKPRequests()) - before; got != 1 {
		t.Fatalf("the writer read %d editions, want 1: a refused one is not read", got)
	}

	// Discarding the new edition leaves the old one as it was, and a new one can be uploaded.
	res, body = s.post(old+"/discard", nil) // the old one is no edition
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.post(fresh+"/discard", nil)
	expectRedirect(t, res, body, old)
	res, body = s.get(fresh)
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get(old)
	expectAll(t, body, "Published", `name="file"`)
	expectNone(t, body, "A new edition is being prepared", "A new edition is waiting")

	// A new edition that failed to read does not stand in the way of the next.
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{Err: aiwriter.ErrUnavailable})
	res, body = s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	failed := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	_, body = s.get(failed)
	expectAll(t, body, "Failed", "the AI service is unavailable.", `action="`+failed+`/discard"`)
	_, body = s.get(old)
	expectAll(t, body, `name="file"`, `href="`+failed+`"`)
	newEdition(s)
	res, body = s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	next := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	res, body = s.get(failed)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get(next)
	expectStatus(t, res, body, http.StatusOK, "Draft")
}

func TestANewEditionNeedsAPublishedOrDraftDSKPOfTheLibraryAndAGoodUpload(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	old := s.publishedDSKP("Sains", "Y4")
	teacher := newDSKPTeacher(s)
	before := len(s.ai.DSKPRequests())

	res, body := teacher.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusForbidden, "")
	for _, c := range []struct {
		name        string
		subject, yr string
		language    string
		pdf         []byte
		status      int
		want        string
	}{
		{"not a PDF", "Sains", "Y4", "ms", jpegFile, http.StatusUnprocessableEntity, "Upload the DSKP as a PDF file."},
		{"no subject", " ", "Y4", "ms", pdfFile, http.StatusUnprocessableEntity, "Enter the subject"},
		{"another language", "Sains", "Y4", "fr", pdfFile, http.StatusUnprocessableEntity, "Pick the language of the DSKP."},
	} {
		res, body := s.replaceDSKP(old, c.subject, c.yr, c.language, c.pdf)
		// The Admin stays on the DSKP's page, with its editions and the reason.
		if res.StatusCode != c.status || !strings.Contains(body, c.want) || !strings.Contains(body, `name="file"`) || !strings.Contains(body, "Memahami kemahiran saintifik") {
			t.Errorf("%s: status %d, want %d with %q on the DSKP's page; body:\n%s", c.name, res.StatusCode, c.status, c.want, body)
		}
	}
	release := s.ai.HoldDSKPs()
	reading := s.uploadedDSKP("Sejarah", "Y5")
	for _, target := range []string{reading, "/admin/dskps/999", "/admin/dskps/abc"} {
		res, body := s.replaceDSKP(target, "Sejarah", "Y5", "ms", pdfFile) // one still being read, one not there
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	release()
	s.application.WaitForDSKPReads()
	newEdition(s)
	res, body = s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	fresh := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	res, body = s.replaceDSKP(fresh, "Sains", "Y4", "ms", pdfFile) // an edition is not itself replaced
	expectStatus(t, res, body, http.StatusNotFound, "")
	if got := len(s.ai.DSKPRequests()) - before; got != 2 { // Sejarah and the one edition
		t.Fatalf("the writer read %d DSKPs, want 2", got)
	}

	// With reading off there is nothing to replace with.
	s.ai.DSKPOff()
	_, body = s.get(old)
	expectNone(t, body, `name="file"`)
	res, body = s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusServiceUnavailable, "DSKP reading is off")
}

func TestReplacingADSKPIsInBothLanguages(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	old := s.publishedDSKP("Sains", "Y4")
	_, body := s.getAsking(old, "ms")
	expectLanguage(t, body, "ms", "Nyahterbit")
	expectAll(t, body, "Gantikan dengan edisi baharu", "Sunting standard")
	newEdition(s)
	res, body := s.replaceDSKP(old, "Sains", "Y4", "ms", pdfFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	fresh := res.Header.Get("Location")
	s.application.WaitForDSKPReads()
	_, body = s.getAsking(old, "ms")
	expectAll(t, body, "Edisi baharu menunggu")
	_, body = s.getAsking(fresh, "ms")
	expectAll(t, body, "Ini edisi baharu", "Guru terus melihat", "Buang edisi ini")
	_, body = s.getAsking(fresh+"/edit", "ms")
	expectAll(t, body, "Tambah SK", "Namakan semula", "Naik", "Turun", "Padam", "Tema atau bidang")
	expectNone(t, body, "Move up", "Delete")
}
