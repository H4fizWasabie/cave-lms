package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

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
	Snippets   int
	Ebooks     bool      // Ebook access is running now
	EbookUntil time.Time // when it ends, after any periods that follow
	Lab        bool      // a period of the Lab Plan is running now
	LabUntil   time.Time // when it ends, after any periods that follow
}

// hasItem is one thing a Learner has: Label is the catalog key naming it, Until when it ends.
type hasItem struct {
	Label string
	Until time.Time
}

// Short is Until as "29 Oct 2027", the Malaysia date.
func (h hasItem) Short(lang string) string { return text.ShortDate(lang, h.Until.In(malaysia)) }

// Full is Until as "29 October 2027", the Malaysia date.
func (h hasItem) Full(lang string) string { return text.Date(lang, h.Until.In(malaysia)) }

// Has lists what the Learner has now, in the order Pro, Lab, Ebooks; empty for a free Learner.
func (r caveTextLearnerRow) Has() []hasItem {
	var has []hasItem
	if r.Pro {
		has = append(has, hasItem{"ctadmin.learners.has-pro", r.ProUntil})
	}
	if r.Lab {
		has = append(has, hasItem{"ctadmin.learners.has-lab", r.LabUntil})
	}
	if r.Ebooks {
		has = append(has, hasItem{"ctadmin.learners.has-ebooks", r.EbookUntil})
	}
	return has
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
	if p.Learners, err = a.caveTextLearners(r.Context(), 0); err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Unmatched, err = a.unmatchedPayments(r.Context()); err != nil {
		a.serverError(w, r, fmt.Errorf("list unmatched payments: %w", err))
		return
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_learners", p)
}

// runningUntilSQL is a scalar subquery over the periods p of a Learner (from names them and starts the WHERE) that were
// not ended early and end after now: when the last ends, or NULL unless one is running now. That is when what the
// Learner has ends, with any periods that follow.
func runningUntilSQL(from string) string {
	return `(SELECT CASE WHEN bool_or(p.starts_at <= now()) THEN max(p.ends_at) END FROM ` + from +
		` AND p.ends_at > now() AND p.ended_early_at IS NULL)`
}

// caveTextLearners lists the CaveText Learners in place order, with what each has; only the one with this id when id is
// not 0.
func (a *App) caveTextLearners(ctx context.Context, id int64) ([]caveTextLearnerRow, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT c.learner_id, c.place, l.name, l.email, `+snippetCapColumns+`,
			(SELECT count(*) FROM cavetext_snippets n WHERE n.learner_id = c.learner_id),
			`+runningUntilSQL(`cavetext_pro_periods p WHERE p.learner_id = c.learner_id AND p.ebooks`)+`,
			`+runningUntilSQL(`plan_periods p JOIN plans pl ON pl.id = p.plan_id WHERE p.learner_id = c.learner_id AND pl.lab`)+`
		FROM cavetext_accounts c JOIN learners l ON l.id = c.learner_id CROSS JOIN cavetext_settings s
		WHERE $1::bigint = 0 OR c.learner_id = $1
		ORDER BY c.place`, id)
	if err != nil {
		return nil, fmt.Errorf("list CaveText Learners: %w", err)
	}
	defer rows.Close()
	var list []caveTextLearnerRow
	for rows.Next() {
		var row caveTextLearnerRow
		var until, ebooks, lab sql.NullTime
		if err := rows.Scan(append(append([]any{&row.ID, &row.Place, &row.Name, &row.Email}, row.dest(&until)...), &row.Snippets, &ebooks, &lab)...); err != nil {
			return nil, err
		}
		row.setUntil(until)
		row.Ebooks, row.EbookUntil = ebooks.Valid, ebooks.Time.UTC()
		row.Lab, row.LabUntil = lab.Valid, lab.Time.UTC()
		list = append(list, row)
	}
	return list, rows.Err()
}

// caveTextLearner loads the CaveText Learner named by {id}. When there is none it writes the Not found page and
// returns false.
func (a *App) caveTextLearner(w http.ResponseWriter, r *http.Request) (caveTextLearnerRow, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var list []caveTextLearnerRow
	if err == nil && id > 0 {
		if list, err = a.caveTextLearners(r.Context(), id); err != nil {
			a.serverError(w, r, err)
			return caveTextLearnerRow{}, false
		}
	}
	if len(list) == 0 {
		a.adminNotFound(w, r, "ctadmin.learners.not-found.title", "ctadmin.learners.not-found")
		return caveTextLearnerRow{}, false
	}
	return list[0], true
}

// caveTextLearnerPath is the Manage page of the CaveText Learner with this id.
func caveTextLearnerPath(id int64) string {
	return "/admin/cavetext/learners/" + strconv.FormatInt(id, 10)
}

// giveKind is one thing the Admin gives a CaveText Learner: Action is its path segment, Heading the catalog key naming
// the give (also its confirm button), Product the key naming what is given. Plan adds a period of the Lab Plan and
// Ebooks adds Ebook access: the Pro Plan and the COMBO Pro Plan.
type giveKind struct {
	Action, Heading, Product string
	Plan, Ebooks             bool
}

var giveKinds = []giveKind{
	{"pro", "ctadmin.learners.give", "ctadmin.learners.product.pro", false, false},
	{"pro-plan", "ctadmin.learners.give-pro-plan", "ctadmin.learners.product.pro-plan", true, false},
	{"combo", "ctadmin.learners.give-combo", "ctadmin.learners.product.combo", true, true},
}

// giveEnd is when the gives would end for a Learner: the year of CaveText Pro starts now or when their Pro ends, the Lab
// Plan period (Lab invalid when no Plan is marked as the Lab Plan, LabDays long) starts now or when their Plan periods end.
type giveEnd struct {
	Pro     time.Time
	Lab     sql.NullTime
	LabDays int
}

// giveEnds reads giveEnd for the Learner, with the starts the gives use.
func (a *App) giveEnds(ctx context.Context, learnerID int64) (giveEnd, error) {
	var e giveEnd
	var days sql.NullInt64
	err := a.db.QueryRowContext(ctx, `
		SELECT (SELECT s.at + '`+proLength+`'::interval FROM `+nextProStart+`),
			(SELECT s.at + make_interval(days => p.days) FROM plans p, `+nextPlanStart+` WHERE p.lab),
			(SELECT p.days FROM plans p WHERE p.lab)`, learnerID).Scan(&e.Pro, &e.Lab, &days)
	e.Pro, e.LabDays = e.Pro.UTC(), int(days.Int64)
	return e, err
}

// What says in lang what the give would give, ending when e says: "a year of CaveText Pro, until 29 Oct 2027, and ...".
func (k giveKind) What(lang string, e giveEnd) string {
	d := func(t time.Time) string { return text.ShortDate(lang, t.In(malaysia)) }
	switch {
	case !k.Plan:
		return text.T(lang, "ctadmin.learners.what.pro", d(e.Pro))
	case k.Ebooks && e.Lab.Valid:
		return text.T(lang, "ctadmin.learners.what.combo", d(e.Pro), e.LabDays, d(e.Lab.Time), d(e.Pro))
	case k.Ebooks:
		return text.T(lang, "ctadmin.learners.what.combo-no-lab", d(e.Pro))
	case e.Lab.Valid:
		return text.T(lang, "ctadmin.learners.what.pro-plan", d(e.Pro), e.LabDays, d(e.Lab.Time))
	}
	return text.T(lang, "ctadmin.learners.what.pro-plan-no-lab", d(e.Pro))
}

// confirmGive asks the Admin to confirm giving a CaveText Learner k, naming what it gives and until when.
func (a *App) confirmGive(k giveKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, ok := a.caveTextLearner(w, r)
		if !ok {
			return
		}
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		ends, err := a.giveEnds(r.Context(), row.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.confirm(w, r, http.StatusOK, confirmation{Side: "cavetext", Heading: k.Heading, Question: "ctadmin.learners.give-ask", Solid: true,
			Cancel: caveTextLearnerPath(row.ID), Action: r.URL.Path}, row.Name, text.T(lang, k.Product), k.What(lang, ends))
	}
}

// caveTextLearnerPage is the Manage page of a CaveText Learner.
type caveTextLearnerPage struct {
	pageBase
	Row   caveTextLearnerRow
	Gives []giveView
}

// giveView is a give on the Manage page: a link to its confirmation page, and Says what it would give.
type giveView struct {
	giveKind
	Says string
}

// manageCaveTextLearner is the Manage page of a CaveText Learner: what they have, what the Admin can give them, the
// Ebook email and End Pro.
func (a *App) manageCaveTextLearner(w http.ResponseWriter, r *http.Request) {
	row, ok := a.caveTextLearner(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ends, err := a.giveEnds(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &caveTextLearnerPage{Row: row}
	for _, k := range giveKinds {
		p.Gives = append(p.Gives, giveView{k, k.What(lang, ends)})
	}
	a.render(w, r, http.StatusOK, "admin_cavetext_learner", p)
}

// giveCaveTextPro gives a CaveText Learner a year of CaveText Pro, as givePro does.
func (a *App) giveCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, true, func(id int64) error { return givePro(r.Context(), a.db, id, false) })
}

// giveCaveTextProPlan gives a CaveText Learner the Pro Plan, as giveProPlan does.
func (a *App) giveCaveTextProPlan(w http.ResponseWriter, r *http.Request) {
	a.giveCaveTextPlan(w, r, false)
}

// giveCaveTextCombo gives a CaveText Learner the COMBO Pro Plan: the Pro Plan with Ebook access (ADR 0020).
func (a *App) giveCaveTextCombo(w http.ResponseWriter, r *http.Request) {
	a.giveCaveTextPlan(w, r, true)
}

// emailCaveTextEbookLink sends a CaveText Learner the Ebook email again.
func (a *App) emailCaveTextEbookLink(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, false, func(id int64) error {
		a.emailEbookLink(r.Context(), id)
		return nil
	})
}

// giveCaveTextPlan gives a CaveText Learner the Pro Plan, with Ebook access when ebooks is set, in one transaction.
func (a *App) giveCaveTextPlan(w http.ResponseWriter, r *http.Request, ebooks bool) {
	a.changePro(w, r, true, func(id int64) error {
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
		if err == nil && ebooks {
			a.emailEbookLink(r.Context(), id)
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

// nextProStart is s.at, when the next CaveText Pro period of the Learner $1 starts: now, or when their last Pro period
// ends if that is later. givePro and giveEnds both start the new year there, so renewing early loses no days.
const nextProStart = `(SELECT GREATEST(now(), (
	SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavetext_pro_periods WHERE learner_id = $1
)) AS at) s`

// givePro gives the Learner a year of CaveText Pro and nothing else, starting now or when their running Pro ends,
// so renewing early loses no days. With ebooks, that year also gives Ebook access (ADR 0020).
// ponytail: an Admin and a payment giving Pro to one Learner at the same moment could overlap; fine at this volume.
func givePro(ctx context.Context, db execer, learnerID int64, ebooks bool) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at, ebooks)
		SELECT $1, s.at, s.at + '`+proLength+`'::interval, $2
		FROM `+nextProStart, learnerID, ebooks); err != nil {
		return fmt.Errorf("give Pro: %w", err)
	}
	return nil
}

