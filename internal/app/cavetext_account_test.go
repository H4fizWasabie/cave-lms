package app_test

import (
	"net/url"
	"strings"
	"testing"
)

// The CaveText website calls the account "your CaveText account" (GLOSSARY.md, CaveText account). It names CAVELMS only
// in the line for people who got CaveText through CAVELMS, and in the Pro Plan's note that the Lab is used on CAVELMS.
var cavelmsAllowed = strings.NewReplacer(
	"Have a CAVELMS account? Log in with it.", "",
	"Ada akaun CAVELMS? Log masuk dengannya.", "",
	"The Lab is used on CAVELMS, with the same account.", "",
	"Lab digunakan di CAVELMS dengan akaun yang sama.", "",
)

func expectNoCAVELMS(t *testing.T, what, got string) {
	t.Helper()
	if strings.Contains(cavelmsAllowed.Replace(got), "CAVELMS") {
		t.Errorf("%s names CAVELMS:\n%s", what, got)
	}
}

func TestTheCaveTextHostNamesCAVELMSOnlyWhereThePersonNeedsToKnowIt(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	s.savePrices(map[string]string{"cavetext-pro_link": "https://bcl.my/pay/cavetext", "pro-plan_link": "https://bcl.my/pay/pro-plan"})
	ada := caveTextPricingLearner(s) // verified, logged in on the CaveText host, connected

	for _, prefix := range []string{"", "/ms"} {
		page := func(who *site, path string) string {
			t.Helper()
			res, body := who.get(caveTextOrigin + prefix + path)
			expectStatus(t, res, body, 200, "")
			expectNoCAVELMS(t, "GET "+prefix+path, body)
			return body
		}
		email := func(what, to string) {
			t.Helper()
			m := s.lastEmailTo(to)
			expectNoCAVELMS(t, what+" email subject", m.Subject)
			expectNoCAVELMS(t, what+" email body", m.Body)
		}

		for _, p := range []string{"/login", "/register", "/forgot-password", "/pricing"} {
			page(s, p)
		}
		page(ada, "/pricing")
		page(ada, "/cavetext/connect?extension="+extensionID)

		// Registering sends the verification email, whose link opens the confirm page, and another email if still unconfirmed.
		tag := strings.TrimPrefix(prefix, "/") // keeps the two rounds' usernames and emails apart
		bea := "bea" + tag + "@example.com"
		register := func(username, to string) string {
			_, body := s.post(caveTextOrigin+prefix+"/register", url.Values{"name": {"Bea"}, "username": {username}, "email": {to}, "password": {adaPassword}})
			return body
		}
		expectNoCAVELMS(t, "register result", register("bea"+tag, bea))
		email("verification", bea)
		page(s, s.caveTextLinkPath(bea)[len(prefix):])
		body := register("bea-again"+tag, bea)
		expectNoCAVELMS(t, "register again result", body)
		email("unconfirmed again", bea)

		// Registering with Ada's email tells her; forgetting the password sends the reset link, whose page opens.
		expectNoCAVELMS(t, "register with a used email result", register("ada-again"+tag, adaEmail))
		email("already registered", adaEmail)
		_, body = s.post(caveTextOrigin+prefix+"/forgot-password", url.Values{"email": {adaEmail}})
		expectNoCAVELMS(t, "forgot result", body)
		email("reset", adaEmail)
		page(s, s.resetLinkPath()[len(prefix):])
	}
}

func TestTheCaveTextLoginAndRegisterPagesTellACAVELMSAccountHolderToLogInWithIt(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	for prefix, line := range map[string]string{"": "Have a CAVELMS account? Log in with it.", "/ms": "Ada akaun CAVELMS? Log masuk dengannya."} {
		for _, p := range []string{"/login", "/register"} {
			res, body := s.get(caveTextOrigin + prefix + p)
			expectStatus(t, res, body, 200, line)
		}
		for _, p := range []string{"/forgot-password", "/pricing"} {
			res, body := s.get(caveTextOrigin + prefix + p)
			expectStatus(t, res, body, 200, "")
			if strings.Contains(body, line) {
				t.Errorf("GET %s%s has the CAVELMS account line", prefix, p)
			}
		}
	}
	// CAVELMS's own pages do not have it.
	res, body := s.get("/login")
	expectStatus(t, res, body, 200, "")
	if strings.Contains(body, "Log in with it.") {
		t.Errorf("CAVELMS login page has the CaveText line:\n%s", body)
	}
}
