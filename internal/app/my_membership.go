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
	if err == nil && acc.Period != nil {
		p.PlanName = acc.Period.PlanName
		var until time.Time
		// The next Plan period would start when the last one ends: with one active, that is after now.
		err = a.db.QueryRowContext(ctx, `SELECT s.at FROM `+nextPlanStart, learner.ID).Scan(&until)
		p.PlanUntil = text.Date(lang, until.UTC())
	}
	if err == nil {
		err = a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)`, learner.ID).Scan(&p.CaveText)
	}
	if err == nil && p.CaveText {
		p.Cap, err = learnerSnippetCap(ctx, a.db, learner.ID)
		p.ProUntil = text.Date(lang, p.Cap.ProUntil)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_membership", p)
}
