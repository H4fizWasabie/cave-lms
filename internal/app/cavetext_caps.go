package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// snippetCap is how many Snippets a Learner may keep, and whether their place is early. With an active CaveText Pro
// period the cap is unlimited; Cap stays the free cap their place gives, the one they return to when Pro lapses.
type snippetCap struct {
	Cap      int
	Early    bool
	Pro      bool
	ProUntil time.Time // when Pro, the end of the last period in the unbroken run of Pro periods
}

// Full reports whether a Learner with have Snippets may not add another.
func (c snippetCap) Full(have int) bool { return !c.Pro && have >= c.Cap }

// Keep is how many of n Snippets an import keeps.
func (c snippetCap) Keep(n int) int {
	if c.Pro {
		return n
	}
	return min(n, c.Cap)
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// execer is a *sql.DB or a *sql.Tx, for writes that may run inside a caller's transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// learnerSnippetCap resolves a Learner's Snippet cap: an active CaveText Pro period gives unlimited Snippets; otherwise
// a place within the early places gets the early cap, any later one the late cap. Every write and every page showing
// the cap goes through here, so a changed setting or a lapsed Pro applies at once.
func learnerSnippetCap(ctx context.Context, q rowQueryer, learnerID int64) (snippetCap, error) {
	var c snippetCap
	var until sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT `+snippetCapColumns+`
		FROM cavetext_accounts c CROSS JOIN cavetext_settings s WHERE c.learner_id = $1`, learnerID).Scan(c.dest(&until)...)
	if err != nil {
		return c, fmt.Errorf("resolve Snippet cap: %w", err)
	}
	c.setUntil(until)
	return c, nil
}

// snippetCapColumns is the one definition of an early place and an active CaveText Pro: four columns over
// cavetext_accounts c and cavetext_settings s, scanned with snippetCap.dest. learnerSnippetCap and the Admin
// CaveText Learners page both select it, so they can't disagree.
const snippetCapColumns = `c.place <= s.early_places, CASE WHEN c.place <= s.early_places THEN s.early_cap ELSE s.late_cap END,
	EXISTS (SELECT 1 FROM cavetext_pro_periods p WHERE p.learner_id = c.learner_id
		AND p.starts_at <= now() AND p.ends_at > now() AND p.ended_early_at IS NULL),
	(SELECT max(p.ends_at) FROM cavetext_pro_periods p WHERE p.learner_id = c.learner_id
		AND p.ends_at > now() AND p.ended_early_at IS NULL)`

// dest is where the snippetCapColumns scan to; pass until to setUntil after the scan.
func (c *snippetCap) dest(until *sql.NullTime) []any { return []any{&c.Early, &c.Cap, &c.Pro, until} }

func (c *snippetCap) setUntil(until sql.NullTime) {
	if c.Pro {
		c.ProUntil = until.Time.UTC()
	}
}

func countSnippets(ctx context.Context, q rowQueryer, learnerID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM cavetext_snippets WHERE learner_id = $1`, learnerID).Scan(&n)
	return n, err
}

// overCapMessage refuses a new Snippet to a Learner with have Snippets against a cap.
func overCapMessage(lang string, have int, c snippetCap) string {
	return text.T(lang, "cavetext.api.cap-reached", have, c.Cap)
}

// skippedMessage says an import stopped at the cap.
func skippedMessage(lang string, kept, skipped int) string {
	rest := text.T(lang, "cavetext.api.skipped-many", skipped)
	if skipped == 1 {
		rest = text.T(lang, "cavetext.api.skipped-one")
	}
	return text.T(lang, "cavetext.api.imported", text.N(lang, "cavetext.snippet-count", kept), rest)
}

type caveTextCapsView struct {
	pageBase
	EarlyPlaces, EarlyCap, LateCap string
}

// maxCapSetting bounds the three numbers, so a typo can't hand out a cap no one meant.
const maxCapSetting = 1000000

func (a *App) caveTextCapsForm(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	v := &caveTextCapsView{}
	var places, early, late int
	if err := a.db.QueryRowContext(r.Context(), `SELECT early_places, early_cap, late_cap FROM cavetext_settings WHERE id = 1`).
		Scan(&places, &early, &late); err != nil {
		a.serverError(w, r, fmt.Errorf("read CaveText caps: %w", err))
		return
	}
	v.EarlyPlaces, v.EarlyCap, v.LateCap = strconv.Itoa(places), strconv.Itoa(early), strconv.Itoa(late)
	if r.URL.Query().Has("saved") {
		v.Message = text.T(lang, "admin.common.saved")
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_caps", v)
}

// saveCaveTextCaps stores the early places, early cap and late cap; all three or none.
func (a *App) saveCaveTextCaps(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	v := &caveTextCapsView{
		EarlyPlaces: strings.TrimSpace(r.PostFormValue("early_places")),
		EarlyCap:    strings.TrimSpace(r.PostFormValue("early_cap")),
		LateCap:     strings.TrimSpace(r.PostFormValue("late_cap"))}
	var n [3]int
	for i, s := range []string{v.EarlyPlaces, v.EarlyCap, v.LateCap} {
		x, err := strconv.Atoi(s)
		if err != nil || x < 0 || x > maxCapSetting {
			v.Error = text.T(lang, "ctadmin.caps.err")
			a.render(w, r, http.StatusUnprocessableEntity, "admin_cavetext_caps", v)
			return
		}
		n[i] = x
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavetext_settings SET early_places = $1, early_cap = $2, late_cap = $3 WHERE id = 1`,
		n[0], n[1], n[2]); err != nil {
		a.serverError(w, r, fmt.Errorf("save CaveText caps: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext/caps?saved", http.StatusSeeOther)
}
