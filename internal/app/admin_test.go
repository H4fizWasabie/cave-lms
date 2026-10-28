package app_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAdminAreaRefusesVisitors(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/admin")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestAdminAreaRefusesLearners(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := s.get("/admin")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
}

func TestAdminAreaLoadsForTheAdmin(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.get("/admin")
	expectStatus(t, res, body, http.StatusOK, "<h1>Admin area</h1>")
}

func TestHomeLinksToAdminAreaOnlyForTheAdmin(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	_, body := s.get("/")
	if strings.Contains(body, `href="/admin"`) {
		t.Fatalf("Learner home page links to the Admin area: %s", body)
	}
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	_, body = s.get("/")
	if !strings.Contains(body, `href="/admin"`) {
		t.Fatalf("Admin home page has no Admin link: %s", body)
	}
}

func TestRegistrationCannotMakeAnAdmin(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email, password = "mallory@example.com", "correct horse battery"
	res, body := s.post("/register", url.Values{"name": {"Mallory"}, "username": {usernameFor(email)}, "email": {email}, "password": {password}, "is_admin": {"true"}})
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	res, body = s.get(s.lastVerifyPath(email))
	expectStatus(t, res, body, http.StatusOK, "Email verified")
	res, body = s.login(email, password)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")

	var isAdmin bool
	if err := s.db.QueryRow(`SELECT is_admin FROM learners WHERE email = $1`, email).Scan(&isAdmin); err != nil {
		t.Fatal(err)
	}
	if isAdmin {
		t.Fatal("registration with a forged is_admin field made an Admin")
	}
}

func TestCreateAdminMakesAVerifiedAdmin(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email, password = "admin@example.com", "correct horse battery"
	s.loginAsNewAdmin("Abah", email, password)

	var isAdmin, verified bool
	if err := s.db.QueryRow(`SELECT is_admin, verified_at IS NOT NULL FROM learners WHERE email = $1`, email).Scan(&isAdmin, &verified); err != nil {
		t.Fatal(err)
	}
	if !isAdmin || !verified {
		t.Fatalf("admin row: is_admin=%v verified=%v, want both true", isAdmin, verified)
	}
}

func TestCreateAdminPromotesExistingLearner(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	const email = "abah@example.com"
	res, body := s.register("Abah", email, "old password one")
	expectStatus(t, res, body, http.StatusOK, "Check your email")

	if err := s.application.CreateAdmin(context.Background(), "Abah", email, "new password two"); err != nil {
		t.Fatal(err)
	}
	res, body = s.login(email, "old password one")
	expectStatus(t, res, body, http.StatusUnauthorized, "Email or password is incorrect")
	res, body = s.login(email, "new password two")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin")
	expectStatus(t, res, body, http.StatusOK, "Admin area")
}

func TestCreateAdminOnAnExistingAccountSignsItOutEverywhere(t *testing.T) {
	t.Parallel()
	s := newCaveTextSite(t)
	const email = "abah@example.com"
	s.loginAsNewAdmin("Abah", email, "old password one")
	caveTextToken := s.connect()
	res, body := s.login(email, "old password one")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	session := res.Cookies()[0]

	if err := s.application.CreateAdmin(context.Background(), "Abah", email, "new password two"); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/admin", nil)
	req.AddCookie(&http.Cookie{Name: session.Name, Value: session.Value})
	res, body = s.do(req)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("old session after CreateAdmin: %d %q, want a redirect to /login; body:\n%s", res.StatusCode, res.Header.Get("Location"), body)
	}
	res, body = s.caveText(http.MethodGet, "me", caveTextToken)
	expectSignedOut(t, res, body)
}

func TestCreateAdminRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, tc := range []struct{ name, email, password string }{
		{"", "admin@example.com", "long enough"},
		{"Abah", "not an email", "long enough"},
		{"Abah", "admin@example.com", "short"},
	} {
		if err := s.application.CreateAdmin(context.Background(), tc.name, tc.email, tc.password); err == nil {
			t.Fatalf("CreateAdmin(%q, %q, %q) succeeded, want an error", tc.name, tc.email, tc.password)
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM learners`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d learners stored after invalid CreateAdmin calls, want 0", n)
	}
}
