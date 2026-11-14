package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Forgetting and resetting a password on the CaveText host (ADR 0028): CAVELMS's rules, in CaveText's look, emails and links.

// resetLinkPath is the path and query of the reset link to the CaveText host in the latest email to adaEmail.
func (s *site) resetLinkPath() string {
	s.t.Helper()
	link := s.caveTextLinkPath(adaEmail)
	if !regexp.MustCompile(`^(/ms)?/reset-password\?token=`).MatchString(link) {
		s.t.Fatalf("link %q is not a reset link", link)
	}
	return link
}

func TestTheCaveTextHostResetsAPasswordWithCaveTextEmailsInThePagesLanguage(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()

	// The login page offers it.
	res, body := s.get(caveTextOrigin + "/login")
	expectStatus(t, res, body, http.StatusOK, `href="/forgot-password"`)
	res, body = s.get(caveTextOrigin + "/ms/login")
	expectStatus(t, res, body, http.StatusOK, `href="/ms/forgot-password"`)

	res, body = s.get(caveTextOrigin + "/forgot-password")
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/forgot-password">`)
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	expectStatus(t, res, body, http.StatusOK, `href="/login"`)
	if strings.Contains(body, "CAVELMS") || strings.Contains(body, "cavelms.css") {
		t.Errorf("CaveText forgot page is CAVELMS's:\n%s", body)
	}

	// English: a CaveText email linking to the CaveText host.
	res, body = s.post(caveTextOrigin+"/forgot-password", url.Values{"email": {adaEmail}})
	expectStatus(t, res, body, http.StatusOK, "If an account exists for "+adaEmail)
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	m := s.lastEmailTo(adaEmail)
	expectEmail(t, m, "Reset your CaveText password", "https://cavetext.cave.example.invalid/reset-password?token=")
	if strings.Contains(m.Subject+m.Body, "CAVELMS") {
		t.Errorf("CaveText reset email names CAVELMS: %q %q", m.Subject, m.Body)
	}

	link := s.resetLinkPath()
	res, body = s.get(caveTextOrigin + link)
	expectStatus(t, res, body, http.StatusOK, `<form class="form" method="post" action="/reset-password">`)
	expectStatus(t, res, body, http.StatusOK, `href="/style.css"`)
	if strings.Contains(body, "CAVELMS") {
		t.Errorf("CaveText reset page names CAVELMS:\n%s", body)
	}
	token := resetToken(t, link)
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"short"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `action="/reset-password"`)
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"new horse staple"}})
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")
	expectStatus(t, res, body, http.StatusOK, `href="/login"`)
	if s.onCaveText() || s.onMain() {
		t.Error("resetting logged the Learner in")
	}
	// The link is single-use, and its dead end offers a new one on the CaveText host.
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"another horse staple"}})
	expectStatus(t, res, body, http.StatusBadRequest, `href="/forgot-password"`)
	if res, body := s.post(caveTextOrigin+"/login", url.Values{"email": {adaEmail}, "password": {"new horse staple"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login with the new password = %d: %s", res.StatusCode, body)
	}
	if s.onMain() {
		t.Error("a login on the CaveText host logged in on CAVELMS")
	}

	// Malay: the page, the email and its link are all Malay and under /ms/, whatever the Learner saved.
	res, body = s.get(caveTextOrigin + "/ms/forgot-password")
	expectStatus(t, res, body, http.StatusOK, `action="/ms/forgot-password"`)
	res, body = s.post(caveTextOrigin+"/ms/forgot-password", url.Values{"email": {adaEmail}})
	expectStatus(t, res, body, http.StatusOK, "Jika "+adaEmail+" mempunyai akaun")
	expectEmail(t, s.lastEmailTo(adaEmail), "Set semula kata laluan CaveText anda", "https://cavetext.cave.example.invalid/ms/reset-password?token=")
	link = s.resetLinkPath()
	res, body = s.get(caveTextOrigin + link)
	expectStatus(t, res, body, http.StatusOK, `action="/ms/reset-password"`)
	res, body = s.post(caveTextOrigin+"/ms/reset-password", url.Values{"token": {resetToken(t, link)}, "password": {"yet another horse"}})
	expectStatus(t, res, body, http.StatusOK, "Kata laluan anda telah ditukar")
	expectStatus(t, res, body, http.StatusOK, `href="/ms/login"`)
	res, body = s.post(caveTextOrigin+"/ms/reset-password", url.Values{"token": {"nope"}, "password": {"yet another horse"}})
	expectStatus(t, res, body, http.StatusBadRequest, `href="/ms/forgot-password"`)
}

func TestTheCaveTextHostGivesTheSameAnswerForAnyEmail(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	before := len(s.mail.Messages())
	res, body := s.post(caveTextOrigin+"/forgot-password", url.Values{"email": {"nobody@example.com"}})
	expectStatus(t, res, body, http.StatusOK, "If an account exists for nobody@example.com")
	if len(s.mail.Messages()) != before {
		t.Error("an email with no account was sent a reset")
	}
}

func TestResettingOnCAVELMSStillSendsCAVELMSsEmail(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	connect := "/cavetext/connect?extension=" + extensionID
	// Its pages never carry a next, even one for the connect page.
	for _, path := range []string{"/forgot-password?next=", "/reset-password?token=x&next="} {
		_, body := s.get(path + url.QueryEscape(connect))
		if strings.Contains(body, `name="next" value="/cavetext`) {
			t.Errorf("CAVELMS %s carries next:\n%s", path, body)
		}
	}
	s.forgotPassword(adaEmail)
	expectEmail(t, s.lastEmailTo(adaEmail), "Reset your CAVELMS password", "https://cave.example.invalid/reset-password?token=")
}

func TestAResetFromConnectEndsSignedInToTheExtension(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	s.registerAda()
	connect := "/cavetext/connect?extension=" + extensionID
	next := "next=" + url.QueryEscape(connect)

	// The login page offers the reset, carrying the connect page on.
	res, body := s.get(caveTextOrigin + "/login?" + next)
	expectStatus(t, res, body, http.StatusOK, `href="/forgot-password?next=`+htmlQuery(connect)+`"`)
	res, body = s.get(caveTextOrigin + "/forgot-password?" + next)
	expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connect+`"`)
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=`+htmlQuery(connect)+`"`)

	s.post(caveTextOrigin+"/forgot-password", url.Values{"email": {adaEmail}, "next": {connect}})
	link := s.resetLinkPath()
	if !strings.Contains(link, "next="+url.QueryEscape(connect)) {
		t.Fatalf("reset link %q does not carry next", link)
	}
	res, body = s.get(caveTextOrigin + link)
	expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connect+`"`)

	// A refused password keeps it.
	token := resetToken(t, link)
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"short"}, "next": {connect}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, `name="next" value="`+connect+`"`)
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"new horse staple"}, "next": {connect}})
	expectStatus(t, res, body, http.StatusOK, "Your password has been changed")
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=`+htmlQuery(connect)+`"`)

	// A dead link offers a new reset, which carries it too.
	res, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {token}, "password": {"new horse staple"}, "next": {connect}})
	expectStatus(t, res, body, http.StatusBadRequest, `href="/forgot-password?next=`+htmlQuery(connect)+`"`)

	// That login is the consent: the token goes straight to the extension.
	res, body = s.post(caveTextOrigin+"/login", url.Values{"email": {adaEmail}, "password": {"new horse staple"}, "next": {connect}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "https://"+extensionID+".chromiumapp.org/#token=") {
		t.Errorf("login redirected to %q, want the extension", loc)
	}
	if s.onMain() {
		t.Error("the Learner was logged in on CAVELMS")
	}
}

