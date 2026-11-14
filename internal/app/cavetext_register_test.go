package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Registering and verifying on the CaveText host (ADR 0028): CAVELMS's rules, in CaveText's look, emails and links.

// registerOnCaveText posts the register form of the CaveText host under prefix ("" or "/ms") and returns the answer.
func (s *site) registerOnCaveText(prefix, name, email, next string) (*http.Response, string) {
	return s.post(caveTextOrigin+prefix+"/register", url.Values{
		"name": {name}, "username": {usernameFor(email)}, "email": {email}, "password": {adaPassword}, "next": {next}})
}

// htmlQuery is url.QueryEscape as the page templates write it, with lowercase hex.
func htmlQuery(s string) string {
	return upperHex.ReplaceAllStringFunc(url.QueryEscape(s), strings.ToLower)
}

var upperHex = regexp.MustCompile(`%[0-9A-F]{2}`)

// caveTextLinkPath is the path and query of the link to the CaveText host in the latest email to `to`; it fails if the
// email links anywhere else.
func (s *site) caveTextLinkPath(to string) string {
	s.t.Helper()
	body := s.lastEmailTo(to).Body
	if strings.Contains(body, "https://cave.example.invalid") {
		s.t.Errorf("email to %s links to CAVELMS: %s", to, body)
	}
	m := regexp.MustCompile(`https://cavetext\.cave\.example\.invalid(/\S+)`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("email to %s has no CaveText link: %s", to, body)
	}
	return m[1]
}

func TestTheCaveTextHostRegistersALearnerWithCaveTextEmailsInThePagesLanguage(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)

	res, body := s.get(caveTextOrigin + "/register")
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/register">`)
	for _, want := range []string{`href="/style.css"`, `href="/guide.html"`, `href="/ms/register"`, `href="/login"`, `href="/privacy.html"`, "CaveText"} {
		if !strings.Contains(body, want) {
			t.Errorf("CaveText register page lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "cavelms.css") || strings.Contains(body, `href="/privacy"`) {
		t.Errorf("CaveText register page is CAVELMS's:\n%s", body)
	}

	// English: a CaveText email linking to the CaveText host.
	res, body = s.registerOnCaveText("", "Ada Lovelace", adaEmail, "")
	expectStatus(t, res, body, http.StatusOK, "We sent a verification link to "+adaEmail)
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	m := s.lastEmailTo(adaEmail)
	expectEmail(t, m, "Confirm your CaveText account", "https://cavetext.cave.example.invalid/verify?token=")
	if strings.Contains(m.Subject+m.Body, "CAVELMS") {
		t.Errorf("CaveText verification email names CAVELMS: %q %q", m.Subject, m.Body)
	}

	// The link opens the confirm page, which changes nothing, and the button verifies: on the CaveText host only.
	link := s.caveTextLinkPath(adaEmail)
	res, body = s.get(caveTextOrigin + link)
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/verify">`)
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	if strings.Contains(body, "CAVELMS") {
		t.Errorf("CaveText confirm page names CAVELMS:\n%s", body)
	}
	if res, body := s.login(adaEmail, adaPassword); res.StatusCode != http.StatusForbidden {
		t.Fatalf("login before the button = %d, want 403 unverified: %s", res.StatusCode, body)
	}
	token := verifyToken.FindStringSubmatch(body)[1]
	res, body = s.post(caveTextOrigin+"/verify", url.Values{"token": {token}})
	expectStatus(t, res, body, http.StatusOK, "Your account is confirmed.")
	expectStatus(t, res, body, http.StatusOK, `href="/login"`)
	if s.onCaveText() || s.onMain() {
		t.Error("verifying logged the Learner in")
	}
	if res, body := s.loginOnCaveText("/login", ""); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login after verifying = %d: %s", res.StatusCode, body)
	}
	if s.onMain() {
		t.Error("a login on the CaveText host logged in on CAVELMS")
	}

	// Malay: the page, the email and its link are all Malay and under /ms/.
	res, body = s.registerOnCaveText("/ms", "Bea", "bea@example.com", "")
	expectStatus(t, res, body, http.StatusOK, "Kami telah menghantar pautan pengesahan")
	expectEmail(t, s.lastEmailTo("bea@example.com"), "Sahkan akaun CaveText anda", "https://cavetext.cave.example.invalid/ms/verify?token=")
	res, body = s.get(caveTextOrigin + s.caveTextLinkPath("bea@example.com"))
	expectStatus(t, res, body, http.StatusOK, `action="/ms/verify"`)
	res, body = s.post(caveTextOrigin+"/ms/verify", url.Values{"token": {verifyToken.FindStringSubmatch(body)[1]}})
	expectStatus(t, res, body, http.StatusOK, "Akaun anda telah disahkan.")
	expectStatus(t, res, body, http.StatusOK, `href="/ms/login"`)
}

