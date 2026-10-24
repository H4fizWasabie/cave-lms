package app

import (
	"net/http"
	"strconv"
	"time"
)

// chatReport is an open Report as the Admin's Reports page lists it.
type chatReport struct {
	ID, MessageID, CourseID  int64
	Course, Writer, Reporter string
	Text                     string // the Message's, empty when its writer deleted it
	Reason                   string
	At                       time.Time
	Lang                     string
}

// Page is where the Chat page that ends with the Message starts paging back from.
func (r chatReport) Page() int64 { return r.MessageID + 1 }

// When is the time the Report was made, Malaysia time.
func (r chatReport) When() string { return dateTimeIn(r.Lang, r.At) }

// chatReportsPage is the Admin's Reports page.
type chatReportsPage struct {
	pageBase
	Reports []chatReport
}

// adminReports is the Admin's Reports page: open Reports, newest first.
// ponytail: lists every open Report on one page; add paging if they ever pile up.
func (a *App) adminReports(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT rp.id, m.id, c.id, c.title, wl.name, rl.name, m.body, rp.reason, rp.created_at
		FROM chat_reports rp JOIN chat_messages m ON m.id = rp.message_id JOIN courses c ON c.id = m.course_id
		JOIN learners wl ON wl.id = m.learner_id JOIN learners rl ON rl.id = rp.learner_id
		WHERE rp.closed_at IS NULL ORDER BY rp.created_at DESC, rp.id DESC`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	var p chatReportsPage
	for rows.Next() {
		rp := chatReport{Lang: lang}
		if err := rows.Scan(&rp.ID, &rp.MessageID, &rp.CourseID, &rp.Course, &rp.Writer, &rp.Reporter, &rp.Text, &rp.Reason, &rp.At); err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Reports = append(p.Reports, rp)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_reports", &p)
}

// dismissReport is the Admin closing one Report, leaving its Message as it is.
func (a *App) dismissReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE chat_reports SET closed_at = now() WHERE id = $1 AND closed_at IS NULL`, id); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/reports", http.StatusSeeOther)
}
