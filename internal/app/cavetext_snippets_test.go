package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type snippet struct {
	ID               int64  `json:"id,omitempty"`
	Folder           string `json:"folder"`
	Name             string `json:"name"`
	Shortcut         string `json:"shortcut"`
	Body             string `json:"body"`
	WithMasterPrompt *bool  `json:"withMasterPrompt,omitempty"`
}

// caveTextSend calls the CaveText API with a JSON body, as the extension does when it writes.
func (s *site) caveTextSend(method, path, token string, body any) (*http.Response, string) {
	s.t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequest(method, s.server.URL+"/cavetext/api/"+path, bytes.NewReader(b))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "chrome-extension://"+extensionID)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	return s.do(req)
}

func (s *site) createSnippet(token string, sn snippet) snippet {
	s.t.Helper()
	res, body := s.caveTextSend(http.MethodPost, "snippets", token, sn)
	expectStatus(s.t, res, body, http.StatusCreated, "")
	var got snippet
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		s.t.Fatalf("create: %v; body: %s", err, body)
	}
	return got
}

func (s *site) snippets(token string) []snippet {
	s.t.Helper()
	res, body := s.caveText(http.MethodGet, "snippets", token)
	expectStatus(s.t, res, body, http.StatusOK, "")
	var got struct{ Snippets []snippet }
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		s.t.Fatalf("list: %v; body: %s", err, body)
	}
	return got.Snippets
}

func describe(list []snippet) string {
	out := ""
	for _, sn := range list {
		flag := "nil"
		if sn.WithMasterPrompt != nil {
			flag = fmt.Sprint(*sn.WithMasterPrompt)
		}
		out += fmt.Sprintf("[%s/%s %s %q master=%s]", sn.Folder, sn.Name, sn.Shortcut, sn.Body, flag)
	}
	return out
}

func TestASnippetSavedInOneBrowserAppearsInAnother(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	first := s.connect()
	off := false
	s.createSnippet(first, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "Review {formtext: name=x}"})
	s.createSnippet(first, snippet{Folder: "Study", Name: "Quiz", Shortcut: "/quiz", Body: "Quiz me", WithMasterPrompt: &off})

	other := s.second()
	other.login("ada@example.com", "correct horse battery")
	got := describe(other.snippets(other.connect()))
	want := `[Work/Review /review "Review {formtext: name=x}" master=true][Study/Quiz /quiz "Quiz me" master=false]`
	if got != want {
		t.Fatalf("Snippets in another browser = %s, want %s", got, want)
	}
}

func TestTheLastWriteWinsBetweenTwoBrowsers(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	first, second := s.connect(), s.connect()
	sn := s.createSnippet(first, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "v1"})
	path := fmt.Sprintf("snippets/%d", sn.ID)

	off := false
	res, body := s.caveTextSend(http.MethodPut, path, first, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "from the first"})
	expectStatus(t, res, body, http.StatusOK, `"from the first"`)
	res, body = s.caveTextSend(http.MethodPut, path, second, snippet{Folder: "Home", Name: "Check", Shortcut: "/check", Body: "from the second", WithMasterPrompt: &off})
	expectStatus(t, res, body, http.StatusOK, "")
	if got, want := describe(s.snippets(first)), `[Home/Check /check "from the second" master=false]`; got != want {
		t.Fatalf("Snippets = %s, want %s", got, want)
	}

	res, body = s.caveText(http.MethodDelete, path, second)
	expectStatus(t, res, body, http.StatusNoContent, "")
	if got := s.snippets(first); len(got) != 0 {
		t.Fatalf("Snippets after delete = %s, want none", describe(got))
	}
	// The first browser still holds the deleted Snippet in its copy; its write is refused, not brought back.
	res, body = s.caveTextSend(http.MethodPut, path, first, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "late"})
	expectStatus(t, res, body, http.StatusNotFound, `"error"`)
	res, body = s.caveText(http.MethodDelete, path, first)
	expectStatus(t, res, body, http.StatusNotFound, `"error"`)
}

func TestAnotherLearnersSnippetsAreNeverReadableOrWritable(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada := s.connect()
	sn := s.createSnippet(ada, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "Ada's"})
	path := fmt.Sprintf("snippets/%d", sn.ID)

	bea := s.second()
	bea.loginAsNewLearner("Bea Lim", "bea@example.com", "correct horse battery")
	beaToken := bea.connect()
	if got := bea.snippets(beaToken); len(got) != 0 {
		t.Fatalf("Bea sees %s", describe(got))
	}
	res, body := bea.caveTextSend(http.MethodPut, path, beaToken, snippet{Folder: "Work", Name: "Mine", Shortcut: "/mine", Body: "Bea's"})
	expectStatus(t, res, body, http.StatusNotFound, `"error"`)
	res, body = bea.caveText(http.MethodDelete, path, beaToken)
	expectStatus(t, res, body, http.StatusNotFound, `"error"`)
	// Bea may use the same shortcut as Ada: clashes are per Learner.
	bea.createSnippet(beaToken, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "Bea's"})

	if got, want := describe(s.snippets(ada)), `[Work/Review /review "Ada's" master=true]`; got != want {
		t.Fatalf("Ada's Snippets = %s, want %s", got, want)
	}
}

