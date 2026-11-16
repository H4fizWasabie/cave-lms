package app

import (
	"net/http"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// myMembershipPage is what the logged-in Learner holds: their Plan, CaveText Pro and Snippet cap.
type myMembershipPage struct {
	pageBase
	PlanUntil string // the end of the last queued Plan period; empty when the Learner has no active Plan period
	CaveText  bool   // has a CaveText account, so a Snippet cap
	Cap       snippetCap
	ProUntil  string

	// A Teacher's Teachers Portal standing (ADR 0029), kept to accounts with a Teacher profile or Teachers Portal Pro.
	TrialUntil       string // the Teacher trial's end, while it has not ended
	TeachersProUntil string // the end of the last Teachers Portal Pro period, running or queued after the trial
	TeacherLapsed    bool   // the trial and Teachers Portal Pro have both ended
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
		p.PlanUntil = text.Date(lang, until.In(malaysia))
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
		p.ProUntil = text.Date(lang, p.Cap.ProUntil.In(malaysia))
	}
	teacher, err := a.teacherAccessOf(ctx, learner)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if teacher.State != teacherNone && teacher.State != teacherAdmin {
		if teacher.TrialEnds.After(time.Now()) {
			p.TrialUntil = text.Date(lang, teacher.TrialEnds.In(malaysia))
		}
		if !teacher.ProUntil.IsZero() {
			p.TeachersProUntil = text.Date(lang, teacher.ProUntil.In(malaysia))
		}
		p.TeacherLapsed = teacher.Lapsed()
	}
	a.render(w, r, http.StatusOK, "my_membership", p)
}
