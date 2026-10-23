package app_test

import (
	"net/http"
	"testing"
	"time"
)

// A login to an email with no account takes as long as a wrong password, so timing doesn't tell which emails exist.
func TestALoginToAnUnknownEmailTakesAsLongAsAWrongPassword(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	visitor := s.second()
	timed := func(email string) time.Duration {
		start := time.Now()
		res, body := visitor.login(email, "wrong password here")
		expectStatus(t, res, body, http.StatusUnauthorized, "")
		return time.Since(start)
	}
	// Alternated and the fastest of each kept, so a busy machine slows both alike.
	known, unknown := time.Hour, time.Hour
	for range 5 {
		known = min(known, timed("ada@example.com"))
		unknown = min(unknown, timed("nobody@example.com"))
	}
	if unknown < known/4 {
		t.Fatalf("a login to an unknown email took %v, a wrong password %v; want them alike", unknown, known)
	}
}