func TestRegisteringOnCAVELMSStillSendsCAVELMSEmails(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	// Its pages never carry a next, even one for the connect page.
	connect := "/cavetext/connect?extension=" + extensionID
	next := "next=" + url.QueryEscape(connect)
	for _, path := range []string{"/register?" + next, "/login?" + next, "/verify?token=x&" + next} {
		_, body := s.get(path)
		if strings.HasPrefix(path, "/login") && strings.Contains(body, `href="/register?next=`) || !strings.HasPrefix(path, "/login") && strings.Contains(body, `name="next" value="/cavetext`) {
			t.Errorf("CAVELMS %s carries next:\n%s", path, body)
		}
	}
	s.register("Ada Lovelace", adaEmail, adaPassword)
	expectEmail(t, s.lastEmailTo(adaEmail), "Confirm your CAVELMS account", "https://cave.example.invalid/verify?token=")
}

func TestRegisteringOnTheCaveTextHostKeepsCAVELMSsRules(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda() // verified, on CAVELMS

	// A taken Username is refused first, even for an email that has an account.
	for _, email := range []string{"new@example.com", adaEmail} {
		res, body := s.post(caveTextOrigin+"/register", url.Values{"name": {"Bea"}, "username": {usernameFor(adaEmail)}, "email": {email}, "password": {adaPassword}})
		expectStatus(t, res, body, http.StatusConflict, "That Username is already taken.")
		expectStatus(t, res, body, http.StatusConflict, `href="/style.css"`)
	}

	// An email with an account gets the same answer as a new one and is emailed instead, in CaveText's name and the page's language.
	before := len(s.emailsTo(adaEmail))
	res, body := s.post(caveTextOrigin+"/register", url.Values{"name": {"Bea"}, "username": {"bea"}, "email": {adaEmail}, "password": {adaPassword}})
	expectStatus(t, res, body, http.StatusOK, "We sent a verification link to "+adaEmail)
	expectEmail(t, s.lastEmailTo(adaEmail), "Someone tried to register with your CaveText email",
		"https://cavetext.cave.example.invalid/login")
	res, body = s.post(caveTextOrigin+"/ms/register", url.Values{"name": {"Bea"}, "username": {"bea2"}, "email": {adaEmail}, "password": {adaPassword}})
	expectStatus(t, res, body, http.StatusOK, "Kami telah menghantar pautan pengesahan")
	m := s.lastEmailTo(adaEmail)
	expectEmail(t, m, "Seseorang cuba mendaftar dengan e-mel CaveText anda", "https://cavetext.cave.example.invalid/ms/login")
	if got := len(s.emailsTo(adaEmail)) - before; got != 2 {
		t.Errorf("%d emails to the account, want 2", got)
	}
	if strings.Contains(m.Subject+m.Body, "CAVELMS") || strings.Contains(m.Body, "https://cave.example.invalid") {
		t.Errorf("CaveText notice names or links CAVELMS: %q %q", m.Subject, m.Body)
	}

	// An unverified account is emailed a link to set its password, on the CaveText host.
	s.registerOnCaveText("", "Cy", "cy@example.com", "")
	res, body = s.post(caveTextOrigin+"/register", url.Values{"name": {"Cy"}, "username": {"cy-again"}, "email": {"cy@example.com"}, "password": {adaPassword}})
	expectStatus(t, res, body, http.StatusOK, "We sent a verification link")
	expectEmail(t, s.lastEmailTo("cy@example.com"), "Set your CaveText password", "https://cavetext.cave.example.invalid/reset-password?token=")
}

func TestVerificationOnTheCaveTextHostKeepsTheConfirmButtonItsExpiryAndResendsInCaveTextsName(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)

	// A bad link offers a new one, from the CaveText host's resend form.
	res, body := s.post(caveTextOrigin+"/ms/verify", url.Values{"token": {"nope"}})
	expectStatus(t, res, body, http.StatusBadRequest, `action="/ms/verify/resend"`)

	s.registerOnCaveText("", "Ada Lovelace", adaEmail, "")
	res, body = s.post(caveTextOrigin+"/verify/resend", url.Values{"email": {adaEmail}})
	expectStatus(t, res, body, http.StatusOK, "we sent it a new link")
	expectEmail(t, s.lastEmailTo(adaEmail), "Confirm your CaveText account", "https://cavetext.cave.example.invalid/verify?token=")
	res, body = s.post(caveTextOrigin+"/ms/verify/resend", url.Values{"email": {adaEmail}})
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	expectEmail(t, s.lastEmailTo(adaEmail), "Sahkan akaun CaveText anda", "https://cavetext.cave.example.invalid/ms/verify?token=")

	// A login before verifying offers the same form.
	res, body = s.loginOnCaveText("/login", "")
	expectStatus(t, res, body, http.StatusForbidden, `action="/verify/resend"`)
}

