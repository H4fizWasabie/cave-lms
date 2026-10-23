package app_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

type site struct {
	t           *testing.T
	application *app.App
	server      *httptest.Server
	client      *http.Client
	mail        *testutil.RecordingMailer
	ai          *testutil.FakeWriter
	db          *sql.DB
	storageDir  string
	cfg         config.Config
}

// newSite serves the app over TLS (session cookies are Secure) to a client
// that keeps cookies and does not follow redirects.
func newSite(t *testing.T, opts ...func(*config.Config)) *site {
	t.Helper()
	var cfg config.Config
	ai := &testutil.FakeWriter{}
	application, mail, db, storageDir := newTestApplicationWith(t, ai, append(opts, func(c *config.Config) { cfg = *c })...)
	server := httptest.NewTLSServer(application)
	t.Cleanup(server.Close)
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &site{t: t, application: application, server: server, client: client, mail: mail, ai: ai, db: db, storageDir: storageDir, cfg: cfg}
}

// do sends req, from a browser asking for English unless req says otherwise, as the tests check English texts.
func (s *site) do(req *http.Request) (*http.Response, string) {
	s.t.Helper()
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "en")
	}
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return res, string(body)
}

func (s *site) get(path string) (*http.Response, string) {
	req, err := http.NewRequest(http.MethodGet, s.server.URL+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	return s.do(req)
}

func (s *site) post(path string, form url.Values) (*http.Response, string) {
	req, err := http.NewRequest(http.MethodPost, s.server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(req)
}

func (s *site) register(name, email, password string) (*http.Response, string) {
	return s.post("/register", url.Values{"name": {name}, "email": {email}, "password": {password}})
}

func (s *site) login(email, password string) (*http.Response, string) {
	return s.post("/login", url.Values{"email": {email}, "password": {password}})
}

// lastEmailLinkPath returns the path and query of the link under route in the latest email to `to`, past any PUBLIC_BASE_URL path.
func (s *site) lastEmailLinkPath(to, route string) string {
	s.t.Helper()
	link := regexp.MustCompile(`https://cave\.example\.invalid\S*?(` + regexp.QuoteMeta(route) + `\?token=[A-Za-z0-9_-]+)`)
	message := s.lastEmailTo(to)
	m := link.FindStringSubmatch(message.Body)
	if m == nil {
		s.t.Fatalf("email to %s has no %s link: %s", to, route, message.Body)
	}
	return m[1]
}

// lastVerifyPath returns the path of the verification link in the latest email to `to`.
func (s *site) lastVerifyPath(to string) string {
	return s.lastEmailLinkPath(to, "/verify")
}

// loginAsNewLearner registers, verifies and logs in a new Learner.
func (s *site) loginAsNewLearner(name, email, password string) {
	s.t.Helper()
	res, body := s.register(name, email, password)
	expectStatus(s.t, res, body, http.StatusOK, "Check your email")
	res, body = s.get(s.lastVerifyPath(email))
	expectStatus(s.t, res, body, http.StatusOK, "Email verified")
	res, body = s.login(email, password)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// loginAsNewAdmin makes an Admin with the same CreateAdmin step the command line uses, then logs in.
func (s *site) loginAsNewAdmin(name, email, password string) {
	s.t.Helper()
	if err := s.application.CreateAdmin(context.Background(), name, email, password); err != nil {
		s.t.Fatal(err)
	}
	res, body := s.login(email, password)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

func expectStatus(t *testing.T, res *http.Response, body string, want int, wantText string) {
	t.Helper()
	if res.StatusCode != want || !strings.Contains(body, wantText) {
		t.Fatalf("%s %s = %d, want %d containing %q; body:\n%s", res.Request.Method, res.Request.URL.Path, res.StatusCode, want, wantText, body)
	}
}

func TestRegisterVerifyLogInAndOut(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"

	res, body := s.register("Ada Lovelace", email, password)
	expectStatus(t, res, body, http.StatusOK, "Check your email")

	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM learners WHERE email = $1`, email).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == password || !strings.HasPrefix(hash, "$2") {
		t.Fatalf("password stored as %q, want a bcrypt hash", hash)
	}

	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusForbidden, "Verify your email first")
	if len(res.Cookies()) != 0 {
		t.Fatalf("unverified login set cookies: %v", res.Cookies())
	}

	link := s.lastVerifyPath(email)
	res, body = s.get(link)
	expectStatus(t, res, body, http.StatusOK, "Email verified")
	res, body = s.get(link)
	expectStatus(t, res, body, http.StatusBadRequest, "already been used")

	res, body = s.login(email, "wrong password")
	expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")

	res, body = s.login("ADA@example.com", password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies, want 1", len(cookies))
	}
	session := cookies[0]
	if !session.Secure || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %+v, want Secure, HttpOnly, SameSite=Lax", session)
	}
	if session.MaxAge < 7*24*60*60 || session.MaxAge > 90*24*60*60 {
		t.Fatalf("session cookie Max-Age = %ds, want between a week and 90 days", session.MaxAge)
	}

	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "Logged in as Ada Lovelace")

	res, body = s.post("/logout", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/")
	if strings.Contains(body, "Logged in") {
		t.Fatalf("still logged in after logout: %s", body)
	}

	// The old cookie is dead server-side, not just deleted from the browser.
	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: session.Name, Value: session.Value})
	res, body = s.do(req)
	if strings.Contains(body, "Logged in") {
		t.Fatalf("old session cookie still logs in after logout: %s", body)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	s := newSite(t)
	res, body := s.register("Ada", "ada@example.com", "password one")
	expectStatus(t, res, body, http.StatusOK, "Check your email")

	res, body = s.register("Someone Else", "Ada@Example.com", "password two")
	expectStatus(t, res, body, http.StatusConflict, "This email is already registered")
	if n := len(s.mail.Messages()); n != 1 {
		t.Fatalf("sent %d emails, want 1", n)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	s := newSite(t)
	for _, tc := range []struct{ name, email, password, want string }{
		{"", "a@example.com", "long enough", "Enter your name"},
		{"Ada", "not an email", "long enough", "Enter a valid email"},
		{"Ada", "a@example.com", "short", "at least 8 characters"},
	} {
		res, body := s.register(tc.name, tc.email, tc.password)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, tc.want)
	}
	if n := len(s.mail.Messages()); n != 0 {
		t.Fatalf("sent %d emails for invalid registrations, want 0", n)
	}
}

func TestExpiredVerificationLinkOffersANewOne(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.register("Ada", email, password)
	expiredLink := s.lastVerifyPath(email)
	if _, err := s.db.ExecContext(context.Background(), `UPDATE one_time_tokens SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}

	res, body := s.get(expiredLink)
	expectStatus(t, res, body, http.StatusGone, "This verification link has expired")
	if !strings.Contains(body, `action="/verify/resend"`) || !strings.Contains(body, `value="ada@example.com"`) {
		t.Fatalf("expired page does not offer a new link: %s", body)
	}

	res, body = s.post("/verify/resend", url.Values{"email": {email}})
	expectStatus(t, res, body, http.StatusOK, "we sent it a new link")
	newLink := s.lastVerifyPath(email)
	if newLink == expiredLink {
		t.Fatal("resend reused the expired link")
	}
	res, body = s.get(newLink)
	expectStatus(t, res, body, http.StatusOK, "Email verified")
	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestFormsRefuseCrossOriginPosts(t *testing.T) {
	s := newSite(t)
	form := url.Values{"name": {"Mallory"}, "email": {"m@example.com"}, "password": {"password123"}}
	for _, path := range []string{"/register", "/login", "/logout", "/verify/resend"} {
		for header, value := range map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"} {
			req, _ := http.NewRequest(http.MethodPost, s.server.URL+path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set(header, value)
			res, body := s.do(req)
			expectStatus(t, res, body, http.StatusForbidden, "cross-origin request refused")
		}
	}
	if n := len(s.mail.Messages()); n != 0 {
		t.Fatalf("sent %d emails for cross-origin posts, want 0", n)
	}

	// The site's own pages still post fine.
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, body := s.do(req)
	expectStatus(t, res, body, http.StatusOK, "Check your email")
}
