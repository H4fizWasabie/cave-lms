package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	"golang.org/x/crypto/bcrypt"
)

func (a *App) forgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "forgot_password", page{})
}

// forgotPassword answers the same way whether or not the email belongs to a Learner, and whether or not it sent: past
// resetEmailsPerAddress it neither sends nor replaces the link, so the link already sent keeps working.
func (a *App) forgotPassword(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var (
		id        int64
		to, saved string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, email, coalesce(language, '') FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &to, &saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	if err == nil && a.limiter.allow("reset email "+strings.ToLower(to), resetEmailsPerAddress, resetEmailWindow, a.now()) {
		token, err := issueOneTimeToken(r.Context(), tx, id, purposeResetPassword, resetLinkLifetime)
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
		if err := a.mailer.Send(context.WithoutCancel(r.Context()), mailer.Message{
			To:      to,
			Subject: text.T(lang, "email.reset.subject"),
			Body:    text.T(lang, "email.reset.body", a.publicBaseURL+"/reset-password?token="+token),
		}); err != nil {
			a.logError(r, fmt.Errorf("send password reset email: %w", err))
		}
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:   text.T(lang, "message.check-email"),
		Message: text.T(lang, "forgot.sent", email),
	})
}

func (a *App) resetPasswordForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "reset_password", page{Token: r.URL.Query().Get("token")})
}

func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	password := r.PostFormValue("password")
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if problem := passwordProblem(lang, password); problem != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "reset_password", page{Token: token, Error: problem})
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var (
		learnerID int64
		expired   bool
	)
	// Deleting the token as it is read makes every link single-use, expired or not.
	err = tx.QueryRowContext(r.Context(), `
		DELETE FROM one_time_tokens WHERE token_hash = $1 AND purpose = $2
		RETURNING learner_id, expires_at <= now()`,
		hashToken(token), purposeResetPassword).Scan(&learnerID, &expired)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Only bad links count, so Learners on one network each using their own link never run out.
		if !a.limiter.allow(r.URL.Path+" "+clientAddress(r), sharedAddressAttempts, rateLimitWindow, time.Now()) {
			a.tooManyAttempts(w, r)
			return
		}
		a.render(w, r, http.StatusBadRequest, "message", page{
			Title:      text.T(lang, "message.link-invalid"),
			Message:    text.T(lang, "reset.invalid"),
			OfferReset: true,
		})
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	case expired:
		if err := tx.Commit(); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusGone, "message", page{
			Title:      text.T(lang, "message.link-expired"),
			Message:    text.T(lang, "reset.expired"),
			OfferReset: true,
		})
		return
	}
	// Hashing only once the link is known good keeps made-up tokens from costing a bcrypt each.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The reset link reached the inbox, which proves the email as well as a verification link would.
	if _, err := tx.ExecContext(r.Context(), `UPDATE learners SET password_hash = $1, verified_at = coalesce(verified_at, now()) WHERE id = $2`, string(hash), learnerID); err != nil {
		a.serverError(w, r, err)
		return
	}
	// Ending every session and CaveText token means a reset also signs out whoever held the old password.
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE learner_id = $1`, learnerID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM cavetext_tokens WHERE learner_id = $1`, learnerID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "message", page{
		Title:      text.T(lang, "reset.done.title"),
		Message:    text.T(lang, "reset.done"),
		OfferLogin: true,
	})
}
