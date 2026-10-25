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
	IC    string
	Typed string // what the Learner typed, kept when it is refused
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
	status, refusal, err := a.storeIC(r, learner.ID, r.PostFormValue("ic"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	switch refusal {
	case "":
		http.Redirect(w, r, "/my-profile", http.StatusSeeOther)
	case "ic.set-once":
		ic, err := a.learnerIC(r.Context(), learner.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, status, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner, Error: text.T(lang, refusal)}, IC: formatIC(ic)})
	default:
		a.render(w, r, status, "my_profile", &myProfilePage{pageBase: pageBase{Learner: learner, Error: text.T(lang, refusal)}, Typed: r.PostFormValue("ic")})
	}
}

// storeIC sets the Learner's IC or passport number, once, to what they typed, normalised, and issues any Certificate
// that was waiting for it. When it is refused it returns the response status and the catalog key of the reason:
// ic.invalid, ic.taken, or ic.set-once when the IC is already set.
func (a *App) storeIC(r *http.Request, learnerID int64, typed string) (status int, refusal string, err error) {
	ic := normaliseIC(typed)
	if !validIC.MatchString(ic) {
		return http.StatusUnprocessableEntity, "ic.invalid", nil
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE learners SET ic = $1 WHERE id = $2 AND ic IS NULL`, ic, learnerID)
	if isUniqueViolation(err) {
		return http.StatusConflict, "ic.taken", nil
	}
	if err != nil {
		return 0, "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, "", err
	}
	if n == 0 {
		return http.StatusConflict, "ic.set-once", nil
	}
	if err := a.issueEarnedCertificates(r.Context(), learnerID, 0); err != nil {
		a.logError(r, fmt.Errorf("issue Certificates on setting learner %d's IC: %w", learnerID, err))
	}
	return 0, "", nil
}

// examICPage is the IC step a Learner without an IC is sent to on starting an Exam: the IC typed twice.
type examICPage struct {
	pageBase
	ErrorField string // the id of the field Error is about
	Course     *course
	Exam       *exam
	IC, Again  string // what the Learner typed, kept when it is refused
}

// examICStep loads the IC step's page, with its Learner, Course and Exam. When the Learner already has an IC it sends
// them on to the Exam and returns false: the IC is set once, so the step only ever leads to the Exam.
func (a *App) examICStep(w http.ResponseWriter, r *http.Request) (*examICPage, bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	c, e, ok := a.publishedExamFor(w, r, learner)
	if !ok {
		return nil, false
	}
	// Only a Learner who could start the Exam reaches the step: anyone else goes back to the Course, saving nothing.
	st, err := a.examStatusFor(r.Context(), learner, c, e)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	if !st.CanStart() {
		http.Redirect(w, r, "/courses/"+strconv.FormatInt(c.ID, 10), http.StatusSeeOther)
		return nil, false
	}
	ic, err := a.learnerIC(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	if ic != "" {
		http.Redirect(w, r, e.Path(""), http.StatusSeeOther)
		return nil, false
	}
	return &examICPage{pageBase: pageBase{Learner: learner}, Course: &c, Exam: &e}, true
}

func (a *App) examIC(w http.ResponseWriter, r *http.Request) {
	if p, ok := a.examICStep(w, r); ok {
		a.render(w, r, http.StatusOK, "exam_ic", p)
	}
}

// setExamIC sets the IC from the IC step, with the same checks as My profile and the two entries matching, then
// goes on to the Exam the Learner was starting.
func (a *App) setExamIC(w http.ResponseWriter, r *http.Request) {
	p, ok := a.examICStep(w, r)
	if !ok {
		return
	}
	typed, again := r.PostFormValue("ic"), r.PostFormValue("ic-confirm")
	status, field, refusal := http.StatusUnprocessableEntity, "ic", ""
	var err error
	switch {
	case !validIC.MatchString(normaliseIC(typed)):
		refusal = "ic.invalid"
	case normaliseIC(typed) != normaliseIC(again):
		field, refusal = "ic-confirm", "ic.mismatch"
	default:
		if status, refusal, err = a.storeIC(r, p.Learner.ID, typed); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if refusal == "" || refusal == "ic.set-once" { // set once, here or in another tab
		http.Redirect(w, r, p.Exam.Path(""), http.StatusSeeOther)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.Error, p.ErrorField, p.IC, p.Again = text.T(lang, refusal), field, typed, again
	a.render(w, r, status, "exam_ic", p)
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
