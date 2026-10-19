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
