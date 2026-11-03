package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func (s *site) forgotPassword(email string) (*http.Response, string) {
	return s.post("/forgot-password", url.Values{"email": {email}})
}

func TestForgotPasswordGivesTheSameResponseWhetherOrNotTheEmailExists(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/login")
	expectStatus(t, res, body, http.StatusOK, "Forgot your password?")
	if !strings.Contains(body, `href="/forgot-password"`) {
		t.Fatalf("login page does not link to /forgot-password: %s", body)
	}
}

// The keyboard goes email, password, Log in, and only then to "Forgot your password?", so it never sits between the two fields.
func TestTheLoginPageTabsFromEmailToPasswordToLogInThenForgotPassword(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.get("/login")
	form, rest, _ := strings.Cut(mainOf(body), "</button>")
	if strings.Contains(form, "/forgot-password") || strings.Contains(body, "tabindex") {
		t.Fatalf("forgot password comes before the Log in button; body:\n%s", mainOf(body))
	}
	expectInOrder(t, form, `id="email"`, `id="password"`, `<button`)
	expectInOrder(t, rest, `href="/forgot-password"`)
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
	t.Parallel()
	s := newSite(t)
	const email, oldPassword, newPassword = "ada@example.com", "correct horse battery", "new horse staple"
	s.loginAsNewLearner("Ada", email, oldPassword)

	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	res, body := s.get(link)
	expectStatus(t, res, body, http.StatusOK, "Choose a new password")

	res, body = s.resetPassword(resetToken(t, link), "short")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "at least 8 characters")
	expectFieldError(t, body, "password", "at least 8 characters", "</button>")

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// The email goes out after the answer, so a mail service that is slow, or down until released, makes an account that
// exists answer no slower than one that does not.
func TestAMailServiceThatStallsDoesNotHoldBackTheAnswerForAnExistingAccount(t *testing.T) {
	t.Parallel()
	const password = "correct horse battery"
	for _, path := range []string{"/forgot-password", "/verify/resend"} {
		s := newSite(t)
		s.register("Ada", "ada@example.com", password) // left unverified, so both forms send
		before := len(s.mail.Messages())
		release := s.mail.HoldSends()
		defer release() // so a failure below never leaves the request waiting

		req := s.newRequest(http.MethodPost, path, strings.NewReader(url.Values{"email": {"ada@example.com"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		answered := make(chan *http.Response, 1)
		go func() {
			res, err := s.client.Do(req)
			if err != nil {
				s.t.Error(err)
				close(answered)
				return
			}
			res.Body.Close()
			answered <- res
		}()
		select {
		case res := <-answered:
			if res == nil || res.StatusCode != http.StatusOK {
				t.Fatalf("POST %s = %v, want 200", path, res)
			}
		case <-time.After(10 * time.Second): // only the bound on a failure: a pass does not wait on it
			t.Fatalf("POST %s did not answer while the mail service held the email", path)
		}
		if n := len(s.mail.Messages()); n != before {
			t.Fatalf("POST %s: %d emails were sent while the mail service held them, want none", path, n-before)
		}

		release()
		s.application.WaitForMail()
		if n := len(s.mail.Messages()); n != before+1 {
			t.Fatalf("POST %s: %d emails sent once the mail service released them, want 1", path, n-before)
		}
	}
}

// A reset link proves the Learner reads that email, so an unverified account is verified by the reset.
func TestAResetVerifiesTheEmail(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email = "ada@example.com"
	res, body := s.register("Ada", email, "correct horse battery")
	expectStatus(t, res, body, http.StatusOK, "Check your email")

	s.forgotPassword(email)
	res, body = s.resetPassword(resetToken(t, s.lastEmailLinkPath(email, "/reset-password")), "new horse staple")
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")

	res, body = s.login(email, "new horse staple")
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestResetToAPasswordOver72BytesIsRefusedWithAMessageNotAServerError(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)
	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	res, body := s.resetPassword(resetToken(t, link), strings.Repeat("é", 37)) // 37 characters, 74 bytes
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Use a password of at most 72 characters.")
	res, body = s.resetPassword(resetToken(t, link), "new horse staple") // the link was not spent
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")
}

func TestAResetWhenTheDatabaseFailsIsAServerErrorAndChangesNothing(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.loginAsNewLearner("Ada", email, password)
	s.forgotPassword(email)
	link := s.lastEmailLinkPath(email, "/reset-password")
	if _, err := s.db.Exec(`ALTER TABLE one_time_tokens RENAME TO one_time_tokens_gone`); err != nil {
		t.Fatal(err)
	}
	res, body := s.resetPassword(resetToken(t, link), "new horse staple")
	expectStatus(t, res, body, http.StatusInternalServerError, "")
	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}
