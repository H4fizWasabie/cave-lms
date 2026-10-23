package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (s *site) forgotPassword(email string) (*http.Response, string) {
	return s.post("/forgot-password", url.Values{"email": {email}})
}

func TestForgotPasswordGivesTheSameResponseWhetherOrNotTheEmailExists(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)

	res, known := s.forgotPassword(email)
	expectStatus(t, res, known, http.StatusOK, "Check your email")
	if n := len(s.mail.Messages()); n != 2 { // verification email plus the reset email
		t.Fatalf("sent %d emails, want 2", n)
	}
	s.lastEmailLinkPath(email, "/reset-password")

	const unknown = "nobody@example.com"
	res, body := s.forgotPassword(unknown)
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	if want := strings.ReplaceAll(known, email, unknown); body != want {
		t.Fatalf("response for unknown email differs from known one:\n%s\nwant:\n%s", body, want)
	}
	if n := len(s.mail.Messages()); n != 2 {
		t.Fatalf("unknown email sent mail: %d messages, want 2", n)
	}
}

func TestLoginPageLinksToPasswordReset(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/login")
	expectStatus(t, res, body, http.StatusOK, "Forgot your password?")
	if !strings.Contains(body, `href="/forgot-password"`) {
		t.Fatalf("login page does not link to /forgot-password: %s", body)
	}
}

// resetToken returns the token from a reset link path such as /reset-password?token=abc.
func resetToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func (s *site) resetPassword(token, password string) (*http.Response, string) {
	return s.post("/reset-password", url.Values{"token": {token}, "password": {password}})
}

func TestResetLinkSetsANewPasswordOnce(t *testing.T) {
	s := newSite(t)
	const email, oldPassword, newPassword = "ada@example.com", "correct horse battery", "new horse staple"
	s.loginAsNewLearner("Ada", email, oldPassword)

	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	res, body := s.get(link)
	expectStatus(t, res, body, http.StatusOK, "Choose a new password")

	res, body = s.resetPassword(resetToken(t, link), "short")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "at least 8 characters")

	res, body = s.resetPassword(resetToken(t, link), newPassword)
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")

	res, body = s.login(email, oldPassword)
	expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	res, body = s.login(email, newPassword)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = s.resetPassword(resetToken(t, link), "another password")
	expectStatus(t, res, body, http.StatusBadRequest, "already been used")
}

func TestExpiredResetLinkIsRefused(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)

	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	if _, err := s.db.ExecContext(context.Background(), `UPDATE one_time_tokens SET expires_at = now() - interval '1 minute' WHERE purpose = 'reset_password'`); err != nil {
		t.Fatal(err)
	}

	res, body := s.resetPassword(resetToken(t, link), "new horse staple")
	expectStatus(t, res, body, http.StatusGone, "This reset link has expired")
	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestResetSignsOutEverySessionOfTheLearnerAndCaveText(t *testing.T) {
	s := newCaveTextSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)
	caveTextToken := s.connect()
	res, body := s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	session := res.Cookies()[0]

	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	res, body = s.resetPassword(resetToken(t, link), "new horse staple")
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")

	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: session.Name, Value: session.Value})
	res, body = s.do(req)
	if strings.Contains(body, "Logged in") {
		t.Fatalf("old session still logs in after a password reset: %s", body)
	}
	res, body = s.caveText(http.MethodGet, "me", caveTextToken)
	expectSignedOut(t, res, body)
}

func TestAFailedEmailSendStillGivesTheNormalAnswer(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)
	s.register("Grace", "grace@example.com", password) // left unverified
	s.mail.FailWith(errors.New("mail server down"))

	res, body := s.forgotPassword(email)
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	res, body = s.post("/verify/resend", url.Values{"email": {"grace@example.com"}})
	expectStatus(t, res, body, http.StatusOK, "we sent it a new link")
}
