package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"text/template/parse"
	"time"

	"github.com/pressly/goose/v3"
)

// MigrateDownTo undoes every migration after version, as if the database were from before them, for tests.
func MigrateDownTo(ctx context.Context, db *sql.DB, version int64) error {
	goose.SetBaseFS(appFiles)
	return goose.DownToContext(ctx, db, "migrations", version)
}

// PruneSystemLog runs the daily System log prune now, for tests.
func (a *App) PruneSystemLog(ctx context.Context) error { return a.pruneSystemLog(ctx) }

// RunDailyPrune runs the whole daily prune now, for tests.
func (a *App) RunDailyPrune(ctx context.Context) { a.dailyPrune(ctx) }

// CheckBCLToken runs the daily BCL API token check now, for tests.
func (a *App) CheckBCLToken(ctx context.Context) { a.checkBCLToken(ctx) }

// TemplateTextKeys lists every catalog key a template names in {{t .Lang "key"}} or {{tn .Lang "key" n}}, for tests.
func TemplateTextKeys() ([]string, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	var keys []string
	var walk func(parse.Node)
	walk = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, c := range n.Nodes {
					walk(c)
				}
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.IfNode:
			walk(&n.BranchNode)
		case *parse.RangeNode:
			walk(&n.BranchNode)
		case *parse.WithNode:
			walk(&n.BranchNode)
		case *parse.BranchNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.TemplateNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			if n != nil {
				for _, c := range n.Cmds {
					walk(c)
				}
			}
		case *parse.CommandNode:
			if id, ok := n.Args[0].(*parse.IdentifierNode); ok && (id.Ident == "t" || id.Ident == "tn") && len(n.Args) > 2 {
				if key, ok := n.Args[2].(*parse.StringNode); ok {
					keys = append(keys, key.Text)
				}
			}
			for _, c := range n.Args {
				walk(c)
			}
		}
	}
	for _, page := range pages {
		for _, t := range page.Templates() {
			if t.Tree != nil {
				walk(t.Tree.Root)
			}
		}
	}
	return keys, nil
}

// DBStats reports the app's database pool, for tests.
func (a *App) DBStats() sql.DBStats { return a.db.Stats() }

// SetMaxDBConns shrinks the database pool, for tests.
func (a *App) SetMaxDBConns(n int) { a.db.SetMaxOpenConns(n) }

// Survive runs one turn of a background loop the way the loops do, for tests.
func (a *App) Survive(job string, run func()) { a.survive(job, run) }

// RunReleaseResults runs the minutely Result release now, for tests.
func (a *App) RunReleaseResults(ctx context.Context) { a.releaseResults(ctx) }

// SetClock makes the app read the time from now, for tests.
func (a *App) SetClock(now func() time.Time) { a.now = now }

// Routes lists every registered pattern, such as "GET /admin/plans", for tests.
func (a *App) Routes() []string { return a.routes }

// Uploading counts the Learners with a submission upload in flight, for tests.
func (a *App) Uploading() int {
	n := 0
	a.uploading.Range(func(_, _ any) bool { n++; return true })
	return n
}

// HoldCleanUps takes n of the Clean-up turns until release is called, as if n uploads were being read, for tests.
func (a *App) HoldCleanUps(n int) (release func()) {
	for range n {
		a.cleaning <- struct{}{}
	}
	return func() {
		for range n {
			<-a.cleaning
		}
	}
}

// SetCleanUpWaitLog sets how long an upload may wait for its turn before the wait is logged, for tests.
func (a *App) SetCleanUpWaitLog(d time.Duration) { a.cleanUpWaitLog = d }

// SessionCookie is the name of the cookie that holds a session.
const SessionCookie = sessionCookie

// FragmentsBothWays renders Message id of Course courseID, or the Community Chat when it is 0, for the Learner behind
// each session token: shared as open Chat streams do when one loaded change serves them all, apart with messageFragment,
// for tests. A viewer the shared change refuses gets "denied" there.
func (a *App) FragmentsBothWays(courseID, id int64, tokens []string) (shared, apart []string, err error) {
	ch := &chatChange{courseID: courseID, id: id}
	views := make([]chatViewer, len(tokens))
	for i, token := range tokens {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		r.Header.Set("Accept-Language", "en")
		l, err := a.lookUpLearner(r)
		if err != nil || l == nil {
			return nil, nil, fmt.Errorf("session %d: %v", i, err)
		}
		lang, err := a.language(r)
		if err != nil {
			return nil, nil, err
		}
		views[i] = chatViewer{l, lang}
		ch.tokens = append(ch.tokens, hashToken(token))
	}
	for i, token := range tokens {
		html, ok, err := a.streamFragment(ch, &views[i], hashToken(token))
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			html = "denied"
		}
		shared = append(shared, html)
		html, err = a.messageFragment(context.Background(), views[i], courseID, id)
		if err != nil {
			return nil, nil, err
		}
		apart = append(apart, html)
	}
	return shared, apart, nil
}
