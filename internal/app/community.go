package app

import (
	"context"
	"net/http"
)

// community is the Community Chat's page: the Chat to a member or the Admin, else what it is and how to join it.
func (a *App) community(w http.ResponseWriter, r *http.Request) {
	l, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if l != nil {
		member, err := a.mayChat(r.Context(), l, 0)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if member {
			a.chat(w, r)
			return
		}
	}
	a.render(w, r, http.StatusOK, "community", page{Learner: l})
}

// communityMember reports whether Learner learnerID has joined the Community Chat.
func (a *App) communityMember(ctx context.Context, learnerID int64) (bool, error) {
	var member bool
	err := a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM community_members WHERE learner_id = $1)`, learnerID).Scan(&member)
	return member, err
}

// setMember returns the handler with which a Learner joins the Community Chat, or leaves it. Their Messages stay.
func (a *App) setMember(join bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		q := `DELETE FROM community_members WHERE learner_id = $1`
		if join {
			q = `INSERT INTO community_members (learner_id) VALUES ($1) ON CONFLICT DO NOTHING`
		}
		if _, err := a.db.ExecContext(r.Context(), q, l.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/community", http.StatusSeeOther)
	}
}
