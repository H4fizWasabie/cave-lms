package app_test

import (
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"
)

// editorIDs lists the ids of kind ("sks" or "sps") that the editor page of the DSKP at p has forms for, in page order, each
// once.
func editorIDs(body, p, kind string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`action="`+regexp.QuoteMeta(p)+`/`+kind+`/(\d+)[/"]`).FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}

// editedDSKP is a draft DSKP of the fake writer's two SKs, with its page and its editor's address.
func editedDSKP(t *testing.T) (s *site, p, edit string) {
	t.Helper()
	s = newDSKPAdmin(t)
	p = s.uploadedDSKP("Sains", "Y4")
	s.application.WaitForDSKPReads()
	return s, p, p + "/edit"
}

func TestTheAdminChangesTheCodeAndTextOfAnSKAndAnSP(t *testing.T) {
	t.Parallel()
	s, p, edit := editedDSKP(t)
	_, body := s.get(p)
	expectAll(t, body, `href="`+edit+`"`)
	_, body = s.get(edit)
	expectAll(t, body, "Sains", `value="1.1"`, "Memahami kemahiran saintifik", `value="1.1.2"`, "Menyatakan kemahiran manipulatif")
	sks, sps := editorIDs(body, p, "sks"), editorIDs(body, p, "sps")
	if len(sks) != 2 || len(sps) != 3 {
		t.Fatalf("the editor has forms for %d SKs and %d SPs, want 2 and 3", len(sks), len(sps))
	}

	res, body := s.post(p+"/sks/"+sks[0], url.Values{"code": {"1.1a"}, "text": {"Memahami kemahiran sains"}})
	expectRedirect(t, res, body, edit)
	res, body = s.post(p+"/sps/"+sps[1], url.Values{"code": {"1.1.2a"}, "text": {" Menyatakan kemahiran manipulatif sains "}})
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectAll(t, body, "SK 1.1a", "Memahami kemahiran sains", "SP 1.1.2a", "Menyatakan kemahiran manipulatif sains", "SP 1.1.1", "SK 1.2")
	expectNone(t, body, "SK 1.1</strong>", "SP 1.1.2</strong>")
}

func TestAnEditToTheStandardsNeedsACodeAndATextAndAnExistingStandardOfThatDSKP(t *testing.T) {
	t.Parallel()
	s, p, edit := editedDSKP(t)
	_, body := s.get(edit)
	sks, sps := editorIDs(body, p, "sks"), editorIDs(body, p, "sps")
	other := s.uploadedDSKP("Sejarah", "Y5")
	s.application.WaitForDSKPReads()

	for _, c := range []struct {
		name, path string
		form       url.Values
		status     int
	}{
		{"an SK without a code", p + "/sks/" + sks[0], url.Values{"code": {" "}, "text": {"Memahami"}}, http.StatusUnprocessableEntity},
		{"an SK without a text", p + "/sks/" + sks[0], url.Values{"code": {"1.1"}}, http.StatusUnprocessableEntity},
		{"an SP with a long code", p + "/sps/" + sps[0], url.Values{"code": {strings.Repeat("1", 41)}, "text": {"Mengenal"}}, http.StatusUnprocessableEntity},
		{"an SP with a long text", p + "/sps/" + sps[0], url.Values{"code": {"1.1.1"}, "text": {strings.Repeat("x", 2001)}}, http.StatusUnprocessableEntity},
		{"an SK that is not there", p + "/sks/999", url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"an SK that is no number", p + "/sks/abc", url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"an SP that is not there", p + "/sps/999", url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"an SK of another DSKP", other + "/sks/" + sks[0], url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"an SP of another DSKP", other + "/sps/" + sps[0], url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"a DSKP that is not there", "/admin/dskps/999/sks/" + sks[0], url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
	} {
		res, body := s.post(c.path, c.form)
		if res.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, res.StatusCode, c.status)
		}
		if c.status == http.StatusUnprocessableEntity && !strings.Contains(body, "Each standard needs a code") {
			t.Errorf("%s: no word on what was wrong; body:\n%s", c.name, body)
		}
	}
	_, body = s.get(p)
	expectAll(t, body, "SK 1.1", "Memahami kemahiran saintifik", "SP 1.1.1", "Mengenal pasti kemahiran proses sains")
	_, body = s.get(other)
	expectAll(t, body, "SK 1.1", "Memahami kemahiran saintifik")

	// Only a DSKP that was read can be edited: not one being read, nor one that failed.
	release := s.ai.HoldDSKPs()
	reading := s.uploadedDSKP("Geografi", "Y5")
	res, body := s.get(reading + "/edit")
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.post(reading+"/sks/"+sks[0], url.Values{"code": {"1"}, "text": {"x"}})
	expectStatus(t, res, body, http.StatusNotFound, "")
	release()
	s.application.WaitForDSKPReads()
}

func TestOnlyTheAdminEditsTheStandardsAndATeacherSeesTheEditOfAPublishedDSKPAtOnce(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	p := s.publishedDSKP("Sains", "Y4")
	teacher := newDSKPTeacher(s)
	_, body := s.get(p + "/edit")
	sks := editorIDs(body, p, "sks")

	res, body := teacher.get(p + "/edit")
	expectStatus(t, res, body, http.StatusForbidden, "")
	res, body = teacher.post(p+"/sks/"+sks[0], url.Values{"code": {"9.9"}, "text": {"Dirosakkan"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = teacher.get("/teachers/dskps/" + path.Base(p))
	expectNone(t, body, "Dirosakkan")

	res, body = s.post(p+"/sks/"+sks[0], url.Values{"code": {"1.1"}, "text": {"Memahami kemahiran saintifik dengan betul"}})
	expectRedirect(t, res, body, p+"/edit")
	_, body = teacher.get("/teachers/dskps/" + path.Base(p))
	expectAll(t, body, "Memahami kemahiran saintifik dengan betul")
}

func TestTheAdminUnpublishesADSKPAndTeachersStopSeeingIt(t *testing.T) {
	t.Parallel()
	s := newDSKPAdmin(t)
	teacher := newDSKPTeacher(s)
	p := s.publishedDSKP("Sains", "Y4")
	id := path.Base(p)
	_, body := s.get(p)
	expectAll(t, body, "Published", `action="`+p+`/unpublish"`)
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "Sains")

	res, body := teacher.post(p+"/unpublish", nil)
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "Sains")

	res, body = s.post(p+"/unpublish", nil)
	expectRedirect(t, res, body, p)
	_, body = s.get(p)
	expectAll(t, body, "Draft", "This DSKP is a draft: Teachers cannot see it.", `action="`+p+`/publish"`, "SK 1.1")
	expectNone(t, body, "/unpublish")
	_, body = teacher.get("/teachers/dskps")
	expectAll(t, body, "No DSKP is published yet.")
	expectNone(t, body, "Sains")
	res, body = teacher.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = s.post(p+"/unpublish", nil) // again changes nothing
	expectRedirect(t, res, body, p)
	res, body = s.post(p+"/publish", nil) // and it publishes again, standards and all
	expectRedirect(t, res, body, p)
	res, body = teacher.get("/teachers/dskps/" + id)
	expectStatus(t, res, body, http.StatusOK, "SK 1.1")

	// A DSKP being read has nothing to unpublish, and neither has one that is not there.
	release := s.ai.HoldDSKPs()
	reading := s.uploadedDSKP("Sejarah", "Y5")
	res, body = s.post(reading+"/unpublish", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
	release()
	s.application.WaitForDSKPReads()
	res, body = s.post("/admin/dskps/999/unpublish", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
}
