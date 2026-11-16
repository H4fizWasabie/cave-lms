package app_test

import (
	"net/url"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// richDSKP is a draft DSKP of two temas, each with two tajuk: Tema 1 has Asas (SK 1.1 with SP 1.1.1 and 1.1.2, SK 1.2) and
// Kaedah (SK 1.3); Tema 2 has Sel (SK 2.1) and Organ (SK 2.2, with SP 2.2.1). It returns the SK and SP ids of its editor in
// page order.
func richDSKP(t *testing.T) (s *site, p, edit string, sks, sps []string) {
	t.Helper()
	s = newDSKPAdmin(t)
	sp := func(code string) aiwriter.SP { return aiwriter.SP{Code: code, Text: "Standard " + code} }
	s.ai.ReplyDSKPs(testutil.FakeDSKPReply{SKs: []aiwriter.SK{
		{Grouping: "Tema 1", Topic: "Asas", Code: "1.1", Text: "Kandungan 1.1", SPs: []aiwriter.SP{sp("1.1.1"), sp("1.1.2")}},
		{Grouping: "Tema 1", Topic: "Asas", Code: "1.2", Text: "Kandungan 1.2", SPs: []aiwriter.SP{sp("1.2.1")}},
		{Grouping: "Tema 1", Topic: "Kaedah", Code: "1.3", Text: "Kandungan 1.3", SPs: []aiwriter.SP{sp("1.3.1")}},
		{Grouping: "Tema 2", Topic: "Sel", Code: "2.1", Text: "Kandungan 2.1", SPs: []aiwriter.SP{sp("2.1.1")}},
		{Grouping: "Tema 2", Topic: "Organ", Code: "2.2", Text: "Kandungan 2.2", SPs: []aiwriter.SP{sp("2.2.1")}},
	}})
	p = s.uploadedDSKP("Sains", "Y4")
	s.application.WaitForDSKPReads()
	edit = p + "/edit"
	_, body := s.get(edit)
	return s, p, edit, editorIDs(body, p, "sks"), editorIDs(body, p, "sps")
}

func TestTheAdminDeletesAnSPAnSKATajukOrATemaWithWhatIsUnderIt(t *testing.T) {
	t.Parallel()
	s, p, edit, sks, sps := richDSKP(t)
	if len(sks) != 5 || len(sps) != 6 {
		t.Fatalf("the editor has forms for %d SKs and %d SPs, want 5 and 6", len(sks), len(sps))
	}

	res, body := s.post(p+"/sps/"+sps[0]+"/delete", nil) // SP 1.1.1
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectAll(t, body, "SP 1.1.2", "SK 1.1")
	expectNone(t, body, "SP 1.1.1")

	res, body = s.post(p+"/sks/"+sks[3]+"/delete", nil) // SK 2.1 alone: Sel goes with its only SK
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectAll(t, body, "Tema 2", "Organ", "SK 2.2")
	expectNone(t, body, "SK 2.1", "SP 2.1.1", ">Sel<")

	res, body = s.post(p+"/sks/"+sks[1]+"/delete", url.Values{"level": {"topic"}}) // Asas, from its second SK
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectAll(t, body, "Tema 1", "Kaedah", "SK 1.3", "SP 1.3.1", "Tema 2")
	expectNone(t, body, ">Asas<", "SK 1.1", "SK 1.2", "SP 1.1.2", "SP 1.2.1")

	res, body = s.post(p+"/sks/"+sks[2]+"/delete", url.Values{"level": {"group"}}) // Tema 1, from its last SK
	expectRedirect(t, res, body, edit)
	_, body = s.get(p)
	expectAll(t, body, "Tema 2", "SK 2.2")
	expectNone(t, body, "Tema 1", "Kaedah", "SK 1.3")

	for _, c := range []struct {
		path string
		form url.Values
	}{
		{p + "/sks/" + sks[0] + "/delete", nil},                            // gone already
		{p + "/sks/" + sks[4] + "/delete", url.Values{"level": {"tajuk"}}}, // no such level
		{p + "/sps/" + sps[0] + "/delete", nil},
		{p + "/sps/abc/delete", nil},
		{"/admin/dskps/999/sks/" + sks[4] + "/delete", nil},
	} {
		if res, _ := s.post(c.path, c.form); res.StatusCode != 404 {
			t.Errorf("POST %s: status %d, want 404", c.path, res.StatusCode)
		}
	}
	_, body = s.get(p)
	expectAll(t, body, "Tema 2", "Organ", "SK 2.2", "SP 2.2.1") // nothing else was touched
}
