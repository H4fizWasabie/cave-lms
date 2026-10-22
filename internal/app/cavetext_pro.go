package app

import (
	"context"
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
	Unmatched  []unmatchedPayment // Pro payments that gave no Pro, until an Admin marks them done
	EarlyTaken string             // "N of 1,000" in the page's language, from the early places setting
}

// adminCaveTextLearners lists every Learner who has signed in to CaveText, in place order.
func (a *App) adminCaveTextLearners(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &caveTextLearnersPage{}
	// Places are never reused, so a deleted Learner's early place stays taken: count from the place sequence.
	var taken, places int
	if err := a.db.QueryRowContext(r.Context(), `
		SELECT LEAST(s.early_places, CASE WHEN q.is_called THEN q.last_value ELSE q.last_value - 1 END), s.early_places
		FROM cavetext_settings s, cavetext_accounts_place_seq q`).Scan(&taken, &places); err != nil {
		a.serverError(w, r, fmt.Errorf("count early places: %w", err))
		return
	}
	p.EarlyTaken = text.T(lang, "ctadmin.learners.n-of", thousands(taken), thousands(places))
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
	if p.Unmatched, err = a.unmatchedPayments(r.Context()); err != nil {
		a.serverError(w, r, fmt.Errorf("list unmatched payments: %w", err))
		return
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_learners", p)
}

// giveCaveTextPro gives a CaveText Learner 12 months of Pro, and a period of the Pro Plan for the same dates. It starts
// now, or when their running Pro ends if that is later, so renewing early loses no days.
func (a *App) giveCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, func(id int64) error {
		return a.givePro(r.Context(), a.db, id)
	})
}

// giveProQuery adds 12 months of Pro for the Learner $1, from now or from when their running Pro ends, and a period
// of the Pro Plan with the same start and end. The Pro Plan's allowance is copied, so editing it never changes the
// period. Its rows affected is the Plan periods made: none when no Plan is marked as the Pro Plan.
// ponytail: an Admin and a payment giving Pro to one Learner at the same moment could overlap; fine at this volume.
const giveProQuery = `
	WITH pro AS (
		INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at)
		SELECT $1, s.at, s.at + '` + proLength + `'::interval FROM (SELECT GREATEST(now(), (
			SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavetext_pro_periods WHERE learner_id = $1
		)) AS at) s
		RETURNING learner_id, starts_at, ends_at)
	INSERT INTO plan_periods (learner_id, plan_id, starts_at, ends_at, deployments)
	SELECT pro.learner_id, p.id, pro.starts_at, pro.ends_at, p.deployments FROM pro, plans p WHERE p.pro`

// givePro gives the Learner a year of Pro and its Plan, as giveProQuery does. Without a Pro Plan it still gives Pro
// and writes a line to the System log, so the Admin sees the missed setting.
func (a *App) givePro(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, learnerID int64) error {
	res, err := db.ExecContext(ctx, giveProQuery, learnerID)
	if err != nil {
		return fmt.Errorf("give Pro: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		a.writeLog("error", fmt.Sprintf("Learner %d got CaveText Pro without a Plan period: no Plan is marked as the Pro Plan", learnerID), "", "")
	}
	return nil
}

// endCaveTextPro ends a Learner's Pro at once: the running period and any given to follow it. Their Plan periods
// are left as they are.
func (a *App) endCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, func(id int64) error {
		_, err := a.db.ExecContext(r.Context(), `
			UPDATE cavetext_pro_periods SET ended_early_at = now()
			WHERE learner_id = $1 AND ended_early_at IS NULL AND ends_at > now()`, id)
		return err
	})
}

// changePro runs change for the CaveText Learner named by the {id} path value, then goes back to the Learners page.
func (a *App) changePro(w http.ResponseWriter, r *http.Request, change func(learnerID int64) error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var found bool
	if err == nil {
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM cavetext_accounts WHERE learner_id = $1)`, id).Scan(&found); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if !found {
		a.adminNotFound(w, r, "ctadmin.learners.not-found.title", "ctadmin.learners.not-found")
		return
	}
	if err := change(id); err != nil {
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
	Price       string // in RM: RM25
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
