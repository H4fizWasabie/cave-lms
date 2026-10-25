package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// caveTextOwnerSite is a CaveText site logged in as the Owner.
func caveTextOwnerSite(t *testing.T) *site {
	t.Helper()
	s := newSite(t, func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.CaveTextExtensionIDs = []string{extensionID}
	})
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	return s
}

// nextPlace makes the next first CaveText sign-in get place n, as if n-1 Learners had signed in before.
func (s *site) nextPlace(n int) {
	s.t.Helper()
	if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE cavetext_accounts ALTER COLUMN place RESTART WITH %d`, n)); err != nil {
		s.t.Fatal(err)
	}
}

// newCaveTextLearner logs a new Learner in on its own client and signs CaveText in.
func (s *site) newCaveTextLearner(name, email string) (*site, string) {
	s.t.Helper()
	l := s.second()
	l.loginAsNewLearner(name, email, "correct horse battery")
	return l, l.connect()
}

func (s *site) fillSnippets(token string, n int) {
	s.t.Helper()
	for i := range n {
		s.createSnippet(token, snippet{Folder: "Work", Name: fmt.Sprintf("S%d", i), Shortcut: fmt.Sprintf("/s%d", i), Body: "x"})
	}
}

func (s *site) saveCaps(places, early, late string) (*http.Response, string) {
	s.t.Helper()
	return s.post("/admin/cavetext/caps", url.Values{"early_places": {places}, "early_cap": {early}, "late_cap": {late}})
}

func TestTheFirstThousandLearnersMayKeepTenSnippetsAndLaterOnesFive(t *testing.T) {
	s := newCaveTextSite(t)
	s.nextPlace(1000)
	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	bea, beaToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")

	if me := ada.me(adaToken); me.Place != 1000 || me.Cap != 10 || !me.Early || me.SnippetCount != 0 {
		t.Fatalf("place 1000: me = %+v, want early with cap 10 and no Snippets", me)
	}
	if me := bea.me(beaToken); me.Place != 1001 || me.Cap != 5 || me.Early {
		t.Fatalf("place 1001: me = %+v, want late with cap 5", me)
	}
	ada.fillSnippets(adaToken, 10)
	bea.fillSnippets(beaToken, 5)
	if me := ada.me(adaToken); me.SnippetCount != 10 {
		t.Fatalf("me = %+v, want 10 Snippets", me)
	}
	res, body := ada.caveTextSend(http.MethodPost, "snippets", adaToken, snippet{Folder: "Work", Name: "One more", Shortcut: "/more", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, "10 free Snippets")
	res, body = bea.caveTextSend(http.MethodPost, "snippets", beaToken, snippet{Folder: "Work", Name: "One more", Shortcut: "/more", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, "5 free Snippets")
}

func TestAddingPastTheCapIsRefusedWhileEditingAndDeletingStillWork(t *testing.T) {
	s := newCaveTextSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 5)

	res, body := ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "Sixth", Shortcut: "/sixth", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, `{"error":"`)
	expectStatus(t, res, body, http.StatusForbidden, "You have 5 of your 5 free Snippets.")
	expectStatus(t, res, body, http.StatusForbidden, "CaveText Pro")
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("403 Content-Type = %q, want JSON", ct)
	}

	list := ada.snippets(token)
	if len(list) != 5 {
		t.Fatalf("Snippets = %s, want 5", describe(list))
	}
	res, body = ada.caveTextSend(http.MethodPut, fmt.Sprintf("snippets/%d", list[0].ID), token, snippet{Folder: "Work", Name: "S0", Shortcut: "/s0", Body: "edited at the cap"})
	expectStatus(t, res, body, http.StatusOK, "edited at the cap")
	res, body = ada.caveText(http.MethodDelete, fmt.Sprintf("snippets/%d", list[1].ID), token)
	expectStatus(t, res, body, http.StatusNoContent, "")
	ada.createSnippet(token, snippet{Folder: "Work", Name: "Sixth", Shortcut: "/sixth", Body: "x"})
	if me := ada.me(token); me.SnippetCount != 5 {
		t.Fatalf("me = %+v, want 5 Snippets", me)
	}
}

func TestRaisingTheEarlyCapLetsAnEarlyLearnerAddMoreAtOnce(t *testing.T) {
	s := caveTextOwnerSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 10)
	extra := snippet{Folder: "Work", Name: "Eleventh", Shortcut: "/eleventh", Body: "x"}
	res, body := ada.caveTextSend(http.MethodPost, "snippets", token, extra)
	expectStatus(t, res, body, http.StatusForbidden, "10 free Snippets")

	res, body = s.saveCaps("1000", "15", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.createSnippet(token, extra)
	if me := ada.me(token); me.Cap != 15 || me.SnippetCount != 11 || !me.Early {
		t.Fatalf("me = %+v, want early with 11 of 15", me)
	}

	// Fewer early places makes Ada late; her Snippets stay, but she can't add until under the late cap.
	res, body = s.saveCaps("0", "15", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if me := ada.me(token); me.Cap != 5 || me.Early || me.SnippetCount != 11 {
		t.Fatalf("me = %+v, want late with 11 of 5", me)
	}
	res, body = ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "Twelfth", Shortcut: "/twelfth", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, "You have 11 of your 5 free Snippets.")
}

