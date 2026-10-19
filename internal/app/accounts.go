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
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie      = "__Host-session"
	sessionLifetime    = 30 * 24 * time.Hour
	verifyLinkLifetime = 24 * time.Hour
	purposeVerifyEmail = "verify_email"
)

type learner struct {
	ID      int64
	Name    string
	IsAdmin bool
}

// currentLearner returns the logged-in Learner, or nil for a Visitor.
func (a *App) currentLearner(r *http.Request) (*learner, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, nil
	}
	var l learner
	err = a.db.QueryRowContext(r.Context(), `
		SELECT l.id, l.name, l.is_admin FROM sessions s JOIN learners l ON l.id = s.learner_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(cookie.Value)).Scan(&l.ID, &l.Name, &l.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up session: %w", err)
	}
	return &l, nil
}

func (a *App) registerForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "register", page{})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	form := page{Name: name, Email: email, Error: accountFieldError(name, email, password)}
	if form.Error != "" {
		a.render(w, http.StatusUnprocessableEntity, "register", form)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		serverError(w, err)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO learners (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		name, email, string(hash)).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		form.Error = "This email is already registered. Log in instead."
		a.render(w, http.StatusConflict, "register", form)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	// Commit only once the email is sent, so a failed send leaves no half-made account.
	if err := a.sendVerification(r.Context(), tx, id, name, email); err != nil {
		serverError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "message", page{
		Title:   "Check your email",
		Message: "We sent a verification link to " + email + ". Open it within 24 hours to confirm your account.",
	})
}

// sendVerification replaces the Learner's verification link with a new one and emails it.
func (a *App) sendVerification(ctx context.Context, tx *sql.Tx, learnerID int64, name, email string) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM one_time_tokens WHERE learner_id = $1 AND purpose = $2`, learnerID, purposeVerifyEmail); err != nil {
		return fmt.Errorf("delete old verification links: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO one_time_tokens (token_hash, learner_id, purpose, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))`,
		hashToken(token), learnerID, purposeVerifyEmail, verifyLinkLifetime.Seconds()); err != nil {
		return fmt.Errorf("store verification link: %w", err)
	}
	return a.mailer.Send(ctx, mailer.Message{
		To:      email,
		Subject: "Confirm your Cave LMS account",
		Body: "Hi " + name + ",\n\nOpen this link within 24 hours to confirm your Cave LMS account:\n\n" +
			a.publicBaseURL + "/verify?token=" + token + "\n\nIf you didn't register, you can ignore this email.\n",
	})
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
	switch {
	case errors.Is(err, sql.ErrNoRows):
		a.render(w, http.StatusBadRequest, "message", page{
			Title:       "Link not valid",
			Message:     "This verification link is not valid or has already been used. Enter your email to get a new one.",
			OfferResend: true,
		})
	case err != nil:
		serverError(w, err)
	case expired:
		a.render(w, http.StatusGone, "message", page{
			Title:       "Link expired",
			Message:     "This verification link has expired. Send yourself a new one.",
			OfferResend: true,
			Email:       email,
		})
	default:
		if _, err := a.db.ExecContext(r.Context(), `UPDATE learners SET verified_at = now() WHERE id = $1 AND verified_at IS NULL`, learnerID); err != nil {
			serverError(w, err)
			return
		}
		a.render(w, http.StatusOK, "message", page{Title: "Email verified", Message: "Your account is confirmed.", OfferLogin: true})
	}
}

func (a *App) resendVerification(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	var (
		id       int64
		name, to string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, name, email FROM learners WHERE lower(email) = lower($1) AND verified_at IS NULL`, email).Scan(&id, &name, &to)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}
	if err == nil {
		if err := a.sendVerification(r.Context(), tx, id, name, to); err != nil {
			serverError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			serverError(w, err)
			return
		}
	}
	a.render(w, http.StatusOK, "message", page{
		Title:   "Check your email",
		Message: "If " + email + " belongs to an account waiting for verification, we sent it a new link.",
	})
}

func (a *App) loginForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "login", page{})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	var (
		id       int64
		hash     string
		verified bool
	)
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash, verified_at IS NOT NULL FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &hash, &verified)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.PostFormValue("password"))) != nil {
		a.render(w, http.StatusUnauthorized, "login", page{Email: email, Error: "Email or password is incorrect."})
		return
	}
	if !verified {
		a.render(w, http.StatusForbidden, "message", page{
			Title:       "Verify your email first",
			Message:     "Open the link we emailed you to confirm your account, then log in. Lost it? Send a new one.",
			OfferResend: true,
			Email:       email,
		})
		return
	}

	token, err := newToken()
	if err != nil {
		serverError(w, err)
		return
	}
	// ponytail: expired session rows are never deleted; add a cleanup when the table grows.
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO sessions (token_hash, learner_id, expires_at) VALUES ($1, $2, now() + make_interval(secs => $3))`,
		hashToken(token), id, sessionLifetime.Seconds()); err != nil {
		serverError(w, err)
		return
	}
	setSessionCookie(w, token, int(sessionLifetime.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if _, err := a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = $1`, hashToken(cookie.Value)); err != nil {
			serverError(w, err)
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

// accountFieldError returns a message for the first invalid name, email or password, or "".
func accountFieldError(name, email, password string) string {
	switch {
	case name == "" || len(name) > 200:
		return "Enter your name (up to 200 characters)."
	case !validEmail(email):
		return "Enter a valid email address."
	case len(password) < 8:
		return "Use a password of at least 8 characters."
	case len(password) > 72: // bcrypt's limit
		return "Use a password of at most 72 characters."
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
