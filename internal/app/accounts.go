package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie        = "__Host-session"
	sessionLifetime      = 30 * 24 * time.Hour
	verifyLinkLifetime   = 24 * time.Hour
	purposeVerifyEmail   = "verify_email"
	purposeResetPassword = "reset_password"
	resetLinkLifetime    = time.Hour
)

type learner struct {
	ID      int64
	Name    string
	IsAdmin bool
	IsOwner bool   // the Admin whose email is OWNER_EMAIL
	Lang    string // the saved language, or "" before the Learner first switches
}

// learnerKey keys the per-request memo of the session lookup that ServeHTTP installs.
type learnerKey struct{}

// currentLearner returns the logged-in Learner, or nil for a Visitor.
// The session is looked up at most once per request, however many handlers ask.
func (a *App) currentLearner(r *http.Request) (*learner, error) {
	if lookup, ok := r.Context().Value(learnerKey{}).(func() (*learner, error)); ok {
		return lookup()
	}
	return a.lookUpLearner(r)
}

func (a *App) lookUpLearner(r *http.Request) (*learner, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, nil
	}
	var l learner
	err = a.db.QueryRowContext(r.Context(), `
		SELECT l.id, l.name, l.is_admin, l.is_admin AND $2 <> '' AND lower(l.email) = lower($2), coalesce(l.language, '')
		FROM sessions s JOIN learners l ON l.id = s.learner_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(cookie.Value), a.ownerEmail).Scan(&l.ID, &l.Name, &l.IsAdmin, &l.IsOwner, &l.Lang)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up session: %w", err)
	}
	return &l, nil
}

func (a *App) registerForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "register", page{})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	form := page{Name: name, Email: email, Error: accountFieldError(lang, name, email, password)}
	if form.Error != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "register", form)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO learners (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		name, email, string(hash)).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		form.Error = text.T(lang, "register.err.taken")
		a.render(w, r, http.StatusConflict, "register", form)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// Commit only once the email is sent, so a failed send leaves no half-made account.
	if err := a.sendVerification(r.Context(), tx, lang, id, name, email); err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:   text.T(lang, "message.check-email"),
		Message: text.T(lang, "register.sent", email),
	})
}

// sendVerification replaces the Learner's verification link with a new one and emails it in lang.
func (a *App) sendVerification(ctx context.Context, tx *sql.Tx, lang string, learnerID int64, name, email string) error {
	token, err := issueOneTimeToken(ctx, tx, learnerID, purposeVerifyEmail, verifyLinkLifetime)
	if err != nil {
		return err
	}
	return a.mailVerification(ctx, lang, name, email, token)
}

// mailVerification emails the verification link for token in lang.
func (a *App) mailVerification(ctx context.Context, lang, name, email, token string) error {
	return a.mailer.Send(ctx, mailer.Message{
		To:      email,
		Subject: text.T(lang, "email.verify.subject"),
		Body:    text.T(lang, "email.verify.body", name, a.publicBaseURL+"/verify?token="+token),
	})
}

// recipientLanguage is the language to email a Learner in: their saved one, else the request's.
func (a *App) recipientLanguage(r *http.Request, saved string) (string, error) {
	if text.Supported(saved) {
		return saved, nil
	}
	return a.language(r)
}

// issueOneTimeToken replaces the Learner's links for purpose with a new one and returns the token to email.
func issueOneTimeToken(ctx context.Context, tx *sql.Tx, learnerID int64, purpose string, lifetime time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM one_time_tokens WHERE learner_id = $1 AND purpose = $2`, learnerID, purpose); err != nil {
		return "", fmt.Errorf("delete old %s links: %w", purpose, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO one_time_tokens (token_hash, learner_id, purpose, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))`,
		hashToken(token), learnerID, purpose, lifetime.Seconds()); err != nil {
		return "", fmt.Errorf("store %s link: %w", purpose, err)
	}
	return token, nil
}

func (a *App) verify(w http.ResponseWriter, r *http.Request) {
	var (
		learnerID int64
		email     string
		expired   bool
	)
	// Deleting the token as it is read makes every link single-use.
	err := a.db.QueryRowContext(r.Context(), `
		DELETE FROM one_time_tokens t USING learners l
		WHERE t.learner_id = l.id AND t.token_hash = $1 AND t.purpose = $2
		RETURNING l.id, l.email, t.expires_at <= now()`,
		hashToken(r.URL.Query().Get("token")), purposeVerifyEmail).Scan(&learnerID, &email, &expired)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	lang, langErr := a.language(r)
	if langErr != nil {
		a.serverError(w, r, langErr)
		return
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		a.render(w, r, http.StatusBadRequest, "message", page{
			Title:       text.T(lang, "message.link-invalid"),
			Message:     text.T(lang, "verify.invalid"),
			OfferResend: true,
		})
	case expired:
		a.render(w, r, http.StatusGone, "message", page{
			Title:       text.T(lang, "message.link-expired"),
			Message:     text.T(lang, "verify.expired"),
			OfferResend: true,
			Email:       email,
		})
	default:
		if _, err := a.db.ExecContext(r.Context(), `UPDATE learners SET verified_at = now() WHERE id = $1 AND verified_at IS NULL`, learnerID); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusOK, "message", page{Title: text.T(lang, "verify.done.title"), Message: text.T(lang, "verify.done"), OfferLogin: true})
	}
}

func (a *App) resendVerification(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var (
		id              int64
		name, to, saved string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, name, email, coalesce(language, '') FROM learners WHERE lower(email) = lower($1) AND verified_at IS NULL`, email).Scan(&id, &name, &to, &saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	if err == nil {
		token, err := issueOneTimeToken(r.Context(), tx, id, purposeVerifyEmail, verifyLinkLifetime)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if err := tx.Commit(); err != nil {
			a.serverError(w, r, err)
			return
		}
		lang, err := a.recipientLanguage(r, saved)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		// A failed send is only logged: an error page would reveal that the account exists.
		if err := a.mailVerification(r.Context(), lang, name, to, token); err != nil {
			a.logError(r, fmt.Errorf("send verification email: %w", err))
		}
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:   text.T(lang, "message.check-email"),
		Message: text.T(lang, "verify.resent", email),
	})
}

