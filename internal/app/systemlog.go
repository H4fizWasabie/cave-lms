package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// systemLogDays is how long a System log line is kept.
const systemLogDays = 30

// systemLogPage is how many lines /admin/log shows at a time.
const systemLogPage = 100

// serverError records err in the System log against the request that failed, then answers 500.
func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.logError(r, err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// logError records an error in the System log. r gives the method and path, never the query string,
// so tokens in links stay out of the log; it is nil for an error outside a request.
func (a *App) logError(r *http.Request, err error) {
	method, path := "", ""
	if r != nil {
		method, path = r.Method, r.URL.Path
	}
	a.writeLog("error", err.Error(), method, path)
}

// logInfo records a line about the site's own running, such as a start-up.
func (a *App) logInfo(format string, args ...any) {
	a.writeLog("info", fmt.Sprintf(format, args...), "", "")
}

// writeLog prints the line for journald, then stores it. The request may already be over, so the
// insert gets its own deadline; when the database is down the line survives in journald alone.
func (a *App) writeLog(level, message, method, path string) {
	if method != "" {
		log.Printf("%s %s: %s", method, path, message)
	} else {
		log.Print(message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := a.db.ExecContext(ctx, `INSERT INTO system_log (level, message, method, path) VALUES ($1, $2, $3, $4)`,
		level, message, method, path); err != nil {
		log.Printf("store System log line: %v", err)
	}
}

// pruneSystemLog deletes lines older than systemLogDays.
func (a *App) pruneSystemLog(ctx context.Context) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM system_log WHERE at < now() - make_interval(days => $1)`, systemLogDays)
	return err
}

// pruneDaily prunes the System log once a day until done is closed.
func (a *App) pruneDaily(done <-chan struct{}) {
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			if err := a.pruneSystemLog(context.Background()); err != nil {
				a.logError(nil, fmt.Errorf("prune System log: %w", err))
			}
		}
	}
}

// Started records a start-up, which marks each deploy, when the server begins listening on addr.
func (a *App) Started(addr string) {
	a.logInfo("Cave LMS started, listening on %s", addr)
}

// requireOwner guards an Owner page. Everyone else, other Admins included, gets the same 404 as an unknown
// address, so the page's existence is not revealed.
func (a *App) requireOwner(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if learner == nil || !learner.IsOwner {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}

type systemLogLine struct {
	At      string
	Level   string
	Message string
	Method  string
	Path    string
}

type systemLogView struct {
	pageBase
	Lines      []systemLogLine
	ErrorsOnly bool
	Page       int
	Newer      int // the page before, or 0 on the first
	Older      int // the page after, or 0 on the last
}

// systemLog lists the System log newest first, systemLogPage lines a page; ?level=error shows errors only.
func (a *App) systemLog(w http.ResponseWriter, r *http.Request) {
	v := &systemLogView{pageBase: pageBase{Title: "System log"}, ErrorsOnly: r.URL.Query().Get("level") == "error", Page: 1}
	// 30 days rarely fill more than a few pages; the cap keeps the OFFSET from overflowing.
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 1 && n <= 100000 {
		v.Page = n
	}
	levels := []string{"error", "info"}
	if v.ErrorsOnly {
		levels = levels[:1]
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT at, level, message, method, path FROM system_log WHERE level = ANY($1)
		ORDER BY at DESC, id DESC LIMIT $2 OFFSET $3`, levels, systemLogPage+1, (v.Page-1)*systemLogPage)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var l systemLogLine
		var at time.Time
		if err := rows.Scan(&at, &l.Level, &l.Message, &l.Method, &l.Path); err != nil {
			a.serverError(w, r, err)
			return
		}
		l.At = at.UTC().Format("2006-01-02 15:04:05 UTC")
		v.Lines = append(v.Lines, l)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(v.Lines) > systemLogPage {
		v.Lines, v.Older = v.Lines[:systemLogPage], v.Page+1
	}
	v.Newer = v.Page - 1
	a.render(w, r, http.StatusOK, "admin_log", v)
}
