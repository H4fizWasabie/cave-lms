package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func expectAll(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestTheCaveTextPageIsInMalayAndEnglish(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)

	visitor := s.second()
	_, body := visitor.getAsking("/cavetext", "ms")
	expectLanguage(t, body, "ms", "Sehingga 2,000 Snippet")
	expectAll(t, body, "RM25 setahun", "RM99 setahun", "Pilih apa yang hendak dibeli", "Log masuk untuk melihat Snippet anda", `href="/login?next=%2Fcavetext">Log masuk</a>`, "Dapatkan sambungan CaveText untuk Chrome")
	_, body = visitor.getAsking("/cavetext", "en")
	expectLanguage(t, body, "en", "Up to 2,000 Snippets")
	expectAll(t, body, "RM25 a year", "Log in to see your Snippets")

	cy := s.second()
	cy.loginAsNewLearner("Cy Tan", "cy@example.com", "correct horse battery")
	_, body = cy.getAsking("/cavetext", "ms")
	expectLanguage(t, body, "ms", "Log masuk ke CaveText dengan akaun CAVELMS ini")
	expectAll(t, body, "5 Snippet percuma", "Hubungi kami untuk membeli CaveText Pro", "Log masuk ke CaveText dahulu")

	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 3)
	_, body = ada.getAsking("/cavetext", "ms")
	expectLanguage(t, body, "ms", "3 daripada 5 Snippet")
	expectAll(t, body, "Percuma", "Hubungi kami untuk membeli CaveText Pro", "Hubungi kami untuk membeli Pelan Pro")
	_, body = ada.getAsking("/cavetext", "en")
	expectLanguage(t, body, "en", "3 of 5 Snippets")

	s.savePrices(map[string]string{"cavetext-pro_link": "https://bcl.my/pay/cavetext"})
	s.givePro("ada@example.com")
	until := ada.me(token).ProUntil.UTC()
	_, body = ada.getAsking("/cavetext", "ms")
	expectLanguage(t, body, "ms", "CaveText Pro hingga "+text.Date("ms", until))
	expectAll(t, body, "3 Snippet, sehingga 2,000", "Perbaharui CaveText Pro</a>")
	_, body = ada.getAsking("/cavetext", "en")
	expectLanguage(t, body, "en", "CaveText Pro until "+text.Date("en", until))
	expectAll(t, body, "3 Snippets, up to 2,000", "Renew CaveText Pro</a>")
}

// createSnippetAsking creates a Snippet from a browser asking for acceptLanguage, and returns the error shown.
func (s *site) createSnippetAsking(token, acceptLanguage string, sn snippet) string {
	s.t.Helper()
	b, _ := json.Marshal(sn)
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/cavetext/api/snippets", bytes.NewReader(b))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", acceptLanguage)
	res, body := s.do(req)
	var got struct{ Error string }
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Error == "" {
		s.t.Fatalf("status %d, body %s: want an error message", res.StatusCode, body)
	}
	return got.Error
}

func TestCaveTextMessagesComeInTheSavedLanguageElseTheBrowsers(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)
	blank := snippet{Folder: "Work", Name: " ", Shortcut: "/x", Body: "x"}

	ada, adaToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("ms", "/")
	if got := ada.createSnippetAsking(adaToken, "en", blank); got != "Nama diperlukan." {
		t.Errorf("saved Malay, browser English: %q", got)
	}
	ada.fillSnippets(adaToken, 5)
	want := "Anda mempunyai 5 daripada 5 Snippet percuma anda. Padam satu untuk menambah yang lain, atau dapatkan CaveText Pro untuk sehingga 2,000 Snippet."
	if got := ada.createSnippetAsking(adaToken, "en", snippet{Folder: "Work", Name: "More", Shortcut: "/more", Body: "x"}); got != want {
		t.Errorf("cap reached in Malay = %q", got)
	}

	_, cyToken := s.newCaveTextLearner("Cy Tan", "cy@example.com")
	if got := s.createSnippetAsking(cyToken, "ms-MY,en;q=0.8", blank); got != "Nama diperlukan." {
		t.Errorf("no saved language, browser Malay: %q", got)
	}
	if got := s.createSnippetAsking(cyToken, "en-GB", blank); got != "A name is needed." {
		t.Errorf("no saved language, browser English: %q", got)
	}
}
