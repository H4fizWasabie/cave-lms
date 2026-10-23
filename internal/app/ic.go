package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// normaliseIC is how an IC or passport number is stored and searched: spaces and dashes removed, letters upper case.
func normaliseIC(s string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
}

// validIC is a normalised IC or passport number a Learner may store: letters and digits, up to 20.
var validIC = regexp.MustCompile(`^[A-Z0-9]{1,20}$`)

// formatIC shows a stored IC: 12 digits is a MyKad IC, shown as YYMMDD-PB-####; anything else as stored.
func formatIC(ic string) string {
	if len(ic) == 12 && strings.Trim(ic, "0123456789") == "" {
		return ic[:6] + "-" + ic[6:8] + "-" + ic[8:]
	}
	return ic
}

// myProfilePage is the logged-in Learner's profile: their IC or passport number, formatted, or a form to set it.
type myProfilePage struct {
	pageBase
	IC string
}

func (a *App) myProfile(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ic, err := a.learnerIC(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner}, IC: formatIC(ic)})
}

// setMyIC is the Learner setting their IC or passport number, once. After that only the Admin can change it.
func (a *App) setMyIC(w http.ResponseWriter, r *http.Request) {
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
	refuse := func(status int, key string) {
		a.render(w, r, status, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner, Error: text.T(lang, key)}})
	}
	ic := normaliseIC(r.PostFormValue("ic"))
	if !validIC.MatchString(ic) {
		refuse(http.StatusUnprocessableEntity, "ic.invalid")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE learners SET ic = $1 WHERE id = $2 AND ic IS NULL`, ic, learner.ID)
	if isUniqueViolation(err) {
		refuse(http.StatusConflict, "ic.taken")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if n == 0 { // already set
		ic, err := a.learnerIC(r.Context(), learner.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusConflict, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner, Error: text.T(lang, "ic.set-once")}, IC: formatIC(ic)})
		return
	}
	if err := a.issueEarnedCertificates(r.Context(), learner.ID, 0); err != nil {
		a.logError(r, fmt.Errorf("issue Certificates on setting learner %d's IC: %w", learner.ID, err))
	}
	http.Redirect(w, r, "/my-profile", http.StatusSeeOther)
}

// learnerIC returns a Learner's stored IC or passport number, or "" when unset.
func (a *App) learnerIC(ctx context.Context, learnerID int64) (string, error) {
	var ic sql.NullString
	err := a.db.QueryRowContext(ctx, `SELECT ic FROM learners WHERE id = $1`, learnerID).Scan(&ic)
	return ic.String, err
}

// correctIC is the Admin changing a Learner's IC or passport number from a Course's Learners. Empty clears it,
// so the Learner can set it again.
func (a *App) correctIC(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.courseFor(w, r); !ok {
		return
	}
	learnerID, err := strconv.ParseInt(r.PathValue("learnerID"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ic := normaliseIC(r.PostFormValue("ic"))
	if ic != "" && !validIC.MatchString(ic) {
		a.render(w, r, http.StatusUnprocessableEntity, "message", page{Title: text.T(lang, "my-profile.ic"), Message: text.T(lang, "ic.invalid")})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE learners SET ic = NULLIF($1, '') WHERE id = $2`, ic, learnerID)
	if isUniqueViolation(err) {
		a.render(w, r, http.StatusConflict, "message", page{Title: text.T(lang, "ic.taken.title"), Message: text.T(lang, "ic.taken")})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.issueEarnedCertificates(r.Context(), learnerID, 0); err != nil {
		a.logError(r, fmt.Errorf("issue Certificates on correcting learner %d's IC: %w", learnerID, err))
	}
	http.Redirect(w, r, "/admin/courses/"+r.PathValue("id"), http.StatusSeeOther)
}
