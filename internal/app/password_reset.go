package app

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"golang.org/x/crypto/bcrypt"
)

func (a *App) forgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "forgot_password", page{})
}

// forgotPassword answers the same way whether or not the email belongs to a Learner.
func (a *App) forgotPassword(w http.ResponseWriter, r *http.Request) {
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
		`SELECT id, name, email FROM learners WHERE lower(email) = lower($1)`, email).Scan(&id, &name, &to)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}
	if err == nil {
		token, err := issueOneTimeToken(r.Context(), tx, id, purposeResetPassword, resetLinkLifetime)
		if err != nil {
			serverError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			serverError(w, err)
			return
		}
		// A failed send is only logged: an error page would reveal that the account exists.
		if err := a.mailer.Send(r.Context(), mailer.Message{
			To:      to,
			Subject: "Reset your Cave LMS password",
			Body: "Hi " + name + ",\n\nOpen this link within an hour to choose a new password:\n\n" +
				a.publicBaseURL + "/reset-password?token=" + token + "\n\nIf you didn't ask for this, you can ignore this email.\n",
		}); err != nil {
			log.Printf("send password reset email: %v", err)
		}
	}
	a.render(w, http.StatusOK, "message", page{
		Title:   "Check your email",
		Message: "If an account exists for " + email + ", we sent a reset link. Open it within an hour to choose a new password.",
	})
}

func (a *App) resetPasswordForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "reset_password", page{Token: r.URL.Query().Get("token")})
}

func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	password := r.PostFormValue("password")
	if problem := passwordProblem(password); problem != "" {
		a.render(w, http.StatusUnprocessableEntity, "reset_password", page{Token: token, Error: problem})
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
		a.render(w, http.StatusBadRequest, "message", page{
			Title:      "Link not valid",
			Message:    "This reset link is not valid or has already been used.",
			OfferReset: true,
		})
		return
	case err != nil:
		serverError(w, err)
		return
	case expired:
		if err := tx.Commit(); err != nil {
			serverError(w, err)
			return
		}
		a.render(w, http.StatusGone, "message", page{
			Title:      "Link expired",
			Message:    "This reset link has expired.",
			OfferReset: true,
		})
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE learners SET password_hash = $1 WHERE id = $2`, string(hash), learnerID); err != nil {
		serverError(w, err)
		return
	}
	// Ending every session means a reset also signs out whoever held the old password.
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE learner_id = $1`, learnerID); err != nil {
		serverError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "message", page{
		Title:      "Password changed",
		Message:    "Your password has been changed and you have been logged out everywhere.",
		OfferLogin: true,
	})
}
