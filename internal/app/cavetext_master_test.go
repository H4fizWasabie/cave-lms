package app_test

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
)

const defaultQuestion = "Based on everything you know about me"

func (s *site) saveUserMasterPrompt(token, text string) (*http.Response, string) {
	s.t.Helper()
	return s.caveTextSend(http.MethodPut, "user-master-prompt", token, map[string]string{"text": text})
}

func (s *site) deleteUserMasterPrompt(token string) {
	s.t.Helper()
	res, body := s.caveText(http.MethodDelete, "user-master-prompt", token)
	expectStatus(s.t, res, body, http.StatusNoContent, "")
}

func userMasterPrompt(me caveTextMe) string {
	if me.UserMasterPrompt == nil {
		return "<none>"
	}
	return *me.UserMasterPrompt
}

func TestAProLearnerSavesOneUserMasterPromptAndDeletesItToReplaceIt(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	me := ada.me(token)
	if me.UserMasterPrompt != nil || !strings.HasPrefix(me.MasterPromptQuestion, defaultQuestion) {
		t.Fatalf("me = %+v, want no User master prompt and the default question", me)
	}

	const first = "I am Ada, a mathematician.\n\nI like short answers."
	res, body := ada.saveUserMasterPrompt(token, first)
	expectStatus(t, res, body, http.StatusOK, `"userMasterPrompt"`)
	if got := userMasterPrompt(ada.me(token)); got != first {
		t.Fatalf("User master prompt = %q, want %q", got, first)
	}

	res, body = ada.saveUserMasterPrompt(token, "Someone else")
	expectStatus(t, res, body, http.StatusConflict, "Delete it first")
	if got := userMasterPrompt(ada.me(token)); got != first {
		t.Fatalf("after a refused overwrite: %q, want the first kept", got)
	}

	ada.deleteUserMasterPrompt(token)
	if me := ada.me(token); me.UserMasterPrompt != nil {
		t.Fatalf("after delete: me = %+v, want none", me)
	}
	ada.deleteUserMasterPrompt(token) // deleting none is fine
	res, body = ada.saveUserMasterPrompt(token, "I am Ada, now an engineer.")
	expectStatus(t, res, body, http.StatusOK, "now an engineer")
}

// Two browsers saving at the same moment: one is kept, the other is told to delete it first, never both overwriting.
func TestTwoUserMasterPromptsSavedAtOnceKeepOneAndRefuseTheOther(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")

	texts := []string{"I am Ada, from the laptop.", "I am Ada, from the phone."}
	statuses := make([]int, len(texts))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, _ := ada.saveUserMasterPrompt(token, text)
			statuses[i] = res.StatusCode
		}()
	}
	close(start)
	wg.Wait()

	got := append([]int(nil), statuses...)
	sort.Ints(got)
	if got[0] != http.StatusOK || got[1] != http.StatusConflict {
		t.Fatalf("statuses = %v, want one 200 and one 409", statuses)
	}
	kept := texts[0]
	if statuses[1] == http.StatusOK {
		kept = texts[1]
	}
	if got := userMasterPrompt(ada.me(token)); got != kept {
		t.Fatalf("User master prompt = %q, want the one answered 200, %q", got, kept)
	}
}

func TestAUserMasterPromptOverEightThousandCharactersIsRefused(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")

	res, body := ada.saveUserMasterPrompt(token, strings.Repeat("é", 8001))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "8,000 characters")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "8,001")
	res, body = ada.saveUserMasterPrompt(token, " \n\t")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Paste")
	if me := ada.me(token); me.UserMasterPrompt != nil {
		t.Fatalf("me = %+v, want nothing saved", me)
	}
	// Characters, not bytes: 8,000 two-byte letters fit.
	res, body = ada.saveUserMasterPrompt(token, strings.Repeat("é", 8000))
	expectStatus(t, res, body, http.StatusOK, "")
}

func TestSavingAUserMasterPromptNeedsProAndALapsedOneIsKeptAndDeletable(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")

	res, body := ada.saveUserMasterPrompt(token, "I am Ada.")
	expectStatus(t, res, body, http.StatusForbidden, "CaveText Pro")
	if me := ada.me(token); me.UserMasterPrompt != nil {
		t.Fatalf("free: me = %+v, want nothing saved", me)
	}

	s.givePro("ada@example.com")
	res, body = ada.saveUserMasterPrompt(token, "I am Ada.")
	expectStatus(t, res, body, http.StatusOK, "")
	s.agePro(400)
	me := ada.me(token)
	if me.Pro || userMasterPrompt(me) != "I am Ada." {
		t.Fatalf("lapsed: me = %+v, want free with the User master prompt kept", me)
	}
	ada.deleteUserMasterPrompt(token)
	res, body = ada.saveUserMasterPrompt(token, "I am Ada again.")
	expectStatus(t, res, body, http.StatusForbidden, "CaveText Pro")

	s.givePro("ada@example.com")
	res, body = ada.saveUserMasterPrompt(token, "I am Ada again.")
	expectStatus(t, res, body, http.StatusOK, "")
}

func TestTheUserMasterPromptIsNotASnippetAndNeverCountsTowardTheCap(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	res, body := ada.saveUserMasterPrompt(token, "I am Ada.")
	expectStatus(t, res, body, http.StatusOK, "")
	if me := ada.me(token); me.SnippetCount != 0 {
		t.Fatalf("me = %+v, want no Snippets", me)
	}
	if list := ada.snippets(token); len(list) != 0 {
		t.Fatalf("Snippets = %s, want none", describe(list))
	}

	s.agePro(400)
	ada.fillSnippets(token, 5)
	if me := ada.me(token); me.Cap != 5 || me.SnippetCount != 5 || userMasterPrompt(me) != "I am Ada." {
		t.Fatalf("me = %+v, want 5 of 5 with the User master prompt kept", me)
	}
}

func (s *site) saveCaveTextQuestion(question string) (*http.Response, string) {
	s.t.Helper()
	return s.post("/admin/cavetext/settings", url.Values{"price": {"25"}, "payment_link": {""}, "question": {question}})
}

func TestAnyAdminEditsTheQuestionShownBesideTheUserMasterPromptBox(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	res, body := s.get("/admin/cavetext/settings")
	expectStatus(t, res, body, http.StatusOK, `name="question"`)
	expectStatus(t, res, body, http.StatusOK, defaultQuestion)

	for _, bad := range []string{"", "  \n", strings.Repeat("x", 2001)} {
		res, body = s.saveCaveTextQuestion(bad)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "question")
	}
	if me := ada.me(token); !strings.HasPrefix(me.MasterPromptQuestion, defaultQuestion) {
		t.Fatalf("after refused saves: question = %q, want the default", me.MasterPromptQuestion)
	}

	const asked = "What do you know about me? Write it for another chatbot."
	res, body = s.saveCaveTextQuestion(asked)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Saved.")
	expectStatus(t, res, body, http.StatusOK, asked)
	// A free Learner gets the question too, for the locked box.
	if me := ada.me(token); me.MasterPromptQuestion != asked {
		t.Fatalf("question = %q, want %q", me.MasterPromptQuestion, asked)
	}
}
