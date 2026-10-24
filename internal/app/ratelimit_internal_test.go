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
		l.allowLogin(address+" "+strconv.Itoa(i)+"@example.com", address, now)
	}
	if want := loginFailuresPerAddress + 1; len(l.attempts) > want {
		t.Fatalf("limiter keeps %d keys, want at most %d", len(l.attempts), want)
	}
}
