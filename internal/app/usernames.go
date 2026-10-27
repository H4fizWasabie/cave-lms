package app

import (
	"crypto/rand"
	"net/http"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// randomUsername is a Username for a Learner who has not picked one: learner- and 6 lowercase letters or digits.
// A clash with another Learner's is a one in two billion chance, and the unique index would refuse the insert.
func randomUsername() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "learner-" + string(b), nil
}

// setMyUsername is the Learner changing their Username. Their old links lead to the new one (ADR 0023).
func (a *App) setMyUsername(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	status, key := http.StatusUnprocessableEntity, nameError(username)
	if key == "" {
		_, err = a.db.ExecContext(r.Context(), `UPDATE learners SET username = $1 WHERE id = $2`, username, learner.ID)
		switch {
		case isUniqueViolation(err):
			status, key = http.StatusConflict, "account.err.username-taken"
		case err != nil:
			a.serverError(w, r, err)
			return
		default:
			http.Redirect(w, r, "/my-profile", http.StatusSeeOther)
			return
		}
	}
	ic, err := a.learnerIC(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner}, IC: formatIC(ic),
		UsernameError: text.T(lang, key), TypedUsername: username})
}
