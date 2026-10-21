package app_test

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

const extensionID = "omcdclpagggfcjlhpkohpcgdmfbelohd"

func newCaveTextSite(t *testing.T) *site {
	return newSite(t, func(c *config.Config) { c.CaveTextExtensionIDs = []string{extensionID} })
}

// connect signs CaveText in through the connect page, as the logged-in Learner, and returns the token.
func (s *site) connect() string {
	s.t.Helper()
	res, body := s.get("/cavetext/connect?extension=" + extensionID)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	prefix := "https://" + extensionID + ".chromiumapp.org/#token="
	location := res.Header.Get("Location")
	if !strings.HasPrefix(location, prefix) || len(location) == len(prefix) {
		s.t.Fatalf("connect redirected to %q, want %s<token>", location, prefix)
	}
	return strings.TrimPrefix(location, prefix)
}

// caveText calls the CaveText API the way the extension does: a bearer token, from the extension's origin.
func (s *site) caveText(method, path, token string) (*http.Response, string) {
	s.t.Helper()
	req, err := http.NewRequest(method, s.server.URL+"/cavetext/api/"+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Origin", "chrome-extension://"+extensionID)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	return s.do(req)
}

type caveTextMe struct {
	Name  string `json:"name"`
	Place int    `json:"place"`
}

func (s *site) me(token string) caveTextMe {
	s.t.Helper()
	res, body := s.caveText(http.MethodGet, "me", token)
	expectStatus(s.t, res, body, http.StatusOK, "")
	var me caveTextMe
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		s.t.Fatalf("me: %v; body: %s", err, body)
	}
	return me
}

func expectSignedOut(t *testing.T, res *http.Response, body string) {
	t.Helper()
	expectStatus(t, res, body, http.StatusUnauthorized, `"error"`)
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("401 Content-Type = %q, want JSON", ct)
	}
}

