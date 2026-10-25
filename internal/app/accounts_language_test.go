package app_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

// postAsking posts form to path with the browser asking for acceptLanguage.
func (s *site) postAsking(path, acceptLanguage string, form url.Values) (*http.Response, string) {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", acceptLanguage)
	return s.do(req)
}

func TestTheAccountPagesAreInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	for path, want := range map[string][2]string{
		"/register":              {"Cipta akaun percuma untuk menyertai kursus.", "Create a free account to join courses."},
		"/login":                 {"Lupa kata laluan anda?", "Forgot your password?"},
		"/forgot-password":       {"Hantar pautan set semula", "Send reset link"},
		"/reset-password?token=": {"Tetapkan kata laluan baharu", "Set new password"},
	} {
		_, body := s.getAsking(path, "ms")
		expectLanguage(t, body, "ms", want[0])
		_, body = s.getAsking(path, "en")
		expectLanguage(t, body, "en", want[1])
	}
}

func TestAccountFormErrorsAndAnswersAreInMalay(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	register := func(name, email, password string) (*http.Response, string) {
		return s.postAsking("/register", "ms", url.Values{"name": {name}, "email": {email}, "password": {password}})
	}
	for _, c := range []struct{ name, email, password, want string }{
		{"", email, password, "Masukkan nama anda"},
		{"Ada", "not an email", password, "Masukkan alamat e-mel yang sah"},
		{"Ada", email, "short", "sekurang-kurangnya 8 aksara"},
		{"Ada", email, strings.Repeat("x", 73), "paling banyak 72 aksara"},
	} {
		res, body := register(c.name, c.email, c.password)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	res, body := register("Ada", email, password)
	expectStatus(t, res, body, http.StatusOK, "Semak e-mel anda")
	verify := s.lastVerifyPath(email)
	res, body = register("Ada", email, password)
	expectStatus(t, res, body, http.StatusOK, "Semak e-mel anda")
	if got := s.lastEmailTo(email).Subject; got != "Tetapkan kata laluan CAVELMS anda" {
		t.Fatalf("registering again mailed %q, want the Malay set-password email", got)
	}

	login := func(password string) (*http.Response, string) {
		return s.postAsking("/login", "ms", url.Values{"email": {email}, "password": {password}})
	}
	res, body = login(password)
	expectStatus(t, res, body, http.StatusForbidden, "Sahkan e-mel anda dahulu")
	res, body = s.getAsking(verify, "ms") // registering again ended the first verification link
	expectStatus(t, res, body, http.StatusBadRequest, "Pautan tidak sah")
	s.postAsking("/verify/resend", "ms", url.Values{"email": {email}})
	res, body = s.getAsking(s.lastVerifyPath(email), "ms")
	expectStatus(t, res, body, http.StatusOK, "E-mel disahkan")
	res, body = s.getAsking("/verify?token=nope", "ms")
	expectStatus(t, res, body, http.StatusBadRequest, "Pautan tidak sah")
	res, body = s.postAsking("/verify/resend", "ms", url.Values{"email": {email}})
	expectStatus(t, res, body, http.StatusOK, "Semak e-mel anda")
	res, body = login("wrong password")
	expectStatus(t, res, body, http.StatusUnauthorized, "E-mel atau kata laluan salah")

	res, body = s.postAsking("/forgot-password", "ms", url.Values{"email": {email}})
	expectStatus(t, res, body, http.StatusOK, "kami telah menghantar pautan set semula")
	token := resetToken(t, s.lastEmailLinkPath(email, "/reset-password"))
	res, body = s.postAsking("/reset-password", "ms", url.Values{"token": {token}, "password": {"short"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "sekurang-kurangnya 8 aksara")
	res, body = s.postAsking("/reset-password", "ms", url.Values{"token": {token}, "password": {"another good password"}})
	expectStatus(t, res, body, http.StatusOK, "Kata laluan ditukar")
	res, body = s.postAsking("/reset-password", "ms", url.Values{"token": {token}, "password": {"another good password"}})
	expectStatus(t, res, body, http.StatusBadRequest, "Pautan set semula ini tidak sah")
}

// lastEmailTo returns the latest email sent to `to`.
func (s *site) lastEmailTo(to string) mailer.Message {
	s.t.Helper()
	messages := s.mail.Messages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].To == to {
			return messages[i]
		}
	}
	s.t.Fatalf("no email sent to %s", to)
	return mailer.Message{}
}

func expectEmail(t *testing.T, m mailer.Message, subject, body string) {
	t.Helper()
	if m.Subject != subject || !strings.Contains(m.Body, body) {
		t.Fatalf("email to %s = %q\n%s\nwant subject %q containing %q", m.To, m.Subject, m.Body, subject, body)
	}
}

func TestVerifyAndResetEmailsComeInTheSavedLanguageElseTheRequests(t *testing.T) {
	s := newSite(t)
	const ada, cy, password = "ada@example.com", "cy@example.com", "correct horse battery"

	// No account yet, so no saved language: the email follows the page it was asked from.
	s.postAsking("/register", "ms", url.Values{"name": {"Ada"}, "email": {ada}, "password": {password}})
	expectEmail(t, s.lastEmailTo(ada), "Sahkan akaun CAVELMS anda", "Hai,")
	s.postAsking("/verify/resend", "en", url.Values{"email": {ada}})
	expectEmail(t, s.lastEmailTo(ada), "Confirm your CAVELMS account", "Hi,")
	s.postAsking("/forgot-password", "ms", url.Values{"email": {ada}})
	expectEmail(t, s.lastEmailTo(ada), "Set semula kata laluan CAVELMS anda", "dalam masa sejam")

	// A saved language wins over the browser the request comes from.
	s.loginAsNewLearner("Cy", cy, password)
	s.switchLanguage("en", "/")
	s.post("/logout", nil)
	s.client.Jar, _ = cookiejar.New(nil)
	s.postAsking("/forgot-password", "ms", url.Values{"email": {cy}})
	expectEmail(t, s.lastEmailTo(cy), "Reset your CAVELMS password", "within an hour")
}

func TestTheTooManyAttemptsPageIsInMalay(t *testing.T) {
	s := newSite(t)
	var res *http.Response
	var body string
	for range 11 {
		res, body = s.postAsking("/forgot-password", "ms", url.Values{"email": {"ada@example.com"}})
	}
	expectStatus(t, res, body, http.StatusTooManyRequests, "Terlalu banyak cubaan")
}
