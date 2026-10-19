package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// CreateAdmin makes name and email the Admin with a verified account and the given password.
// An existing Learner with that email is promoted and its password replaced.
// It is the only way to make an Admin: registration never can.
func (a *App) CreateAdmin(ctx context.Context, name, email, password string) error {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if msg := accountFieldError(name, email, password); msg != "" {
		return errors.New(msg)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = a.db.ExecContext(ctx, `
		INSERT INTO learners (name, email, password_hash, verified_at, is_admin)
		VALUES ($1, $2, $3, now(), true)
		ON CONFLICT ((lower(email))) DO UPDATE SET
			name = EXCLUDED.name,
			password_hash = EXCLUDED.password_hash,
			verified_at = COALESCE(learners.verified_at, now()),
			is_admin = true`,
		name, email, string(hash))
	if err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

// admin guards an Admin page: Visitors go to the login page, other Learners get 403.
func (a *App) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			serverError(w, err)
			return
		}
		if learner == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !learner.IsAdmin {
			a.render(w, http.StatusForbidden, "message", page{
				Title:   "Admin only",
				Message: "Only the Admin can open this page.",
			})
			return
		}
		h(w, r)
	}
}

func (a *App) adminHome(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "admin", page{})
}
