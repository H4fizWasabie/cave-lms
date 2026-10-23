package app

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	rateLimitWindow = 15 * time.Minute
	// rateLimitAttempts is each address's budget at most forms. Sign-up allows a whole class on one network.
	rateLimitAttempts = 10
	signUpAttempts    = 100
	// certificateSearches is each address's budget at the public Certificate search, enough for an employer checking a
	// class but too few to guess ICs.
	certificateSearches = 30
	// Only failed logins count: per account from one address, and per address across all accounts.
	loginFailuresPerAccount = 10
	loginFailuresPerAddress = 100
)

// rateLimiter counts attempts per key (a form and client address, for logins also the account), in memory.
// ponytail: per process and reset on restart; move the counts to Postgres if there is ever more than one server.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	swept    time.Time // when keys with no recent attempt were last dropped
}

// limited refuses the request with 429 once its client has made limit attempts at this form in the window.
// Each form has its own budget, so failed logins never block a password reset.
func (a *App) limited(limit int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.allow(r.URL.Path+" "+clientAddress(r), limit, time.Now()) {
			a.tooManyAttempts(w, r)
			return
		}
		next(w, r)
	}
}

func (a *App) tooManyAttempts(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusTooManyRequests, "message", page{
		Title:   text.T(lang, "limited.title"),
		Message: text.T(lang, "limited"),
	})
}

// loginKeys are the limiter keys for a login to email from r's client: that account from there, and there at all.
// The email is cut to the longest valid address, so a huge one cannot bloat the limiter.
func loginKeys(r *http.Request, email string) (account, address string) {
	address = r.URL.Path + " " + clientAddress(r)
	return address + " " + strings.ToLower(email[:min(len(email), 254)]), address
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

// allow records an attempt at key at now and reports whether it was within limit. Recording before the work, not
// after, means attempts sent at once cannot all slip under the limit together.
func (l *rateLimiter) allow(key string, limit int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	if l.recent(key, now) >= limit {
		return false
	}
	l.attempts[key] = append(l.attempts[key], now)
	return true
}

// sweep drops, at most once a window, every key with no attempt left in it, so the map doesn't grow with each
// address ever seen. The caller holds mu.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.swept) < rateLimitWindow {
		return
	}
	l.swept = now
	for key := range l.attempts {
		if l.recent(key, now) == 0 {
			delete(l.attempts, key)
		}
	}
}

// forget takes back key's newest attempt, for one that turned out not to count.
func (l *rateLimiter) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.attempts[key]); n > 0 {
		l.attempts[key] = l.attempts[key][:n-1]
	}
}

// recent drops key's attempts older than the window and returns how many are left. The caller holds mu.
func (l *rateLimiter) recent(key string, now time.Time) int {
	if l.attempts == nil {
		l.attempts = map[string][]time.Time{}
	}
	cutoff := now.Add(-rateLimitWindow)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[key] = kept
	return len(kept)
}