// endCaveTextPro ends a Learner's Pro at once: the running period and any given to follow it. Their Plan periods
// are left as they are.
func (a *App) endCaveTextPro(w http.ResponseWriter, r *http.Request) {
	a.changePro(w, r, true, func(id int64) error {
		_, err := a.db.ExecContext(r.Context(), `
			UPDATE cavetext_pro_periods SET ended_early_at = now()
			WHERE learner_id = $1 AND ended_early_at IS NULL AND ends_at > now()`, id)
		return err
	})
}

// confirmEndCaveTextPro asks the Admin to confirm ending a Learner's Pro.
func (a *App) confirmEndCaveTextPro(w http.ResponseWriter, r *http.Request) {
	if row, ok := a.caveTextLearner(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Side: "cavetext", Heading: "ctadmin.learners.end", Question: "ctadmin.learners.end-ask",
			Cancel: caveTextLearnerPath(row.ID), Action: r.URL.Path}, row.Email)
	}
}

// changePro runs change for the CaveText Learner named by the {id} path value, then goes to their Manage page. With
// confirm set (a give or End Pro) the post must come from the change's confirmation page, else it does nothing.
func (a *App) changePro(w http.ResponseWriter, r *http.Request, confirm bool, change func(learnerID int64) error) {
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
	if confirm && unconfirmed(w, r) {
		return
	}
	if err := change(id); err != nil {
		a.serverError(w, r, fmt.Errorf("change CaveText Learner %d: %w", id, err))
		return
	}
	http.Redirect(w, r, caveTextLearnerPath(id), http.StatusSeeOther)
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