func TestResettingOnTheCaveTextHostCarriesOnlyAListedConnectNext(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	for i, next := range []string{
		"https://evil.example/", "//evil.example/", `/\evil.example/`, "/\t/evil.example/",
		"/cavetext/connect?extension=unlisted", "/my-profile",
	} {
		email := fmt.Sprintf("n%d@example.com", i) // a reset email is sent to one address only three times an hour
		s.register("N", email, adaPassword)
		s.verifyEmail(s.lastVerifyPath(email))
		s.post(caveTextOrigin+"/forgot-password", url.Values{"email": {email}, "next": {next}})
		if link := s.caveTextLinkPath(email); strings.Contains(link, "next=") {
			t.Errorf("next=%q went into the link %q", next, link)
		}
		_, body := s.get(caveTextOrigin + "/forgot-password?next=" + url.QueryEscape(next))
		if strings.Contains(body, `name="next" value="/`) {
			t.Errorf("next=%q went into the forgot form:\n%s", next, body)
		}
		_, body = s.post(caveTextOrigin+"/reset-password", url.Values{"token": {"x"}, "password": {"new horse staple"}, "next": {next}})
		if strings.Contains(body, "next=") {
			t.Errorf("dead link page carries next=%q:\n%s", next, body)
		}
	}
	// The Malay connect page is a listed next as well.
	s.registerAda()
	s.post(caveTextOrigin+"/ms/forgot-password", url.Values{"email": {adaEmail}, "next": {"/ms/cavetext/connect?extension=" + extensionID}})
	if link := s.resetLinkPath(); !strings.HasPrefix(link, "/ms/reset-password?token=") || !strings.Contains(link, "next=") {
		t.Errorf("Malay link = %q", link)
	}
}

func TestForgotAndResetOnTheCaveTextHostAreRateLimitedLikeCAVELMSs(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/forgot-password", "/reset-password"} {
		s := newCaveTextSite(t)
		form := url.Values{"email": {adaEmail}, "token": {"not-a-token"}, "password": {"correct horse battery"}}
		for i := range 100 {
			// Half of them under /ms: the Malay pages are the same budget, not a second one.
			prefix := ""
			if i%2 == 1 {
				prefix = "/ms"
			}
			res, body := s.post(caveTextOrigin+prefix+path, form)
			if res.StatusCode == http.StatusTooManyRequests {
				t.Fatalf("%s attempt %d refused early: %s", path, i+1, body)
			}
		}
		res, body := s.post(caveTextOrigin+path, form)
		expectStatus(t, res, body, http.StatusTooManyRequests, `href="/style.css"`)
		res, body = s.post(caveTextOrigin+"/ms"+path, form)
		expectStatus(t, res, body, http.StatusTooManyRequests, "Terlalu banyak cubaan")
	}
}