func TestDeletingALearnerDeletesTheirSnippets(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	s.createSnippet(s.connect(), snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})
	if _, err := s.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := s.db.QueryRow(`SELECT count(*) FROM cavetext_snippets`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d Snippets left after deleting the Learner, want 0", left)
	}
}

func TestShortcutClashesAreRefusedWithAMessage(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	review := s.createSnippet(token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})
	tldr := s.createSnippet(token, snippet{Folder: "Work", Name: "TLDR", Shortcut: "/tldr", Body: "x"})

	for _, c := range []struct{ shortcut, message string }{
		{"/review", `"Review\" already uses /review."`},
		{"/REVIEW", `"Review\" already uses /review."`},
		{"/re", `/re would always fire before /review (\"Review\") finished typing.`},
		{"/reviews", `/review (\"Review\") would always fire before /reviews finished typing.`},
		{"/my review", `Shortcuts cannot contain spaces.`},
		{"/tab\there", `Shortcuts cannot contain spaces.`},
		{"", `A shortcut is needed.`},
	} {
		res, body := s.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "New", Shortcut: c.shortcut, Body: "y"})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, `{"error":"`)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
		// Moving another Snippet onto the shortcut is refused the same way.
		res, body = s.caveTextSend(http.MethodPut, fmt.Sprintf("snippets/%d", tldr.ID), token, snippet{Folder: "Work", Name: "TLDR", Shortcut: c.shortcut, Body: "x"})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
	}
	res, body := s.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: " ", Shortcut: "/new", Body: "y"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "A name is needed.")

	// A Snippet keeps its own shortcut when edited.
	res, body = s.caveTextSend(http.MethodPut, fmt.Sprintf("snippets/%d", review.ID), token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "edited"})
	expectStatus(t, res, body, http.StatusOK, "edited")
	if got, want := describe(s.snippets(token)), `[Work/Review /review "edited" master=true][Work/TLDR /tldr "x" master=true]`; got != want {
		t.Fatalf("Snippets = %s, want %s", got, want)
	}
}

func TestAnIdBelowOneIsNoSnippet(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	for _, path := range []string{"snippets/0", "snippets/-1"} {
		res, body := s.caveTextSend(http.MethodPut, path, token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "x"})
		expectStatus(t, res, body, http.StatusNotFound, `"error"`)
	}
	if got := s.snippets(token); len(got) != 0 {
		t.Fatalf("Snippets = %s, want none", describe(got))
	}
}

func TestAnImportReplacesEverySnippetOrNone(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	s.createSnippet(token, snippet{Folder: "Work", Name: "Review", Shortcut: "/review", Body: "old"})
	s.createSnippet(token, snippet{Folder: "Work", Name: "TLDR", Shortcut: "/tldr", Body: "old"})
	before := `[Work/Review /review "old" master=true][Work/TLDR /tldr "old" master=true]`

	for _, c := range []struct {
		set     []snippet
		message string
	}{
		{[]snippet{{Folder: "A", Name: "One", Shortcut: "/one", Body: "1"}, {Folder: "A", Name: "Two", Shortcut: "/ONE", Body: "2"}}, `"One\" already uses /one."`},
		{[]snippet{{Folder: "A", Name: "One", Shortcut: "/one", Body: "1"}, {Folder: "A", Name: "Two", Shortcut: "/o", Body: "2"}}, `/o would always fire before /one (\"One\") finished typing.`},
		{[]snippet{{Folder: "A", Name: "One", Shortcut: "/one", Body: "1"}, {Folder: "A", Name: " ", Shortcut: "/two", Body: "2"}}, "A name is needed."},
		{[]snippet{{Folder: "A", Name: "One", Shortcut: "/one", Body: "1"}, {Folder: "A", Name: "Two", Shortcut: "/t wo", Body: "2"}}, "Shortcuts cannot contain spaces."},
	} {
		res, body := s.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": c.set})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.message)
		if got := describe(s.snippets(token)); got != before {
			t.Fatalf("Snippets after a refused import = %s, want %s", got, before)
		}
	}

	off := false
	// The set replaces the old one, so it may reuse an old shortcut.
	res, body := s.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": []snippet{
		{Folder: "Kept", Name: "Review", Shortcut: "/review", Body: "new"},
		{Folder: "Kept", Name: "Yo", Shortcut: "/yo", Body: "Yo!", WithMasterPrompt: &off},
	}})
	expectStatus(t, res, body, http.StatusOK, `"snippets"`)
	var got struct{ Snippets []snippet }
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := `[Kept/Review /review "new" master=true][Kept/Yo /yo "Yo!" master=false]`
	if d := describe(got.Snippets); d != want {
		t.Fatalf("answer = %s, want %s", d, want)
	}
	if d := describe(s.snippets(token)); d != want {
		t.Fatalf("Snippets = %s, want %s", d, want)
	}

	res, body = s.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": []snippet{}})
	expectStatus(t, res, body, http.StatusOK, `"snippets":[]`)
	if got := s.snippets(token); len(got) != 0 {
		t.Fatalf("Snippets after importing none = %s", describe(got))
	}
}
