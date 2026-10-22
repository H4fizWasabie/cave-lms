package app

import (
	"net/http"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// myMembershipPage is what the logged-in Learner holds: their Plan, CaveText Pro and Snippet cap.
type myMembershipPage struct {
	pageBase
	PlanName  string // empty when the Learner has no active Plan period
	PlanUntil string // the end of the last queued Plan period
	CaveText  bool   // has a CaveText account, so a Snippet cap
	Cap       snippetCap
	ProUntil  string
}

// myMembership shows the logged-in Learner their current Plan until its last queued period ends,
// their CaveText Pro and their Snippet cap.
func (a *App) myMembership(w http.ResponseWriter, r *http.Request) {
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
	p := &myMembershipPage{pageBase: pageBase{Learner: learner}}
	ctx := r.Context()
	acc, err := a.labAccessOf(ctx, learner)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if acc.Period != nil {
		var until time.Time
		// The next Plan period would start when the last one ends: with one active, that is after now.
		if err := a.db.QueryRowContext(ctx, `SELECT s.at FROM `+nextPlanStart, learner.ID).Scan(&until); err != nil {
			a.serverError(w, r, err)
			return
		}
		p.PlanName, p.PlanUntil = acc.Period.PlanName, text.Date(lang, until.UTC())
	}
	if err := a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)`, learner.ID).Scan(&p.CaveText); err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.CaveText {
		if p.Cap, err = learnerSnippetCap(ctx, a.db, learner.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		p.ProUntil = text.Date(lang, p.Cap.ProUntil)
	}
	a.render(w, r, http.StatusOK, "my_membership", p)
}
