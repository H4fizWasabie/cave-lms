package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTheAdminRenamesATemaOrATajukForAllItsSKs(t *testing.T) {
	t.Parallel()
	s, p, edit, sks, _ := richDSKP(t)
	_, body := s.get(edit)
	expectAll(t, body, `name="level" value="group"`, `name="level" value="topic"`, `value="Tema 1"`, `value="Asas"`)

	res, body := s.post(p+"/sks/"+sks[2]+"/rename", url.Values{"level": {"group"}, "name": {" Tema Satu "}}) // from its last SK
	expectRedirect(t, res, body, edit)
	res, body = s.post(p+"/sks/"+sks[0]+"/rename", url.Values{"level": {"topic"}, "name": {"Dasar"}}) // Asas, from its first SK
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectInOrder(t, body, ">Tema Satu<", ">Dasar<", "SK 1.1", "SK 1.2", ">Kaedah<", "SK 1.3", ">Tema 2<", ">Sel<")
	expectNone(t, body, ">Tema 1<", ">Asas<")
	if n := strings.Count(body, ">Tema Satu<"); n != 1 {
		t.Errorf("the tema is a heading %d times, want once, over all its SKs", n)
	}

	for _, c := range []struct {
		name, path string
		form       url.Values
		status     int
	}{
		{"a long name", p + "/sks/" + sks[0] + "/rename", url.Values{"level": {"group"}, "name": {strings.Repeat("x", 201)}}, http.StatusUnprocessableEntity},
		{"an SK has no name", p + "/sks/" + sks[0] + "/rename", url.Values{"level": {"sk"}, "name": {"x"}}, http.StatusNotFound},
		{"no level", p + "/sks/" + sks[0] + "/rename", url.Values{"name": {"x"}}, http.StatusNotFound},
		{"an SK that is not there", p + "/sks/999/rename", url.Values{"level": {"group"}, "name": {"x"}}, http.StatusNotFound},
	} {
		if res, _ := s.post(c.path, c.form); res.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, res.StatusCode, c.status)
		}
	}
	_, body = s.get(p)
	expectAll(t, body, ">Tema Satu<", ">Dasar<")
}
