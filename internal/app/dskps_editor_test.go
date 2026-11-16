package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTheAdminAddsAnSPToAnSKAndAnSKToAnExistingOrNewGroupingAndTopic(t *testing.T) {
	t.Parallel()
	s, p, edit := editedDSKP(t)
	_, body := s.get(edit)
	sks := editorIDs(body, p, "sks")

	res, body := s.post(p+"/sks/"+sks[1]+"/sps", url.Values{"code": {"1.2.2"}, "text": {"Menguji hipotesis"}})
	expectRedirect(t, res, body, edit)
	// An SK joins the end of its grouping and tajuk when the DSKP has them, the end of its grouping under a new tajuk, and
	// the end of the DSKP under a new grouping.
	res, body = s.post(p+"/sks", url.Values{"grouping": {"Tema 1: Kajian Saintifik"}, "topic": {"Kemahiran Saintifik"}, "code": {"1.3"}, "text": {"Mengukur"}})
	expectRedirect(t, res, body, edit)
	res, body = s.post(p+"/sks", url.Values{"grouping": {"Tema 2: Hidupan"}, "topic": {"Sel"}, "code": {"2.1"}, "text": {"Mengenal sel"}})
	expectRedirect(t, res, body, edit)
	res, body = s.post(p+"/sks", url.Values{"grouping": {"Tema 1: Kajian Saintifik"}, "topic": {"Pelaporan"}, "code": {"1.4"}, "text": {"Melapor"}})
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectInOrder(t, body, "Tema 1: Kajian Saintifik", "Kemahiran Saintifik", "SK 1.1", "SP 1.1.2", "SK 1.3", "Mengukur", "Penyiasatan", "SK 1.2", "SP 1.2.1", "SP 1.2.2", "Menguji hipotesis",
		"Pelaporan", "SK 1.4", "Tema 2: Hidupan", "Sel", "SK 2.1", "Mengenal sel")
	for _, h := range []string{"Tema 1: Kajian Saintifik", "Kemahiran Saintifik", "Penyiasatan"} {
		if n := strings.Count(body, ">"+h+"<"); n != 1 {
			t.Errorf("%q is a heading %d times, want once", h, n)
		}
	}
	_, body = s.get(edit)
	expectAll(t, body, `action="`+p+`/sks"`)
	if got := len(editorIDs(body, p, "sks")); got != 5 {
		t.Errorf("the editor has %d SKs, want 5", got)
	}

	for _, c := range []struct {
		name, path string
		form       url.Values
		status     int
	}{
		{"an SK without a text", p + "/sks", url.Values{"grouping": {"Tema 3"}, "code": {"3.1"}}, http.StatusUnprocessableEntity},
		{"an SK under a long grouping", p + "/sks", url.Values{"grouping": {strings.Repeat("x", 201)}, "code": {"3.1"}, "text": {"x"}}, http.StatusUnprocessableEntity},
		{"an SK under a long tajuk", p + "/sks", url.Values{"topic": {strings.Repeat("x", 201)}, "code": {"3.1"}, "text": {"x"}}, http.StatusUnprocessableEntity},
		{"an SP without a code", p + "/sks/" + sks[0] + "/sps", url.Values{"text": {"x"}}, http.StatusUnprocessableEntity},
		{"an SP to an SK that is not there", p + "/sks/999/sps", url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
		{"an SP to an SK of a DSKP that is not there", "/admin/dskps/999/sks/" + sks[0] + "/sps", url.Values{"code": {"1"}, "text": {"x"}}, http.StatusNotFound},
	} {
		if res, _ := s.post(c.path, c.form); res.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, res.StatusCode, c.status)
		}
	}
	_, body = s.get(edit)
	if got := len(editorIDs(body, p, "sks")); got != 5 {
		t.Errorf("after the refused ones the editor has %d SKs, want 5", got)
	}
}
