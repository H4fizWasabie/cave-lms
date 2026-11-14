package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// The CaveText website's host (ADR 0028): its login and connect pages are served by the app, in CaveText's look.
const caveTextOrigin = "https://cavetext.cave.example.invalid"

const (
	adaEmail    = "ada@example.com"
	adaPassword = "correct horse battery"
)

// registerAda makes a verified Learner on the main host, logged in nowhere.
func (s *site) registerAda() {
	s.t.Helper()
	s.register("Ada Lovelace", adaEmail, adaPassword)
	s.verifyEmail(s.lastVerifyPath(adaEmail))
}

// loginOnCaveText logs in on the CaveText host, to next, and returns the answer.
func (s *site) loginOnCaveText(path, next string) (*http.Response, string) {
	return s.post(caveTextOrigin+path, url.Values{"email": {adaEmail}, "password": {adaPassword}, "next": {next}})
}

// onMain is whether the main host knows the client as a Learner; onCaveText is the same for the CaveText host.
func (s *site) onMain() bool {
	s.t.Helper()
	res, _ := s.get("/my-profile")
	return res.StatusCode == http.StatusOK
}

func (s *site) onCaveText() bool {
	s.t.Helper()
	res, _ := s.get(caveTextOrigin + "/cavetext/connect?extension=" + extensionID)
	return res.StatusCode == http.StatusOK
}

func TestTheCaveTextHostShowsLoginInCaveTextsLookInEnglishAndMalay(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)

	res, body := s.get(caveTextOrigin + "/login")
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/login">`)
	for _, want := range []string{`href="/style.css"`, `href="/guide.html"`, `href="/ms/login"`, "CaveText", "Log in"} {
		if !strings.Contains(body, want) {
			t.Errorf("English login page lacks %q:\n%s", want, body)
		}
	}
	for _, not := range []string{"/courses", "cavelms.css", "/forgot-password"} {
		if strings.Contains(body, not) {
			t.Errorf("CaveText login page shows %q, which is CAVELMS's or not built here yet:\n%s", not, body)
		}
	}

	// The page's language is the path's, whatever the browser asks for.
	req := s.newRequest(http.MethodGet, caveTextOrigin+"/ms/login", nil)
	req.Header.Set("Accept-Language", "en")
	res, body = s.do(req)
	expectStatus(t, res, body, http.StatusOK, `action="/ms/login"`)
	for _, want := range []string{"Log masuk", `href="/ms/guide.html"`, `href="/login"`, `lang="ms"`} {
		if !strings.Contains(body, want) {
			t.Errorf("Malay login page lacks %q:\n%s", want, body)
		}
	}
}

func TestLoggingInOnTheCaveTextHostKeepsTheLearnerLoggedInThereOnlyAndTheReverse(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()

	res, body := s.loginOnCaveText("/login", "")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/" {
		t.Errorf("login redirected to %q, want /", got)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set cookies %v, want just the session", cookies)
	}
	c := cookies[0]
	if c.Name == app.SessionCookie || !strings.HasPrefix(c.Name, "__Host-") || c.Domain != "" || c.Path != "/" || !c.Secure || !c.HttpOnly {
		t.Errorf("CaveText session cookie = %+v, want its own host-only __Host- cookie, not CAVELMS's %s", c, app.SessionCookie)
	}
	if !s.onCaveText() || s.onMain() {
		t.Fatalf("after logging in on the CaveText host: logged in there %v, on CAVELMS %v; want only there", s.onCaveText(), s.onMain())
	}

	// The reverse: logging in to CAVELMS does not log in the CaveText host's other client, and logging out of one leaves the other.
	s.login(adaEmail, adaPassword)
	if !s.onMain() || !s.onCaveText() {
		t.Fatalf("after logging in on both: CAVELMS %v, CaveText %v", s.onMain(), s.onCaveText())
	}
	other := s.second()
	other.login(adaEmail, adaPassword)
	if other.onCaveText() {
		t.Error("a CAVELMS login counts as logged in on the CaveText host")
	}
	res, body = s.post("/logout", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if s.onMain() || !s.onCaveText() {
		t.Errorf("after logging out of CAVELMS: CAVELMS %v, CaveText %v; want the CaveText login kept", s.onMain(), s.onCaveText())
	}
}

