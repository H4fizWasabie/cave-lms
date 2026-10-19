package app

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	rateLimitWindow   = 15 * time.Minute
	rateLimitAttempts = 10
)

// rateLimiter counts attempts per client address, in memory.
// ponytail: per process and reset on restart; move the counts to Postgres if there is ever more than one server.
// ponytail: entries are never swept, so memory grows with distinct client addresses; prune when that matters.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// limited refuses the request with 429 once its client has made too many attempts in the window.
func (a *App) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.allow(clientAddress(r), time.Now()) {
			a.render(w, http.StatusTooManyRequests, "message", page{
				Title:   "Too many attempts",
				Message: "Too many attempts from your address. Wait 15 minutes and try again.",
			})
			return
		}
		next(w, r)
	}
}

// clientAddress is the host part of r.RemoteAddr. Behind a reverse proxy such as Caddy that is the
// proxy's address, so every visitor shares one limit until forwarded headers are trusted.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allow records an attempt by client at now and reports whether it is within the limit.
func (l *rateLimiter) allow(client string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.attempts == nil {
		l.attempts = map[string][]time.Time{}
	}
	cutoff := now.Add(-rateLimitWindow)
	recent := l.attempts[client][:0]
	for _, t := range l.attempts[client] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	allowed := len(recent) < rateLimitAttempts
	if allowed {
		recent = append(recent, now)
	}
	l.attempts[client] = recent
	return allowed
}
