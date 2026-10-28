package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var randomUsername = regexp.MustCompile(`^learner-[a-z0-9]{6}$`)

func TestRegisteringNeedsAValidUsernameNobodyHasTaken(t *testing.T) {
	s := newSite(t)
	register := func(username, email string) (*http.Response, string) {
		return s.post("/register", url.Values{"name": {"Ada Lovelace"}, "username": {username}, "email": {email}, "password": {"correct horse battery"}})
	}
	for _, c := range []struct{ username, want string }{
		{"", "The Username must be 3 to 40 characters"},
		{"ab", "The Username must be 3 to 40 characters"},
		{"Ada Lovelace", "single dashes in the Username"},
		{"ada--l", "single dashes in the Username"},
		{"admin", "That Username is reserved for CAVELMS"},
	} {
		res, body := register(c.username, "ada@example.com")
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectFieldError(t, body, "username", c.want, `for="email"`)
		expectInOrder(t, mainOf(body), `id="username"`, `value="`+c.username+`"`)
	}
	if n := len(s.mail.Messages()); n != 0 {
		t.Fatalf("sent %d emails for refused Usernames, want 0", n)
	}

	res, body := register("ada-l", "ada@example.com")
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	res, body = register("ada-l", "bea@example.com")
	expectStatus(t, res, body, http.StatusConflict, "That Username is already taken")
	expectFieldError(t, body, "username", "That Username is already taken", `for="email"`)
	if n := len(s.mail.Messages()); n != 1 {
		t.Fatalf("sent %d emails, want only Ada's verification", n)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT username FROM learners WHERE email = 'ada@example.com'`).Scan(&stored); err != nil || stored != "ada-l" {
		t.Fatalf("Ada's Username = %q, %v", stored, err)
	}
}

func TestTheRegisterFormExplainsTheUsernameInMalayToo(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/register", "en")
	expectLanguage(t, body, "en", "so do not use your real name")
	_, body = s.getAsking("/register", "ms")
	expectLanguage(t, body, "ms", "jangan gunakan nama sebenar anda")
	res, body := s.postAsking("/register", "ms", url.Values{"name": {"Ada"}, "username": {"ab"}, "email": {"ada@example.com"}, "password": {"correct horse battery"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "3 hingga 40 aksara")
}

func TestALearnerChangesTheirUsernameInMyProfile(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	bea := s.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")

	res, body := s.get("/my-profile")
	expectStatus(t, res, body, http.StatusOK, `name="username" value="ada-example-com"`)

	for _, c := range []struct {
		username string
		status   int
		want     string
	}{
		{"x", http.StatusUnprocessableEntity, "must be 3 to 40 characters"},
		{"Ada L", http.StatusUnprocessableEntity, "Use only lowercase letters"},
		{"login", http.StatusUnprocessableEntity, "reserved"},
		{"bea-example-com", http.StatusConflict, "That Username is already taken"},
	} {
		res, body = s.post("/my-profile/username", url.Values{"username": {c.username}})
		expectStatus(t, res, body, c.status, c.want)
		expectStatus(t, res, body, c.status, `value="`+c.username+`"`)
	}
	if _, body = s.get("/my-profile"); !strings.Contains(body, `value="ada-example-com"`) {
		t.Fatalf("a refused Username changed the profile: %s", body)
	}

	res, body = s.post("/my-profile/username", url.Values{"username": {"ada-l"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if res.Header.Get("Location") != "/my-profile?saved" {
		t.Fatalf("saving went to %q", res.Header.Get("Location"))
	}
	res, body = s.get("/my-profile?saved")
	expectStatus(t, res, body, http.StatusOK, `value="ada-l"`)
	expectStatus(t, res, body, http.StatusOK, `<p class="field-ok" role="status">Saved.</p>`)
	// Giving up a Username frees it for another Learner.
	res, body = bea.post("/my-profile/username", url.Values{"username": {"ada-example-com"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = s.second().post("/my-profile/username", url.Values{"username": {"visitor"}})
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("a Visitor got %d to %q, want sent to log in", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestMyProfileSaysSavedOnlyAfterASave(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")

	_, body := s.get("/my-profile")
	expectNone(t, body, "Saved.")
	res, body := s.post("/my-profile/username", url.Values{"username": {"x"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "must be 3 to 40 characters")
	expectNone(t, body, "Saved.")

	_, body = s.getAsking("/my-profile?saved", "ms")
	expectLanguage(t, body, "ms", `<p class="field-ok" role="status">Disimpan.</p>`)
}

func TestTheAdminGetsARandomUsername(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	var username string
	if err := s.db.QueryRow(`SELECT username FROM learners WHERE email = 'admin@example.com'`).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if !randomUsername.MatchString(username) {
		t.Fatalf("the Admin's Username is %q, want learner- and 6 lowercase letters or digits", username)
	}
	res, _ := s.get("/my-folder")
	if res.Header.Get("Location") != "/my-folder/"+username {
		t.Fatalf("the Admin's Folder is at %q", res.Header.Get("Location"))
	}
}
