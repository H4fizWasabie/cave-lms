package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"golang.org/x/crypto/bcrypt"
)

// CreateAdmin makes name and email the Admin with a verified account and the given password.
// An existing Learner with that email is promoted and its password replaced, and its sessions and CaveText tokens end.
// It is the only way to make an Admin: registration never can.
func (a *App) CreateAdmin(ctx context.Context, name, email, password string) error {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if _, msg := accountFieldError("en", name, email, password); msg != "" { // the command line speaks English
		return errors.New(msg)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO learners (name, email, password_hash, verified_at, is_admin)
		VALUES ($1, $2, $3, now(), true)
		ON CONFLICT ((lower(email))) DO UPDATE SET
			name = EXCLUDED.name,
			password_hash = EXCLUDED.password_hash,
			verified_at = COALESCE(learners.verified_at, now()),
			is_admin = true
		RETURNING id`,
		name, email, string(hash)).Scan(&id)
	if err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	// A replaced password signs out whoever held the old one, as a password reset does.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE learner_id = $1`, id); err != nil {
		return fmt.Errorf("end sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cavetext_tokens WHERE learner_id = $1`, id); err != nil {
		return fmt.Errorf("end CaveText tokens: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

// admin guards an Admin page: Visitors go to the login page, other Learners get 403.
func (a *App) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if learner == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !learner.IsAdmin {
			lang := requestLanguage(learner.Lang, r)
			a.render(w, r, http.StatusForbidden, "message", page{
				Title:   text.T(lang, "adm.only.title"),
				Message: text.T(lang, "adm.only"),
			})
			return
		}
		h(w, r)
	}
}

func (a *App) adminHome(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "admin", page{})
}