func TestLoggingOutOfTheCaveTextHostLeavesCAVELMSAndTheExtensionSignedIn(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	s.login(adaEmail, adaPassword)
	token := s.connect()
	s.loginOnCaveText("/login", "")

	res, body := s.post(caveTextOrigin+"/logout", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/" {
		t.Errorf("logout redirected to %q, want /", got)
	}
	if cookies := res.Cookies(); len(cookies) != 1 || cookies[0].MaxAge >= 0 || cookies[0].Name == app.SessionCookie {
		t.Errorf("logout set cookies %v, want the CaveText session cleared and nothing else", cookies)
	}
	if s.onCaveText() || !s.onMain() {
		t.Errorf("after logging out of the CaveText host: there %v, CAVELMS %v; want only CAVELMS", s.onCaveText(), s.onMain())
	}
	if me := s.me(token); me.Name != "Ada Lovelace" {
		t.Errorf("the extension's token after logging out of the CaveText host: me = %+v", me)
	}

	// Malay: the way back is the Malay home page.
	s.loginOnCaveText("/ms/login", "")
	res, body = s.post(caveTextOrigin+"/ms/logout", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/ms/" {
		t.Errorf("Malay logout redirected to %q, want /ms/", got)
	}
}

func TestTheCaveTextConnectPageSendsAVisitorToLogInAndBack(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	for prefix, login := range map[string]string{"": "Log in", "/ms": "Log masuk"} {
		connect := prefix + "/cavetext/connect?extension=" + extensionID
		res, body := s.get(caveTextOrigin + connect)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		want := prefix + "/login?next=" + url.QueryEscape(connect)
		if got := res.Header.Get("Location"); got != want {
			t.Fatalf("Visitor sent to %q, want %q", got, want)
		}
		res, body = s.get(caveTextOrigin + want)
		expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connect+`"`)
		expectStatus(t, res, body, http.StatusOK, login)
		// Chrome checks the redirect to the extension a login sends against the login page's form-action.
		if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' https://"+extensionID+".chromiumapp.org") {
			t.Errorf("login page for connect has form-action in %q, want the extension allowed", csp)
		}
	}
}

func TestAFreshLoginOnTheCaveTextHostFromConnectSendsTheTokenAtOnce(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	for _, prefix := range []string{"", "/ms"} {
		connect := prefix + "/cavetext/connect?extension=" + extensionID
		res, body := s.loginOnCaveText(prefix+"/login", connect)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		const to = "https://" + extensionID + ".chromiumapp.org/#token="
		loc := res.Header.Get("Location")
		if !strings.HasPrefix(loc, to) || len(loc) == len(to) {
			t.Fatalf("login from connect redirected to %q, want %s<token>", loc, to)
		}
		if me := s.me(strings.TrimPrefix(loc, to)); me.Name != "Ada Lovelace" {
			t.Errorf("me = %+v", me)
		}
		if !s.onCaveText() {
			t.Error("the login from connect did not log in on the CaveText host")
		}
		s.post(caveTextOrigin+prefix+"/logout", nil)
	}
	if s.onMain() {
		t.Error("a login on the CaveText host logged in on CAVELMS")
	}
}

