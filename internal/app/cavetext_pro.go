package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// caveTextSite is where the extension's own page links; it leads on to the Chrome Web Store.
const caveTextSite = "https://cavetext.cavelms.com"

// proLength is how long a CaveText Pro year lasts.
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

// giveCaveTextPro gives a CaveText Learner a year of CaveText Pro, as givePro does.
func (a *App) giveCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, func(id int64) error { return givePro(r.Context(), a.db, id, false) })
}

// giveCaveTextProPlan gives a CaveText Learner the Pro Plan, as giveProPlan does.
func (a *App) giveCaveTextProPlan(w http.ResponseWriter, r *http.Request) {
	a.giveCaveTextPlan(w, r, false)
}

// giveCaveTextCombo gives a CaveText Learner the COMBO Pro Plan: the Pro Plan with Ebook access (ADR 0020).
func (a *App) giveCaveTextCombo(w http.ResponseWriter, r *http.Request) {
	a.giveCaveTextPlan(w, r, true)
}

// giveCaveTextPlan gives a CaveText Learner the Pro Plan, with Ebook access when ebooks is set, in one transaction.
func (a *App) giveCaveTextPlan(w http.ResponseWriter, r *http.Request, ebooks bool) {
	a.changePro(w, r, func(id int64) error {
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		planned, err := giveProPlan(r.Context(), tx, id, ebooks)
		if err == nil {
			err = tx.Commit()
		}
		if err == nil && !planned {
			a.warnNoLabPlan(id)
		}
		return err
	})
}

// giveProPlan gives the Learner the Pro Plan: a year of CaveText Pro, as givePro does, and a period of the Lab Plan,
// as giveLabPlan does. With ebooks, that year of CaveText Pro gives Ebook access: the COMBO Pro Plan. planned is
// false when no Plan is marked as the Lab Plan, so only CaveText Pro was given: the caller writes warnNoLabPlan once
// its transaction has committed.
func giveProPlan(ctx context.Context, db execer, learnerID int64, ebooks bool) (planned bool, err error) {
	if err := givePro(ctx, db, learnerID, ebooks); err != nil {
		return false, err
	}
	return giveLabPlan(ctx, db, learnerID)
}

// warnNoLabPlan writes to the System log that the Learner got the Pro Plan's CaveText Pro without its Plan period,
// so the Admin sees the missed setting.
func (a *App) warnNoLabPlan(learnerID int64) {
	a.writeLog("error", fmt.Sprintf("Learner %d got CaveText Pro without a Plan period: no Plan is marked as the Lab Plan", learnerID), "", "")
}

// givePro gives the Learner a year of CaveText Pro and nothing else, starting now or when their running Pro ends,
// so renewing early loses no days. With ebooks, that year also gives Ebook access (ADR 0020).
// ponytail: an Admin and a payment giving Pro to one Learner at the same moment could overlap; fine at this volume.
func givePro(ctx context.Context, db execer, learnerID int64, ebooks bool) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at, ebooks)
		SELECT $1, s.at, s.at + '`+proLength+`'::interval, $2
		FROM (SELECT GREATEST(now(), (
			SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavetext_pro_periods WHERE learner_id = $1
		)) AS at) s`, learnerID, ebooks); err != nil {
		return fmt.Errorf("give Pro: %w", err)
	}
	return nil
}

// endCaveTextPro ends a Learner's Pro at once: the running period and any given to follow it. Their Plan periods
// are left as they are.
func (a *App) endCaveTextPro(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.caveTextLearnerEmail(w, r); !ok || unconfirmed(w, r) {
		return
	}
	a.changePro(w, r, func(id int64) error {
		_, err := a.db.ExecContext(r.Context(), `
			UPDATE cavetext_pro_periods SET ended_early_at = now()
			WHERE learner_id = $1 AND ended_early_at IS NULL AND ends_at > now()`, id)
		return err
	})
}

// confirmEndCaveTextPro asks the Admin to confirm ending a Learner's Pro.
func (a *App) confirmEndCaveTextPro(w http.ResponseWriter, r *http.Request) {
	if email, ok := a.caveTextLearnerEmail(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Side: "cavetext", Heading: "ctadmin.learners.end", Question: "ctadmin.learners.end-ask",
			Cancel: "/admin/cavetext", Action: r.URL.Path}, email)
	}
}

// caveTextLearnerEmail loads the email of the CaveText Learner named by {id}. When there is none it writes the Not
// found page and returns false.
func (a *App) caveTextLearnerEmail(w http.ResponseWriter, r *http.Request) (string, bool) {
	var email string
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT l.email FROM cavetext_accounts c JOIN learners l ON l.id = c.learner_id
			WHERE c.learner_id = $1`, id).Scan(&email)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
		a.adminNotFound(w, r, "ctadmin.learners.not-found.title", "ctadmin.learners.not-found")
		return "", false
	}
	if err != nil {
		a.serverError(w, r, err)
		return "", false
	}
	return email, true
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
		a.serverError(w, r, fmt.Errorf("change CaveText Learner %d: %w", id, err))
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
	ProUntil    string  // the day Pro ends, in the page's language
	Offers      []offer // CaveText Pro and the Pro Plan
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
	var places int
	if err := a.db.QueryRowContext(r.Context(), `SELECT early_places, early_cap, late_cap FROM cavetext_settings WHERE id = 1`).
		Scan(&places, &p.EarlyCap, &p.LateCap); err != nil {
		a.serverError(w, r, fmt.Errorf("read CaveText settings: %w", err))
		return
	}
	if p.Offers, err = a.offersOf(r.Context(), caveTextProProduct, proPlanProduct); err != nil {
		a.serverError(w, r, err)
		return
	}
	p.EarlyPlaces = thousands(places)
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
			for i := range p.Offers {
				p.Offers[i].Renew = p.Offers[i].Product == caveTextProProduct
			}
		}
	}
	a.render(w, r, http.StatusOK, "cavetext", p)
}
