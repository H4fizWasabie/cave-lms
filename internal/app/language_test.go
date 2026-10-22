package app_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

var languageSwitch = regexp.MustCompile(`(?s)<form class="lang-switch".*?</form>`)

// getAsking requests path with the browser asking for acceptLanguage.
func (s *site) getAsking(path, acceptLanguage string) (*http.Response, string) {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodGet, s.server.URL+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", acceptLanguage)
	return s.do(req)
}

func (s *site) switchLanguage(lang, next string) (*http.Response, string) {
	return s.post("/language", url.Values{"lang": {lang}, "next": {next}})
}

func expectLanguage(t *testing.T, body, lang, wantText string) {
	t.Helper()
	if !strings.Contains(body, `<html lang="`+lang+`">`) || !strings.Contains(body, wantText) {
		t.Fatalf("page is not in %s with %q; body:\n%s", lang, wantText, body)
	}
}

func TestAVisitorGetsTheBrowsersLanguageElseMalay(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en-GB,en;q=0.9")
	expectLanguage(t, body, "en", "Welcome to CAVELMS")
	_, body = s.getAsking("/", "ms-MY")
	expectLanguage(t, body, "ms", "Selamat Datang ke CAVELMS")
	_, body = s.getAsking("/", "fr-FR,de;q=0.8")
	expectLanguage(t, body, "ms", "Selamat Datang ke CAVELMS")

	recorder := httptest.NewRecorder()
	s.application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	expectLanguage(t, recorder.Body.String(), "ms", "Log masuk")
}

func TestTheSwitchReturnsToTheSamePageInTheNewLanguageAndIsRemembered(t *testing.T) {
	s := newSite(t)
	res, body := s.switchLanguage("en", "/gallery?format=ebook")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/gallery?format=ebook" {
		t.Fatalf("switch went to %q, want back to /gallery?format=ebook", got)
	}
	_, body = s.getAsking("/", "ms")
	expectLanguage(t, body, "en", "Welcome to CAVELMS")

	s.switchLanguage("ms", "/")
	_, body = s.getAsking("/", "en")
	expectLanguage(t, body, "ms", "Selamat Datang ke CAVELMS")
}

func TestTheSwitchRefusesAnOffSiteReturnPathAndAnUnknownLanguage(t *testing.T) {
	s := newSite(t)
	for _, next := range []string{"https://evil.example/", "//evil.example", "/\t/evil.example", "/\\evil.example"} {
		res, body := s.switchLanguage("en", next)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		if got := res.Header.Get("Location"); got != "/" {
			t.Errorf("switch with next %q went to %q, want /", next, got)
		}
	}
	res, body := s.switchLanguage("fr", "/")
	expectStatus(t, res, body, http.StatusBadRequest, "")

	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/language", strings.NewReader(url.Values{"lang": {"en"}, "next": {"/"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, body = s.do(req)
	expectStatus(t, res, body, http.StatusForbidden, "cross-origin request refused")
}

func TestALearnersSavedLanguageWinsOverTheCookieAndIsAppliedAtLogin(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	s.switchLanguage("en", "/")

	other := s.second()
	other.switchLanguage("ms", "/")
	res, body := other.login("ada@example.com", "correct horse battery")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = other.getAsking("/", "ms")
	expectLanguage(t, body, "en", "Logged in as Ada Lovelace")

	other.post("/logout", nil)
	_, body = other.getAsking("/", "ms")
	expectLanguage(t, body, "en", "Welcome to CAVELMS")
}

func TestEveryTemplateTextIsInTheCatalog(t *testing.T) {
	keys, err := app.TemplateTextKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) == 0 {
		t.Fatal("found no texts in the templates")
	}
	for _, key := range keys {
		if !text.Has(key) {
			t.Errorf("a template asks for %q, which the catalog lacks", key)
		}
	}
}
