package app

// An internal test: the sweep needs explicit times, which aren't reachable over HTTP.

import (
	"strconv"
	"testing"
	"time"
)

// Keys whose attempts have all aged out of the window are dropped, so the limiter doesn't grow with every address.
func TestTheLimiterForgetsKeysOnceTheirWindowHasPassed(t *testing.T) {
	var l rateLimiter
	start := time.Now()
	for _, key := range []string{"POST /login 192.0.2.1", "POST /login 192.0.2.2"} {
		l.allow(key, 10, rateLimitWindow, start)
	}
	l.allow("POST /login 192.0.2.3", 10, rateLimitWindow, start.Add(longestWindow+time.Second))
	if len(l.attempts) != 1 {
		t.Fatalf("limiter keeps %d keys, want only the one with a recent attempt", len(l.attempts))
	}
}

// A key whose only attempt is taken back holds nothing, so it is dropped, not kept until the sweep.
func TestTheLimiterDropsAKeyWhoseAttemptIsTakenBack(t *testing.T) {
	var l rateLimiter
	l.allow("POST /login 192.0.2.1 ada@example.com", 10, rateLimitWindow, time.Now())
	l.forget("POST /login 192.0.2.1 ada@example.com")
	if len(l.attempts) != 0 {
		t.Fatalf("limiter keeps %d keys, want none", len(l.attempts))
	}
}

// Once an address has used its login budget, logins from it to new emails add no keys: the address is checked first.
func TestABlockedAddressAddsNoAccountKeys(t *testing.T) {
	var l rateLimiter
	now := time.Now()
	const address = "POST /login 192.0.2.1"
	for i := range loginFailuresPerAddress + 1000 {
		email := strconv.Itoa(i) + "@example.com"
		l.allowLogin(address+" "+email, address, "POST /login "+email, now)
	}
	// One address key, and an account and an email key per login it let through.
	if want := 2*loginFailuresPerAddress + 1; len(l.attempts) > want {
		t.Fatalf("limiter keeps %d keys, want at most %d", len(l.attempts), want)
	}
}

// Failed logins to one email from many addresses stop at loginFailuresPerEmail, and the first refusal is reported
// once, not on every refusal after it.
func TestLoginsToOneEmailFromManyAddressesStopAtTheEmailLimit(t *testing.T) {
	var l rateLimiter
	now := time.Now()
	allowed, reported := 0, 0
	for i := range loginFailuresPerEmail + 20 {
		address := "POST /login 192.0.2." + strconv.Itoa(i)
		ok, report := l.allowLogin(address+" ada@example.com", address, "POST /login ada@example.com", now)
		if ok {
			allowed++
		}
		if report {
			reported++
		}
	}
	if allowed != loginFailuresPerEmail || reported != 1 {
		t.Fatalf("allowed %d logins and reported %d times, want %d and 1", allowed, reported, loginFailuresPerEmail)
	}
	// A refusal leaves no count at the address it came from.
	if n := len(l.attempts["POST /login 192.0.2.60"]); n != 0 {
		t.Fatalf("refused address kept %d attempts, want 0", n)
	}
}
