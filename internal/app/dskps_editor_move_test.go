package app_test

import (
	"net/url"
	"testing"
)

func TestTheAdminReordersSPsSKsTajukAndTemasWithinWhatHoldsThem(t *testing.T) {
	t.Parallel()
	s, p, edit, sks, sps := richDSKP(t) // SKs 1.1 1.2 | 1.3 || 2.1 | 2.2
	move := func(path, dir, level string) {
		t.Helper()
		res, body := s.post(path, url.Values{"dir": {dir}, "level": {level}})
		expectRedirect(t, res, body, edit)
	}
	expectOrder := func(wants ...string) {
		t.Helper()
		_, body := s.get(p)
		expectInOrder(t, body, wants...)
	}

	// An SP moves within its SK.
	move(p+"/sps/"+sps[0]+"/move", "down", "") // SP 1.1.1 below 1.1.2
	expectOrder("SK 1.1", "SP 1.1.2", "SP 1.1.1", "SK 1.2")
	move(p+"/sps/"+sps[0]+"/move", "down", "") // already last: nothing moves
	expectOrder("SK 1.1", "SP 1.1.2", "SP 1.1.1", "SK 1.2")
	move(p+"/sps/"+sps[0]+"/move", "up", "") // SP 1.1.1 above 1.1.2 again
	move(p+"/sps/"+sps[2]+"/move", "up", "") // SP 1.2.1 is the only SP of its SK: it does not cross into SK 1.1
	expectOrder("SK 1.1", "SP 1.1.1", "SP 1.1.2", "SK 1.2", "SP 1.2.1", "SK 1.3")

	// An SK moves among the SKs of its tajuk, and no further.
	move(p+"/sks/"+sks[1]+"/move", "up", "sk") // SK 1.2 above 1.1
	expectOrder("Asas", "SK 1.2", "SK 1.1", "Kaedah", "SK 1.3")
	move(p+"/sks/"+sks[1]+"/move", "up", "")     // already first of Asas: nothing moves
	move(p+"/sks/"+sks[0]+"/move", "down", "sk") // SK 1.1 is last of Asas: it does not cross into Kaedah
	expectOrder("Tema 1", "Asas", "SK 1.2", "SK 1.1", "Kaedah", "SK 1.3", "Tema 2")

	// A tajuk moves among the tajuk of its tema as a whole.
	move(p+"/sks/"+sks[2]+"/move", "up", "topic") // Kaedah above Asas, from its only SK
	expectOrder("Tema 1", "Kaedah", "SK 1.3", "Asas", "SK 1.2", "SK 1.1", "Tema 2")
	move(p+"/sks/"+sks[0]+"/move", "up", "topic") // Asas, from its SK 1.1, above Kaedah again
	expectOrder("Tema 1", "Asas", "SK 1.2", "SK 1.1", "Kaedah", "SK 1.3", "Tema 2")
	move(p+"/sks/"+sks[2]+"/move", "down", "topic") // Kaedah is last in Tema 1: it does not cross into Tema 2

	// A tema moves as a whole.
	move(p+"/sks/"+sks[4]+"/move", "up", "group") // Tema 2, from its SK 2.2, above Tema 1
	expectOrder("Tema 2", "Sel", "SK 2.1", "Organ", "SK 2.2", "Tema 1", "Asas", "SK 1.2", "SK 1.1", "Kaedah", "SK 1.3")
	move(p+"/sks/"+sks[4]+"/move", "up", "group") // first already
	move(p+"/sks/"+sks[3]+"/move", "down", "group")
	expectOrder("Tema 1", "Asas", "SK 1.2", "SK 1.1", "Kaedah", "SK 1.3", "Tema 2", "Sel", "SK 2.1", "Organ", "SK 2.2", "SP 2.2.1")

	// The order is the editor's too.
	_, body := s.get(edit)
	if got := editorIDs(body, p, "sks"); len(got) != 5 || got[0] != sks[1] || got[1] != sks[0] || got[2] != sks[2] || got[3] != sks[3] || got[4] != sks[4] {
		t.Errorf("the editor has the SKs in the order %v, want SK 1.2, 1.1, 1.3, 2.1, 2.2 (%v)", got, []string{sks[1], sks[0], sks[2], sks[3], sks[4]})
	}

	for _, c := range []struct{ path, dir, level string }{
		{p + "/sks/" + sks[0] + "/move", "sideways", ""},
		{p + "/sks/" + sks[0] + "/move", "up", "tajuk"},
		{p + "/sks/999/move", "up", ""},
		{p + "/sps/999/move", "up", ""},
		{p + "/sps/" + sps[0] + "/move", "", ""},
		{"/admin/dskps/999/sps/" + sps[0] + "/move", "up", ""},
	} {
		if res, _ := s.post(c.path, url.Values{"dir": {c.dir}, "level": {c.level}}); res.StatusCode != 404 {
			t.Errorf("POST %s %q %q: status %d, want 404", c.path, c.dir, c.level, res.StatusCode)
		}
	}
}
