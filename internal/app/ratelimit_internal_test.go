package app

import (
	"testing"
	"time"
)

// Keys whose attempts have all aged out of the window are dropped, so the limiter doesn't grow with every address.
func TestTheLimiterForgetsKeysOnceTheirWindowHasPassed(t *testing.T) {
	var l rateLimiter
	start := time.Now()
	for _, key := range []string{"POST /login 192.0.2.1", "POST /login 192.0.2.2"} {
		l.allow(key, 10, start)
	}
	l.allow("POST /login 192.0.2.3", 10, start.Add(rateLimitWindow+time.Second))
	if len(l.attempts) != 1 {
		t.Fatalf("limiter keeps %d keys, want only the one with a recent attempt", len(l.attempts))
	}
}