func TestANewcomerFromConnectEndsSignedInToTheExtensionAfterVerifying(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	connect := "/cavetext/connect?extension=" + extensionID

	// The login page offers to register, carrying the connect page on.
	res, body := s.get(caveTextOrigin + "/login?next=" + url.QueryEscape(connect))
	expectStatus(t, res, body, http.StatusOK, `href="/register?next=`+htmlQuery(connect)+`"`)
	res, body = s.get(caveTextOrigin + "/ms/login")
	expectStatus(t, res, body, http.StatusOK, `href="/ms/register"`)

	res, body = s.get(caveTextOrigin + "/register?next=" + url.QueryEscape(connect))
	expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connect+`"`)
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=`+htmlQuery(connect)+`"`)

	// A refused form keeps it.
	res, body = s.post(caveTextOrigin+"/register", url.Values{"name": {"Ada"}, "username": {"ada"}, "email": {"bad"}, "password": {adaPassword}, "next": {connect}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `name="next" value="`+connect+`"`)

	res, body = s.registerOnCaveText("", "Ada Lovelace", adaEmail, connect)
	expectStatus(t, res, body, http.StatusOK, "We sent a verification link")
	link := s.caveTextLinkPath(adaEmail)
	if !strings.Contains(link, "next="+url.QueryEscape(connect)) {
		t.Fatalf("verification link %q does not carry next", link)
	}
	res, body = s.get(caveTextOrigin + link)
	expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connect+`"`)
	m := verifyToken.FindStringSubmatch(body)
	res, body = s.post(caveTextOrigin+"/verify", url.Values{"token": {m[1]}, "next": {connect}})
	expectStatus(t, res, body, http.StatusOK, "Your account is confirmed.")
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=`+htmlQuery(connect)+`"`)

	// That login is the consent: the token goes straight to the extension.
	res, body = s.loginOnCaveText("/login", connect)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "https://"+extensionID+".chromiumapp.org/#token=") {
		t.Errorf("login redirected to %q, want the extension", loc)
	}
	if s.onMain() {
		t.Error("the newcomer was logged in on CAVELMS")
	}
}

func TestRegisteringOnTheCaveTextHostCarriesOnlyAListedConnectNext(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	for i, next := range []string{
		"https://evil.example/", "//evil.example/", `/\evil.example/`, "/\t/evil.example/",
		"/cavetext/connect?extension=unlisted", "/my-profile", "/ms/cavetext/connect?extension=" + extensionID + "x",
	} {
		email := fmt.Sprintf("n%d@example.com", i)
		res, body := s.registerOnCaveText("", "N", email, next)
		expectStatus(t, res, body, http.StatusOK, "We sent a verification link")
		if link := s.caveTextLinkPath(email); strings.Contains(link, "next=") {
			t.Errorf("next=%q went into the link %q", next, link)
		}
		_, body = s.get(caveTextOrigin + "/register?next=" + url.QueryEscape(next))
		if strings.Contains(body, `name="next" value="/cavetext`) {
			t.Errorf("next=%q went into the register form:\n%s", next, body)
		}
		// A link someone edited to carry such a next does not carry it to the verified page.
		_, body = s.post(caveTextOrigin+"/verify", url.Values{"token": {"x"}, "next": {next}})
		if strings.Contains(body, "next=") {
			t.Errorf("verify page carries next=%q:\n%s", next, body)
		}
	}
	// The Malay connect page is a listed next as well.
	res, body := s.registerOnCaveText("/ms", "N", "ms@example.com", "/ms/cavetext/connect?extension="+extensionID)
	expectStatus(t, res, body, http.StatusOK, "Kami telah menghantar")
	if link := s.caveTextLinkPath("ms@example.com"); !strings.HasPrefix(link, "/ms/verify?token=") || !strings.Contains(link, "next=") {
		t.Errorf("Malay link = %q", link)
	}
}

func TestRegisterAndVerifyOnTheCaveTextHostAreRateLimitedLikeCAVELMSs(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	for i := range 100 {
		res, body := s.post(caveTextOrigin+"/register", url.Values{"email": {"bad"}})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("attempt %d = %d, want 422: %s", i+1, res.StatusCode, body)
		}
	}
	res, body := s.post(caveTextOrigin+"/register", url.Values{"email": {"bad"}})
	expectStatus(t, res, body, http.StatusTooManyRequests, `href="/style.css"`)
	// The Malay pages are the same budget, not a second one.
	res, body = s.post(caveTextOrigin+"/ms/register", url.Values{"email": {"bad"}})
	expectStatus(t, res, body, http.StatusTooManyRequests, "Terlalu banyak cubaan")
}