func TestALoggedInLearnerOnTheCaveTextHostConnectsWithAButtonAndMayUseAnotherAccount(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	s.loginOnCaveText("/login", "")
	connect := "/cavetext/connect?extension=" + extensionID

	res, body := s.get(caveTextOrigin + connect)
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/cavetext/connect">`)
	for _, want := range []string{"Ada Lovelace", ">Connect</button>", `href="/login?next=` + url.QueryEscape(connect) + `"`, "Use another account"} {
		if !strings.Contains(body, want) {
			t.Errorf("Connect page lacks %q:\n%s", want, body)
		}
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' https://"+extensionID+".chromiumapp.org") {
		t.Errorf("Connect page CSP = %q, want the extension allowed in form-action", csp)
	}
	if n := s.caveTextTokenCount(); n != 0 {
		t.Fatalf("opening the Connect page made %d tokens, want 0: only the button's POST connects", n)
	}
	// The link opens the login form even while logged in, and a login there is a new account's.
	res, body = s.get(caveTextOrigin + "/login?next=" + url.QueryEscape(connect))
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/login">`)

	res, body = s.post(caveTextOrigin+"/cavetext/connect", url.Values{"extension": {extensionID}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "https://"+extensionID+".chromiumapp.org/#token=") {
		t.Errorf("Connect redirected to %q", loc)
	}

	// Malay: the button and the link stay on /ms/.
	res, body = s.get(caveTextOrigin + "/ms" + connect)
	expectStatus(t, res, body, http.StatusOK, `action="/ms/cavetext/connect"`)
	expectStatus(t, res, body, http.StatusOK, `href="/ms/login?next=`+url.QueryEscape("/ms"+connect)+`"`)

	// CAVELMS's own Connect page is as it was: no link to another account.
	main := s.second()
	main.login(adaEmail, adaPassword)
	res, body = main.get(connect)
	expectStatus(t, res, body, http.StatusOK, `action="/cavetext/connect"`)
	if strings.Contains(body, "Use another account") {
		t.Errorf("CAVELMS Connect page links to log in:\n%s", body)
	}
}

func TestTheCaveTextHostSendsATokenToListedExtensionsOnly(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	const other = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	connect := "/cavetext/connect?extension=" + other

	res, body := s.get(caveTextOrigin + connect)
	expectStatus(t, res, body, http.StatusBadRequest, "Unknown extension")
	// A login whose next names it is an ordinary login: no token, and the login page does not let the form reach it.
	res, body = s.get(caveTextOrigin + "/login?next=" + url.QueryEscape(connect))
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(res.Header.Get("Content-Security-Policy"), other) {
		t.Errorf("login page CSP allows the unlisted extension: %s", res.Header.Get("Content-Security-Policy"))
	}
	res, body = s.loginOnCaveText("/login", connect)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != connect {
		t.Errorf("login with an unlisted extension redirected to %q, want %q", got, connect)
	}
	res, body = s.post(caveTextOrigin+"/cavetext/connect", url.Values{"extension": {other}})
	expectStatus(t, res, body, http.StatusBadRequest, "Unknown extension")
	if n := s.caveTextTokenCount(); n != 0 {
		t.Errorf("%d tokens exist, want none for an unlisted extension", n)
	}
}

func TestLoginOnTheCaveTextHostKeepsTheLimitsAndTheNextGuardAndAnswersInTheHostsLook(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()

	for _, next := range []string{"https://evil.example/", "//evil.example/", `/\evil.example/`, "/\t/evil.example/"} {
		res, body := s.loginOnCaveText("/login", next)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		if got := res.Header.Get("Location"); got != "/" {
			t.Errorf("next=%q redirected to %q, want /", next, got)
		}
	}
	s.post(caveTextOrigin+"/logout", nil)

	bad := url.Values{"email": {adaEmail}, "password": {"not the password"}}
	res, body := s.post(caveTextOrigin+"/ms/login", bad)
	expectStatus(t, res, body, http.StatusUnauthorized, "E-mel atau kata laluan salah.")
	expectStatus(t, res, body, http.StatusUnauthorized, `href="/style.css"`)
	for i := 0; i < 10; i++ {
		if res, _ := s.post(caveTextOrigin+"/login", bad); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong password %d = %d, want 401", i+1, res.StatusCode)
		}
	}
	res, body = s.post(caveTextOrigin+"/login", bad)
	expectStatus(t, res, body, http.StatusTooManyRequests, `href="/style.css"`)
	if res, _ := s.loginOnCaveText("/login", ""); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("a right password after the limit = %d, want 429", res.StatusCode)
	}
}

func TestEveryOtherCAVELMSPathIsNotFoundOnTheCaveTextHost(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	s.loginOnCaveText("/login", "")
	s.login(adaEmail, adaPassword)

	for _, p := range []string{"/", "/forgot-password", "/reset-password", "/courses", "/admin", "/lab", "/gallery", "/privacy",
		"/pricing", "/cavetext", "/cavetext/api/me", "/my-profile", "/static/cavelms.css", "/language", "/d/x/y/", "/ms/", "/ms/forgot-password", "/ms/pricing"} {
		res, body := s.get(caveTextOrigin + p)
		expectStatus(t, res, body, http.StatusNotFound, `href="/style.css"`)
		if strings.Contains(body, "site-header") {
			t.Errorf("GET %s on the CaveText host shows CAVELMS's look: %s", p, body)
		}
	}
	for _, p := range []string{"/forgot-password", "/language", "/cavetext/api/sign-out", "/cavetext/bcl/x"} {
		res, body := s.post(caveTextOrigin+p, url.Values{"lang": {"ms"}})
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
	// A trailing dot, the default port or capitals are still the CaveText host.
	for _, host := range []string{"https://cavetext.cave.example.invalid.", "https://cavetext.cave.example.invalid:443", "https://CAVETEXT.cave.example.invalid"} {
		res, body := s.get(host + "/forgot-password")
		expectStatus(t, res, body, http.StatusNotFound, `href="/style.css"`)
	}
	// And the main host does not serve the CaveText look.
	res, body := s.get("/login")
	expectStatus(t, res, body, http.StatusOK, "cavelms.css")
}
