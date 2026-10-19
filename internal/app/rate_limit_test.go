package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginIsRateLimitedPerClient(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	// Registration and login share one budget of ten attempts per address; this setup uses two.
	s.loginAsNewLearner("Ada", email, password)

	for i := 0; i < 7; i++ {
		res, body := s.login(email, "wrong password")
		expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	}
	res, body := s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.login(email, "wrong password")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	if len(res.Cookies()) != 0 {
		t.Fatalf("refused login set cookies: %v", res.Cookies())
	}
}

func TestRegisterIsRateLimitedPerClient(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 10; i++ {
		res, body := s.register("Visitor", fmt.Sprintf("v%d@example.com", i), "correct horse battery")
		expectStatus(t, res, body, http.StatusOK, "Check your email")
	}
	res, body := s.register("Visitor", "v10@example.com", "correct horse battery")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	if n := len(s.mail.Messages()); n != 10 {
		t.Fatalf("sent %d emails, want 10", n)
	}
}

func TestRateLimitCountsEachClientAddressSeparately(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 10; i++ {
		s.login("nobody@example.com", "wrong password")
	}
	res, body := s.login("nobody@example.com", "wrong password")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")

	// Another client address still gets through.
	form := url.Values{"email": {"nobody@example.com"}, "password": {"wrong password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.7:50000"
	recorder := httptest.NewRecorder()
	s.application.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("POST /login from another address = %d, want 401", recorder.Code)
	}
}

// loginFrom posts a wrong login through the TLS server (a loopback proxy) with the given X-Forwarded-For.
func (s *site) loginFrom(forwardedFor string) (*http.Response, string) {
	form := url.Values{"email": {"nobody@example.com"}, "password": {"wrong password"}}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", forwardedFor)
	return s.do(req)
}

func TestBehindALocalProxyEachForwardedClientHasItsOwnBudget(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 10; i++ {
		s.loginFrom("198.51.100.1")
	}
	res, body := s.loginFrom("198.51.100.1")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	// The proxy appends the real client last; a forged first entry changes nothing.
	res, body = s.loginFrom("198.51.100.2, 198.51.100.1")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")

	res, body = s.loginFrom("198.51.100.2")
	expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
}

func TestForwardedForFromANonLocalAddressIsIgnored(t *testing.T) {
	s := newSite(t)
	post := func(i int) int {
		form := url.Values{"email": {"nobody@example.com"}, "password": {"wrong password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		req.RemoteAddr = "203.0.113.7:50000"
		recorder := httptest.NewRecorder()
		s.application.ServeHTTP(recorder, req)
		return recorder.Code
	}
	for i := 0; i < 10; i++ {
		post(i)
	}
	if code := post(10); code != http.StatusTooManyRequests {
		t.Fatalf("11th login from one address with a new X-Forwarded-For = %d, want 429", code)
	}
}

func TestForgotPasswordAndResendAreRateLimited(t *testing.T) {
	for _, path := range []string{"/forgot-password", "/verify/resend"} {
		s := newSite(t)
		for i := 0; i < 10; i++ {
			s.post(path, url.Values{"email": {"ada@example.com"}})
		}
		res, body := s.post(path, url.Values{"email": {"ada@example.com"}})
		expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	}
}