func TestAnImportOverTheCapStopsAtTheCapAndSaysHowManyWereSkipped(t *testing.T) {
	s := newCaveTextSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	var set []snippet
	for i := range 7 {
		set = append(set, snippet{Folder: "Kept", Name: fmt.Sprintf("I%d", i), Shortcut: fmt.Sprintf("/i%d", i), Body: "x"})
	}
	res, body := ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": set})
	expectStatus(t, res, body, http.StatusOK, `"skipped":2`)
	var got struct {
		Snippets []snippet
		Skipped  int
		Message  string
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if want := "Imported 5 Snippets, your free cap; the other 2 were skipped. CaveText Pro gives up to 2,000 Snippets."; got.Message != want {
		t.Fatalf("message = %q, want %q", got.Message, want)
	}
	want := `[Kept/I0 /i0 "x" master=true][Kept/I1 /i1 "x" master=true][Kept/I2 /i2 "x" master=true][Kept/I3 /i3 "x" master=true][Kept/I4 /i4 "x" master=true]`
	if d := describe(got.Snippets); d != want {
		t.Fatalf("answer = %s, want %s", d, want)
	}
	if d := describe(ada.snippets(token)); d != want {
		t.Fatalf("Snippets = %s, want %s", d, want)
	}

	res, body = ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": set[:3]})
	expectStatus(t, res, body, http.StatusOK, `"skipped":0`)
	if strings.Contains(body, `"message"`) {
		t.Fatalf("an import within the cap carries a message: %s", body)
	}
}

func TestOnlyTheOwnerEditsTheCaveTextCaps(t *testing.T) {
	s := caveTextOwnerSite(t)
	for _, path := range []string{"/admin", "/admin/plans"} {
		_, body := s.get(path)
		if !strings.Contains(body, `href="/admin/cavetext/caps"`) {
			t.Errorf("the Owner's Admin sidebar on %s has no CaveText caps link", path)
		}
	}
	res, body := s.get("/admin/cavetext/caps")
	expectStatus(t, res, body, http.StatusOK, "<h1>Admin: CaveText caps</h1>")
	for _, want := range []string{`name="early_places" value="1000"`, `name="early_cap" value="10"`, `name="late_cap" value="5"`} {
		if !strings.Contains(body, want) {
			t.Errorf("caps page lacks %s", want)
		}
	}

	for _, bad := range [][3]string{{"", "10", "5"}, {"1000", "-1", "5"}, {"1000", "10", "five"}, {"1000", "10", "2.5"}, {"1000", "10", "1000001"}} {
		res, body = s.saveCaps(bad[0], bad[1], bad[2])
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "whole numbers")
	}
	res, body = s.saveCaps("2000", "15", "6")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/cavetext/caps")
	expectStatus(t, res, body, http.StatusOK, `name="early_places" value="2000"`)
	expectStatus(t, res, body, http.StatusOK, `name="early_cap" value="15"`)
	expectStatus(t, res, body, http.StatusOK, `name="late_cap" value="6"`)

	admin := s.second()
	admin.loginAsNewAdmin("Mini", "mini@example.com", "correct horse battery")
	learner := s.second()
	learner.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	visitor := s.second()
	for _, other := range []*site{admin, learner, visitor} {
		res, body = other.get("/admin/cavetext/caps")
		expectStatus(t, res, body, http.StatusNotFound, "")
		res, body = other.saveCaps("1", "1", "1")
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	_, body = admin.get("/admin")
	if strings.Contains(body, `href="/admin/cavetext/caps"`) {
		t.Errorf("another Admin's sidebar links to the CaveText caps")
	}
	res, body = s.get("/admin/cavetext/caps")
	expectStatus(t, res, body, http.StatusOK, `name="early_cap" value="15"`)
}

func TestAnImportOverTheCapChecksOnlyTheSnippetsItKeeps(t *testing.T) {
	s := newCaveTextSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	var set []snippet
	for i := range 5 {
		set = append(set, snippet{Folder: "Kept", Name: fmt.Sprintf("I%d", i), Shortcut: fmt.Sprintf("/i%d", i), Body: "x"})
	}
	clash := snippet{Folder: "Kept", Name: "Clash", Shortcut: "/i0", Body: "x"}

	res, body := ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": append(set[:5:5], clash)})
	expectStatus(t, res, body, http.StatusOK, `"skipped":1`)
	if n := len(ada.snippets(token)); n != 5 {
		t.Fatalf("Snippets = %d, want the 5 kept", n)
	}

	// A clash among the kept ones still refuses the whole import.
	res, body = ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": append([]snippet{set[0], clash}, set[1:]...)})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	if n := len(ada.snippets(token)); n != 5 {
		t.Fatalf("Snippets after a refused import = %d, want the 5 from before", n)
	}
}

func TestTheCaveTextCapsRefuseACrossOriginSave(t *testing.T) {
	s := caveTextOwnerSite(t)
	form := url.Values{"early_places": {"1"}, "early_cap": {"1"}, "late_cap": {"1"}}
	for header, value := range map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"} {
		req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/admin/cavetext/caps", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(header, value)
		res, body := s.do(req)
		expectStatus(t, res, body, http.StatusForbidden, "cross-origin request refused")
	}
	res, body := s.get("/admin/cavetext/caps")
	expectStatus(t, res, body, http.StatusOK, `name="early_cap" value="10"`)
}

func TestSavingTheCaveTextCapsSaysSaved(t *testing.T) {
	s := caveTextOwnerSite(t)
	res, body := s.get("/admin/cavetext/caps")
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(body, "Saved.") {
		t.Fatalf("the caps page says Saved. before any save")
	}
	if strings.Contains(body, "nobody loses") || !strings.Contains(body, "never deletes a Snippet") {
		t.Fatalf("the caps page doesn't say plainly that a cap change never deletes a Snippet")
	}
	res, body = s.saveCaps("1000", "12", "5")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Saved.")
	expectStatus(t, res, body, http.StatusOK, `name="early_cap" value="12"`)
}
