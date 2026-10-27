package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// accountRow is one account on the Admin Learners page: a Learner, or an Admin.
type accountRow struct {
	Name, Email string
	IsAdmin     bool
	Verified    bool // false until the Learner opens the link in their verification email
	Joined      time.Time
}

// JoinedOn is the day the account was made in lang, Malaysia time.
func (r accountRow) JoinedOn(lang string) string { return text.Date(lang, r.Joined.In(malaysia)) }

type accountsPage struct {
	pageBase
	Accounts []accountRow
}

// adminLearners lists every account, newest first.
// ponytail: no paging or search; add them when the list outgrows one page.
func (a *App) adminLearners(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT name, email, is_admin, verified_at IS NOT NULL, created_at FROM learners ORDER BY created_at DESC, id DESC`)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("list the accounts: %w", err))
		return
	}
	defer rows.Close()
	p := &accountsPage{}
	for rows.Next() {
		var row accountRow
		if err := rows.Scan(&row.Name, &row.Email, &row.IsAdmin, &row.Verified, &row.Joined); err != nil {
			a.serverError(w, r, fmt.Errorf("list the accounts: %w", err))
			return
		}
		p.Accounts = append(p.Accounts, row)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, fmt.Errorf("list the accounts: %w", err))
		return
	}
	a.render(w, r, http.StatusOK, "admin_learners", p)
}
