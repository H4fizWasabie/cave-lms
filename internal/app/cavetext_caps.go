package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// snippetCap is how many Snippets a Learner may keep, and whether their place is early.
type snippetCap struct {
	Cap   int
	Early bool
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// learnerSnippetCap resolves a Learner's Snippet cap: a place within the early places gets the early cap, any later
// one the late cap. Every write and every page showing the cap goes through here, so a changed setting applies to
// every Learner at once.
func learnerSnippetCap(ctx context.Context, q rowQueryer, learnerID int64) (snippetCap, error) {
	var c snippetCap
	err := q.QueryRowContext(ctx, `
		SELECT c.place <= s.early_places, CASE WHEN c.place <= s.early_places THEN s.early_cap ELSE s.late_cap END
		FROM cavetext_accounts c CROSS JOIN cavetext_settings s WHERE c.learner_id = $1`, learnerID).Scan(&c.Early, &c.Cap)
	if err != nil {
		return c, fmt.Errorf("resolve Snippet cap: %w", err)
	}
	return c, nil
}

func countSnippets(ctx context.Context, q rowQueryer, learnerID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM cavetext_snippets WHERE learner_id = $1`, learnerID).Scan(&n)
	return n, err
}

// overCapMessage refuses a new Snippet to a Learner with have Snippets against a cap.
func overCapMessage(have int, c snippetCap) string {
	return fmt.Sprintf("You have %d of your %d free Snippets. Delete one to add another, or get CaveText Pro for unlimited Snippets.", have, c.Cap)
}

// skippedMessage says an import stopped at the cap.
func skippedMessage(kept, skipped int) string {
	rest := fmt.Sprintf("the other %d were skipped", skipped)
	if skipped == 1 {
		rest = "the other one was skipped"
	}
	snippets := "Snippets"
	if kept == 1 {
		snippets = "Snippet"
	}
	return fmt.Sprintf("Imported %d %s, your free cap; %s. CaveText Pro gives unlimited Snippets.", kept, snippets, rest)
}

type caveTextCapsView struct {
	pageBase
	EarlyPlaces, EarlyCap, LateCap string
}

// maxCapSetting bounds the three numbers, so a typo can't hand out a cap no one meant.
const maxCapSetting = 1000000

func (a *App) caveTextCapsForm(w http.ResponseWriter, r *http.Request) {
	v := &caveTextCapsView{pageBase: pageBase{Title: "CaveText caps"}}
	var places, early, late int
	if err := a.db.QueryRowContext(r.Context(), `SELECT early_places, early_cap, late_cap FROM cavetext_settings WHERE id = 1`).
		Scan(&places, &early, &late); err != nil {
		a.serverError(w, r, fmt.Errorf("read CaveText caps: %w", err))
		return
	}
	v.EarlyPlaces, v.EarlyCap, v.LateCap = strconv.Itoa(places), strconv.Itoa(early), strconv.Itoa(late)
	if r.URL.Query().Has("saved") {
		v.Message = "Saved."
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_caps", v)
}

// saveCaveTextCaps stores the early places, early cap and late cap; all three or none.
func (a *App) saveCaveTextCaps(w http.ResponseWriter, r *http.Request) {
	v := &caveTextCapsView{pageBase: pageBase{Title: "CaveText caps"},
		EarlyPlaces: strings.TrimSpace(r.PostFormValue("early_places")),
		EarlyCap:    strings.TrimSpace(r.PostFormValue("early_cap")),
		LateCap:     strings.TrimSpace(r.PostFormValue("late_cap"))}
	var n [3]int
	for i, s := range []string{v.EarlyPlaces, v.EarlyCap, v.LateCap} {
		x, err := strconv.Atoi(s)
		if err != nil || x < 0 || x > maxCapSetting {
			v.Error = "Enter whole numbers from 0 to 1,000,000."
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
