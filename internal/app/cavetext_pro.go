package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// caveTextSite is where the extension's own page links; it leads on to the Chrome Web Store.
const caveTextSite = "https://cavetext.wasabietech.com"

// proLength is how long CaveText Pro lasts from the day it starts.
const proLength = "12 months"

// caveTextLearnerRow is one CaveText Learner on the Admin CaveText Learners page.
type caveTextLearnerRow struct {
	ID    int64
	Place int64
	Name  string
	Email string
	snippetCap
	Snippets int
}

// Until is when the Learner's Pro ends, or empty without Pro.
func (r caveTextLearnerRow) Until() string {
	if !r.Pro {
		return ""
	}
	return r.ProUntil.Format(periodTime)
}

type caveTextLearnersPage struct {
	pageBase
	Learners   []caveTextLearnerRow
	EarlyTaken string // "N of 1,000", from the early places setting
}

// adminCaveTextLearners lists every Learner who has signed in to CaveText, in place order.
func (a *App) adminCaveTextLearners(w http.ResponseWriter, r *http.Request) {
	p := &caveTextLearnersPage{pageBase: pageBase{Title: "CaveText Learners"}}
	// Places are never reused, so a deleted Learner's early place stays taken: count from the place sequence.
	var taken, places int
	if err := a.db.QueryRowContext(r.Context(), `
		SELECT LEAST(s.early_places, CASE WHEN q.is_called THEN q.last_value ELSE q.last_value - 1 END), s.early_places
		FROM cavetext_settings s, cavetext_accounts_place_seq q`).Scan(&taken, &places); err != nil {
		a.serverError(w, r, fmt.Errorf("count early places: %w", err))
		return
	}
	p.EarlyTaken = thousands(taken) + " of " + thousands(places)
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT c.learner_id, c.place, l.name, l.email, `+snippetCapColumns+`,
			(SELECT count(*) FROM cavetext_snippets n WHERE n.learner_id = c.learner_id)
		FROM cavetext_accounts c JOIN learners l ON l.id = c.learner_id CROSS JOIN cavetext_settings s
		ORDER BY c.place`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var row caveTextLearnerRow
		var until sql.NullTime
		if err := rows.Scan(append(append([]any{&row.ID, &row.Place, &row.Name, &row.Email}, row.dest(&until)...), &row.Snippets)...); err != nil {
			a.serverError(w, r, err)
			return
		}
		row.setUntil(until)
		p.Learners = append(p.Learners, row)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_learners", p)
}

// giveCaveTextPro gives a CaveText Learner 12 months of Pro. It starts now, or when their running Pro ends if that is
// later, so renewing early loses no days.
func (a *App) giveCaveTextPro(w http.ResponseWriter, r *http.Request) {
	// ponytail: two Admins giving Pro to one Learner at once could overlap; fine while Pro is given by hand.
	a.changePro(w, r, `
		INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at)
		SELECT $1, s.at, s.at + '`+proLength+`'::interval FROM (SELECT GREATEST(now(), (
			SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavetext_pro_periods WHERE learner_id = $1
		)) AS at) s`)
}

// endCaveTextPro ends a Learner's Pro at once: the running period and any given to follow it.
func (a *App) endCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, `
		UPDATE cavetext_pro_periods SET ended_early_at = now()
		WHERE learner_id = $1 AND ended_early_at IS NULL AND ends_at > now()`)
}

// changePro runs query for the CaveText Learner named by the {id} path value, then goes back to the Learners page.
func (a *App) changePro(w http.ResponseWriter, r *http.Request, query string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var found bool
	if err == nil {
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)`, id).Scan(&found); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if !found {
		a.render(w, r, http.StatusNotFound, "message", page{Title: "Learner not found", Message: "No CaveText Learner has this address."})
		return
	}
	if _, err := a.db.ExecContext(r.Context(), query, id); err != nil {
		a.serverError(w, r, fmt.Errorf("change CaveText Pro: %w", err))
		return
	}
	http.Redirect(w, r, "/admin/cavetext", http.StatusSeeOther)
}

// thousands writes n with commas: 1,000.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// caveTextPage is /cavetext, the Learner's CaveText page: their cap and Pro, what Pro gives and how to buy it.
// A Visitor sees what Pro gives and is asked to log in.
type caveTextPage struct {
	pageBase
	Account     bool // the Learner has signed in to CaveText
	Cap         snippetCap
	Count       int
	ProUntil    string // the day Pro ends, in the page's language
	Price      string // in RM: RM25
	PaymentLink string // where Buy goes; empty asks the Learner to contact us
	EarlyPlaces string
	EarlyCap    int
	LateCap     int
	Extension   string
}

// OverCap reports whether a free Learner has more Snippets than their cap, as after Pro lapses.
func (p *caveTextPage) OverCap() bool { return !p.Cap.Pro && p.Count > p.Cap.Cap }

func (a *App) caveTextHome(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &caveTextPage{pageBase: pageBase{Learner: learner, Title: "CaveText"}, Extension: caveTextSite}
	var sen, places int
	if err := a.db.QueryRowContext(r.Context(), `SELECT pro_price_sen, pro_payment_link, early_places, early_cap, late_cap FROM cavetext_settings WHERE id = 1`).
		Scan(&sen, &p.PaymentLink, &places, &p.EarlyCap, &p.LateCap); err != nil {
		a.serverError(w, r, fmt.Errorf("read CaveText settings: %w", err))
		return
	}
	p.Price, p.EarlyPlaces = rm(sen), thousands(places)
	if learner != nil {
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)`, learner.ID).Scan(&p.Account); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if p.Account {
		if p.Cap, err = learnerSnippetCap(r.Context(), a.db, learner.ID); err == nil {
			p.Count, err = countSnippets(r.Context(), a.db, learner.ID)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if p.Cap.Pro {
			lang, err := a.language(r)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			p.ProUntil = text.Date(lang, p.Cap.ProUntil)
		}
	}
	a.render(w, r, http.StatusOK, "cavetext", p)
}
