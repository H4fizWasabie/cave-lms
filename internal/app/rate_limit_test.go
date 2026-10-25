package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// A class on one network shares one address; only failed logins count, so everyone gets in.
func TestAClassLoggingInFromOneAddressAllGetsIn(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 30; i++ {
		s.second().loginAsNewLearner("Learner", fmt.Sprintf("learner%d@example.com", i), "correct horse battery")
	}
}

func TestTenWrongPasswordsBlockThatAccountFromThatAddressOnly(t *testing.T) {
	s := newSite(t)
	const password = "correct horse battery"
	s.second().loginAsNewLearner("Ada", "ada@example.com", password)
	s.second().loginAsNewLearner("Bea", "bea@example.com", password)
	for i := 0; i < 10; i++ {
		res, body := s.login("ADA@example.com", "wrong password")
		expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	}
	res, body := s.login("ada@example.com", password)
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	if len(res.Cookies()) != 0 {
		t.Fatalf("refused login set cookies: %v", res.Cookies())
	}
	res, body = s.login("bea@example.com", password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

// Guesses sent all at once still get only ten tries at the password.
func TestWrongPasswordsSentAtOnceGetTenTriesInAll(t *testing.T) {
	s := newSite(t)
	s.second().loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	var wg sync.WaitGroup
	codes := make(chan int, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := s.second().login("ada@example.com", "wrong password")
			codes <- res.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	tried := 0
	for code := range codes {
		if code == http.StatusUnauthorized {
			tried++
		}
	}
	if tried != 10 {
		t.Fatalf("%d of 30 simultaneous wrong passwords were checked, want 10", tried)
	}
}

// One password tried against many accounts from one address is still stopped.
func TestAHundredFailedLoginsFromOneAddressBlockIt(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 100; i++ {
		res, body := s.login(fmt.Sprintf("guess%d@example.com", i), "password123")
		expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	}
	res, body := s.login("guess100@example.com", "password123")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
}

// A whole class can sign up together from one network.
func TestRegisterIsRateLimitedPerClient(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 100; i++ {
		res, body := s.register("Visitor", fmt.Sprintf("v%d@example.com", i), "correct horse battery")
		expectStatus(t, res, body, http.StatusOK, "Check your email")
	}
	res, body := s.register("Visitor", "v100@example.com", "correct horse battery")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	if n := len(s.mail.Messages()); n != 100 {
		t.Fatalf("sent %d emails, want 100", n)
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
	return s.loginVia(forwardedFor, "nobody@example.com", "wrong password")
}

// loginVia posts a login through the TLS server (a loopback proxy) with the given X-Forwarded-For.
func (s *site) loginVia(forwardedFor, email, password string) (*http.Response, string) {
	form := url.Values{"email": {email}, "password": {password}}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", forwardedFor)
	return s.do(req)
}

// Guesses at one email spread over many addresses stop at the per-email limit, which refuses even the right password
// from a new address, like the per-account one. The Owner sees one masked line in the System log, and a password
// reset still works.
func TestFailedLoginsToOneEmailFromManyAddressesBlockItAndResetStillWorks(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.second().loginAsNewLearner("Ada", email, password)
	for i := range 50 {
		res, body := s.loginVia(fmt.Sprintf("198.51.100.%d", i/10), "ADA@example.com", "wrong password")
		expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	}
	for range 2 {
		res, body := s.loginVia("198.51.100.99", email, password)
		expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	}
	var lines int
	if err := s.db.QueryRow(`SELECT count(*) FROM system_log WHERE message LIKE '%"a***@example.com"%'`).Scan(&lines); err != nil {
		t.Fatal(err)
	}
	var full int
	if err := s.db.QueryRow(`SELECT count(*) FROM system_log WHERE message LIKE '%ada@%'`).Scan(&full); err != nil {
		t.Fatal(err)
	}
	if lines != 1 || full != 0 {
		t.Fatalf("System log has %d masked lines and %d with the full email, want 1 and 0", lines, full)
	}

	res, body := s.forgotPassword(email)
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	res, body = s.resetPassword(resetToken(t, s.lastEmailLinkPath(email, "/reset-password")), "new horse staple")
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")
	// The reset leaves the count: logins stay refused until the hour has passed.
	res, body = s.loginVia("198.51.100.99", email, "new horse staple")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
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

// Each reset try hashes a password with bcrypt, so without a limit anyone could keep the server's CPUs busy.
func TestForgotPasswordResendAndResetAreRateLimited(t *testing.T) {
	for _, path := range []string{"/forgot-password", "/verify/resend", "/reset-password"} {
		s := newSite(t)
		form := url.Values{"email": {"ada@example.com"}, "token": {"not-a-token"}, "password": {"correct horse battery"}}
		for i := 0; i < 10; i++ {
			s.post(path, form)
		}
		res, body := s.post(path, form)
		expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")
	}
}

func TestFailedLoginsDoNotBlockAPasswordReset(t *testing.T) {
	s := newSite(t)
	for i := 0; i < 10; i++ {
		s.login("ada@example.com", "wrong password")
	}
	res, body := s.login("ada@example.com", "wrong password")
	expectStatus(t, res, body, http.StatusTooManyRequests, "Too many attempts")

	// Each form has its own budget, so a Learner locked out of login can still ask for a reset.
	res, body = s.post("/forgot-password", url.Values{"email": {"ada@example.com"}})
	if res.StatusCode == http.StatusTooManyRequests {
		t.Fatalf("forgot-password refused after failed logins: %s", body)
	}
}