func (a *App) loginForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "login", page{Next: localPath(r.URL.Query().Get("next"))})
}

// localPath returns next if it is a path on this site, else "", so a login link can't send anyone off-site.
// Control characters are refused outright: browsers drop tabs and newlines, so "/\t/evil.com" becomes "//evil.com".
func localPath(next string) string {
	if strings.ContainsFunc(next, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") ||
		strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return ""
	}
	return next
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	next := localPath(r.PostFormValue("next"))
	// Every attempt is counted first; a right password takes its count back, so only failures stay and a class sharing
	// one address all gets in while guessing is still stopped.
	account, address := loginKeys(r, email)
	now := time.Now()
	if !a.limiter.allow(account, loginFailuresPerAccount, now) {
		a.tooManyAttempts(w, r)
		return
	}
	if !a.limiter.allow(address, loginFailuresPerAddress, now) {
		a.limiter.forget(account)
		a.tooManyAttempts(w, r)
		return
	}
	var (
		id       int64
		hash     string
		verified bool
		saved    string
	)
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash, verified_at IS NOT NULL, coalesce(language, '') FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &hash, &verified, &saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	failed := err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.PostFormValue("password"))) != nil
	if !failed {
		a.limiter.forget(account)
		a.limiter.forget(address)
	}
	if failed || !verified {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if failed {
			a.render(w, r, http.StatusUnauthorized, "login", page{Email: email, Error: text.T(lang, "login.err.wrong"), Next: next})
			return
		}
		a.render(w, r, http.StatusForbidden, "message", page{
			Title:       text.T(lang, "login.unverified.title"),
			Message:     text.T(lang, "login.unverified"),
			OfferResend: true,
			Email:       email,
		})
		return
	}

	token, err := newToken()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// ponytail: expired session rows are never deleted; add a cleanup when the table grows.
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO sessions (token_hash, learner_id, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3))`,
		hashToken(token), id, sessionLifetime.Seconds()); err != nil {
		a.serverError(w, r, err)
		return
	}
	setSessionCookie(w, token, int(sessionLifetime.Seconds()))
	if saved != "" {
		setLanguageCookie(w, saved) // so the site stays in it after logging out
	}
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if _, err := a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = $1`, hashToken(cookie.Value)); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	setSessionCookie(w, "", -1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// accountFieldError returns a message in lang for the first invalid name, email or password, or "".
func accountFieldError(lang, name, email, password string) string {
	switch {
	case name == "" || len(name) > 200:
		return text.T(lang, "account.err.name")
	case !validEmail(email):
		return text.T(lang, "account.err.email")
	}
	return passwordProblem(lang, password)
}

// passwordProblem says in lang why a password is unacceptable, or returns "" if it is fine.
func passwordProblem(lang, password string) string {
	switch {
	case len(password) < 8:
		return text.T(lang, "account.err.password-short")
	case len(password) > 72: // bcrypt's limit
		return text.T(lang, "account.err.password-long")
	}
	return ""
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && len(email) <= 254
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