func TestCaveTextConnectSendsAVisitorToLogInAndBack(t *testing.T) {
	s := newCaveTextSite(t)
	connectPath := "/cavetext/connect?extension=" + extensionID

	res, body := s.get(connectPath)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	loginURL := res.Header.Get("Location")
	if loginURL != "/login?next="+url.QueryEscape(connectPath) {
		t.Fatalf("Visitor sent to %q, want the login page with next", loginURL)
	}
	res, body = s.get(loginURL)
	expectStatus(t, res, body, http.StatusOK, `name="next" value="`+connectPath+`"`)

	const email, password = "ada@example.com", "correct horse battery"
	s.register("Ada Lovelace", email, password)
	s.get(s.lastVerifyPath(email))
	res, body = s.post("/login", url.Values{"email": {email}, "password": {password}, "next": {connectPath}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != connectPath {
		t.Fatalf("login redirected to %q, want %q", got, connectPath)
	}

	if me := s.me(s.connect()); me.Name != "Ada Lovelace" || me.Place != 1 {
		t.Fatalf("me = %+v, want Ada Lovelace in place 1", me)
	}
}

func TestLoginRefusesToRedirectOffSite(t *testing.T) {
	s := newSite(t)
	const email, password = "ada@example.com", "correct horse battery"
	s.register("Ada Lovelace", email, password)
	s.get(s.lastVerifyPath(email))
	// Browsers drop tabs and newlines from URLs, so "/\t/evil.example" would land on //evil.example.
	// Form values arrive decoded, so these also cover next=/%09/evil.example.
	for _, next := range []string{"https://evil.example/", "//evil.example/", `/\evil.example/`,
		"/\t/evil.example/", "/\n/evil.example/", "/\r/evil.example/", "/\r\n/evil.example/", "\t//evil.example/"} {
		res, body := s.get("/login?next=" + url.QueryEscape(next))
		expectStatus(t, res, body, http.StatusOK, "")
		if strings.Contains(body, "evil.example") {
			t.Fatalf("login page for next=%q carries it on", next)
		}
		res, body = s.post("/login", url.Values{"email": {email}, "password": {password}, "next": {next}})
		expectStatus(t, res, body, http.StatusSeeOther, "")
		if got := res.Header.Get("Location"); got != "/" {
			t.Fatalf("next=%q redirected to %q, want /", next, got)
		}
	}
}

func TestCaveTextConnectRefusesAnUnlistedExtension(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	for _, id := range []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ""} {
		res, body := s.get("/cavetext/connect?extension=" + id)
		expectStatus(t, res, body, http.StatusBadRequest, "")
		if res.Header.Get("Location") != "" {
			t.Fatalf("extension %q was redirected to %q", id, res.Header.Get("Location"))
		}
	}
}

func TestCaveTextPlacesAreAssignedOnceAndNeverReused(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada := s.connect()
	bea := s.second()
	bea.loginAsNewLearner("Bea Lim", "bea@example.com", "correct horse battery")
	if me := bea.me(bea.connect()); me.Place != 2 {
		t.Fatalf("Bea's place = %d, want 2", me.Place)
	}

	// Signing in again, and in another browser, keeps Ada's place.
	if me := s.me(s.connect()); me.Place != 1 {
		t.Fatalf("Ada's place after signing in again = %d, want 1", me.Place)
	}
	other := s.second()
	other.login("ada@example.com", "correct horse battery")
	if me := other.me(other.connect()); me.Place != 1 {
		t.Fatalf("Ada's place in another browser = %d, want 1", me.Place)
	}

	// Deleting Ada deletes her tokens and her CaveText account; her place is not handed out again.
	if _, err := s.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	res, body := s.caveText(http.MethodGet, "me", ada)
	expectSignedOut(t, res, body)
	again := s.second()
	again.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	if me := again.me(again.connect()); me.Place != 3 {
		t.Fatalf("Ada's place after re-registering = %d, want 3", me.Place)
	}
}

func TestDeletingALearnerSignsOutCaveTextAndRemovesTheirAccount(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	s.me(token)

	// There is no delete-account page yet; a Learner is deleted by removing the row.
	if _, err := s.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	res, body := s.caveText(http.MethodGet, "me", token)
	expectSignedOut(t, res, body)
	var accounts int
	if err := s.db.QueryRow(`SELECT count(*) FROM cavetext_accounts`).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 {
		t.Fatalf("%d CaveText accounts left after deleting the Learner, want 0", accounts)
	}
}

func TestCaveTextTokensAreHashedAndLastAYearRenewedOnUse(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	hash := sha256.Sum256([]byte(token))

	var yearLeft bool
	if err := s.db.QueryRow(`SELECT expires_at > now() + interval '364 days' FROM cavetext_tokens WHERE token_hash = $1`, hash[:]).Scan(&yearLeft); err != nil {
		t.Fatalf("token not stored by its hash: %v", err)
	}
	if !yearLeft {
		t.Fatal("a new token lasts less than a year")
	}

	if _, err := s.db.Exec(`UPDATE cavetext_tokens SET expires_at = now() + interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	s.me(token)
	if err := s.db.QueryRow(`SELECT expires_at > now() + interval '364 days' FROM cavetext_tokens WHERE token_hash = $1`, hash[:]).Scan(&yearLeft); err != nil || !yearLeft {
		t.Fatalf("using the token did not renew it for a year (err %v)", err)
	}

	if _, err := s.db.Exec(`UPDATE cavetext_tokens SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	res, body := s.caveText(http.MethodGet, "me", token)
	expectSignedOut(t, res, body)
}

func TestCaveTextAPIAcceptsTheBearerTokenOnly(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	s.connect()

	// The session cookie alone is not enough.
	res, body := s.caveText(http.MethodGet, "me", "")
	expectSignedOut(t, res, body)
	res, body = s.caveText(http.MethodGet, "me", "not-a-token")
	expectSignedOut(t, res, body)
	res, body = s.caveText(http.MethodPost, "sign-out", "")
	expectSignedOut(t, res, body)
}

func TestCaveTextSignOutRevokesTheToken(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()
	other := s.connect()

	res, body := s.caveText(http.MethodPost, "sign-out", token)
	expectStatus(t, res, body, http.StatusNoContent, "")
	res, body = s.caveText(http.MethodGet, "me", token)
	expectSignedOut(t, res, body)
	s.me(other) // another browser stays signed in
}

func TestLoggingOutOfTheSiteKeepsCaveTextSignedIn(t *testing.T) {
	s := newCaveTextSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	token := s.connect()

	res, body := s.post("/logout", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if me := s.me(token); me.Name != "Ada Lovelace" {
		t.Fatalf("me after logout = %+v", me)
	}
}
