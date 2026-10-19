package app

import (
	"net"
	"net/http"
	"strings"
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

// limited refuses the request with 429 once its client has made too many attempts at this form in the window.
// Each form has its own budget, so failed logins never block a password reset.
func (a *App) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.allow(r.URL.Path+" "+clientAddress(r), time.Now()) {
			a.render(w, http.StatusTooManyRequests, "message", page{
				Title:   "Too many attempts",
				Message: "Too many attempts from your address. Wait 15 minutes and try again.",
			})
			return
		}
		next(w, r)
	}
}

// clientAddress is the host part of r.RemoteAddr. When that is loopback, the request came through the local
// reverse proxy (Caddy), which appends the real client as the last X-Forwarded-For entry; earlier entries are
// whatever the client sent, so only the last one is trusted.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			entries := strings.Split(xff[len(xff)-1], ",")
			if last := strings.TrimSpace(entries[len(entries)-1]); last != "" {
				return last
			}
		}
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
