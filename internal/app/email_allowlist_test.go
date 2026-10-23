package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// Staging emails only the allow-list; anyone else's email is kept in the System log for the Owner instead (ADR 0010).
func TestWithAnAllowListEmailToAnyoneElseGoesToTheSystemLogInstead(t *testing.T) {
	s := newSite(t, func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.EmailAllowlist = []string{"Abah@Example.com", "@client.example"}
	})
	for _, email := range []string{"ada@example.com", "abah@example.com", "zed@Client.Example"} {
		res, body := s.register("Learner", email, "correct horse battery")
		expectStatus(t, res, body, http.StatusOK, "Check your email")
	}
	var sentTo []string
	for _, m := range s.mail.Messages() {
		sentTo = append(sentTo, m.To)
	}
	if got := strings.Join(sentTo, ","); got != "abah@example.com,zed@Client.Example" {
		t.Fatalf("emails sent to %q, want only the allow-listed abah@example.com and zed@Client.Example", got)
	}
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	_, body := s.get("/admin/log")
	expectAll(t, body, "not on EMAIL_ALLOWLIST", "ada@example.com", "https://cave.example.invalid/verify?token=")
}
