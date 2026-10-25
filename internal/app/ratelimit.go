package app

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	rateLimitWindow = 15 * time.Minute
	// longestWindow is the longest window any key is counted over; a key with no attempt in it is dropped.
	longestWindow = verificationWindow
	// sharedAddressAttempts is each address's budget at sign-up and the account-recovery forms, and for bad links at
	// reset-password: enough for every Learner on one network (a classroom, carrier NAT) to use each once.
	sharedAddressAttempts = 100
	// certificateSearches is each address's budget at the public Certificate search, enough for an employer checking a
	// class but too few to guess ICs.
	certificateSearches = 30
	// Only failed logins count: per account from one address, and per address across all accounts.
	loginFailuresPerAccount = 10
	loginFailuresPerAddress = 100
	// loginFailuresPerEmail caps guesses at one email from all addresses together, which the two above leave
	// unbounded. 50 an hour is past what one Learner forgetting a password reaches (10 per 15 minutes from each
	// address), and with bcrypt and 8-character passwords it holds a distributed guesser to about 1200 a day. The cost:
	// anyone who knows an email can keep its logins refused, an hour at a time; password reset is not limited by it.
	loginFailuresPerEmail = 50
	loginEmailWindow      = time.Hour
	// verificationsPerAddress caps the verification emails and used-email notices to one address together, over sign-ups and
	// resends from anywhere.
	// ponytail: in memory like every count here, so a restart resets it.
	verificationsPerAddress = 3
	verificationWindow      = 24 * time.Hour
	// resetEmailsPerAddress caps the password reset emails to one address from anywhere. An hour, not a day like
	// verifications: anyone who knows an email can hold back its reset emails, so not for long.
	resetEmailsPerAddress = 3
	resetEmailWindow      = time.Hour
)

// rateLimiter counts attempts per key (a form and client address, for logins also the account; in a Chat the
// Learner), in memory.
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
		if !a.limiter.allow(r.URL.Path+" "+clientAddress(r), limit, rateLimitWindow, time.Now()) {
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

// loginKeys are the limiter keys for a login to email from r's client: that account from there, there at all, and
// that account from anywhere. The email is cut to the longest valid address, so a huge one cannot bloat the limiter.
func loginKeys(r *http.Request, email string) (account, address, emailKey string) {
	address = r.URL.Path + " " + clientAddress(r)
	email = strings.ToLower(email[:min(len(email), 254)])
	return address + " " + email, address, r.URL.Path + " " + email
}

// maskEmail keeps a login email's first character and domain, lowercased and cut like in loginKeys, as
// a***@example.com, for the System log.
func maskEmail(email string) string {
	email = strings.ToLower(email[:min(len(email), 254)])
	_, first := utf8.DecodeRuneInString(email)
	at := strings.LastIndex(email, "@")
	if at < first {
		return "***"
	}
	return email[:first] + "***" + email[at:]
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

// allow records an attempt at key at now and reports whether it was within limit attempts in window. Each key is
// always counted over the same window. Recording before the work, not
// after, means attempts sent at once cannot all slip under the limit together.
func (l *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	if l.recent(key, window, now) >= limit {
		return false
	}
	l.attempts[key] = append(l.attempts[key], now)
	return true
}

// allowLogin records a login attempt from address to account, and to emailKey (that email from any address), and
// reports whether all three are within budget. The address goes first, so once it is blocked a flood of new emails
// from it adds no account keys; a refusal leaves no count. report is set on the first refusal by emailKey in its
// window, so the caller logs it once, not on every refusal after it.
func (l *rateLimiter) allowLogin(account, address, emailKey string, now time.Time) (ok, report bool) {
	if !l.allow(address, loginFailuresPerAddress, rateLimitWindow, now) {
		return false, false
	}
	if !l.allow(account, loginFailuresPerAccount, rateLimitWindow, now) {
		l.forget(address)
		return false, false
	}
	if !l.allow(emailKey, loginFailuresPerEmail, loginEmailWindow, now) {
		l.forget(account)
		l.forget(address)
		return false, l.allow("reported "+emailKey, 1, loginEmailWindow, now)
	}
	return true, false
}

// sweep drops, at most once per longestWindow, every key with no attempt in the last longestWindow, so the map doesn't
// grow with each address ever seen. The caller holds mu.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.swept) < longestWindow {
		return
	}
	l.swept = now
	for key := range l.attempts {
		if l.recent(key, longestWindow, now) == 0 {
			delete(l.attempts, key)
		}
	}
}

// forget takes back key's newest attempt, for one that turned out not to count.
func (l *rateLimiter) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch n := len(l.attempts[key]); n {
	case 0, 1:
		delete(l.attempts, key) // an empty key holds nothing
	default:
		l.attempts[key] = l.attempts[key][:n-1]
	}
}

// recent drops key's attempts older than window, and key itself once none are left, and returns how many are left. The caller holds mu.
func (l *rateLimiter) recent(key string, window time.Duration, now time.Time) int {
	if l.attempts == nil {
		l.attempts = map[string][]time.Time{}
	}
	cutoff := now.Add(-window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, key)
	} else {
		l.attempts[key] = kept
	}
	return len(kept)
}
