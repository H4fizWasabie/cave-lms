package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// maxCaveHostDays is the longest CaveHost Plan period the Admin can assign: ten years, well inside what a timestamp holds.
const maxCaveHostDays = 3650

// maxCaveHostDeployments is how many CaveHost Deployments a Learner may have (ADR 0030); the Admin has no cap.
const maxCaveHostDeployments = 3

// caveHostOpen reports whether CaveHost is open to the Learner: a CaveHost Plan period is running, or they are the Admin,
// who uses CaveHost without one. A Visitor (nil) has none. The Lab's Plan gives no CaveHost, and this gives no Lab.
func (a *App) caveHostOpen(ctx context.Context, l *learner) (bool, error) {
	if l == nil {
		return false, nil
	}
	if l.IsAdmin {
		return true, nil
	}
	var open bool
	err := a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = $1 AND `+
		activePeriodSQL+`)`, l.ID).Scan(&open)
	return open, err
}

// caveHostPage is /cavehost. Locked is set for a Learner without a CaveHost Plan: it says CaveHost comes with the Pro Plan.
// Otherwise it lists the Learner's Deployments under the form that links a Spreadsheet, with the values of one it refused.
type caveHostPage struct {
	pageBase
	Locked      bool
	Deployments []deployment
	Prefix      string // the start of every link of the Learner's, for the name's hint
	Link, Name  string // the form's values
}

// caveHost is CaveHost for those it is open to. A Visitor logs in first; a Learner without a CaveHost Plan is told CaveHost
// comes with the Pro Plan (ADR 0030). Every CaveHost screen goes through this.
func (a *App) caveHost(h http.HandlerFunc) http.HandlerFunc {
	return a.requireLearner(func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		open, err := a.caveHostOpen(r.Context(), learner)
		switch {
		case err != nil:
			a.serverError(w, r, err)
		case !open:
			a.render(w, r, http.StatusForbidden, "cavehost", &caveHostPage{Locked: true})
		default:
			h(w, r)
		}
	})
}

// caveHostPeriod is one CaveHost Plan period of a Learner, as the Admin sees it.
type caveHostPeriod struct {
	ID           int64
	Starts, Ends string
	Status       string // active, upcoming, ended or ended-early
	Shown        string // Status as the page says it, with when it ended early
}

// nextCaveHostStart is s.at, when the next CaveHost Plan period of the Learner $1 starts: now, or when their last one ends
// if that is later, the same stacking as nextPlanStart.
const nextCaveHostStart = `(SELECT GREATEST(now(), (
	SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavehost_plan_periods WHERE learner_id = $1
)) AS at) s`

// giveCaveHostPlan gives the Learner a CaveHost Plan period of this many days, queued after nextCaveHostStart.
func giveCaveHostPlan(ctx context.Context, db execer, learnerID int64, days int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO cavehost_plan_periods (learner_id, starts_at, ends_at)
		SELECT $1, s.at, s.at + make_interval(days => $2) FROM `+nextCaveHostStart, learnerID, days)
	if err != nil {
		return fmt.Errorf("give CaveHost Plan: %w", err)
	}
	return nil
}

// giveCaveHostLikeLabPlan gives the Learner a CaveHost Plan period as long as the Lab Plan's, queued after nextCaveHostStart.
// Only giveProPlan calls it, once giveLabPlan has found a Lab Plan.
func giveCaveHostLikeLabPlan(ctx context.Context, db execer, learnerID int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO cavehost_plan_periods (learner_id, starts_at, ends_at)
		SELECT $1, s.at, s.at + make_interval(days => p.days) FROM plans p, `+nextCaveHostStart+` WHERE p.lab`, learnerID)
	if err != nil {
		return fmt.Errorf("give CaveHost Plan with the Pro Plan: %w", err)
	}
	return nil
}

// assignCaveHostPlan gives a Learner a CaveHost Plan period of the days asked for. It starts now, or when the Learner's last
// CaveHost Plan period ends if that is later.
func (a *App) assignCaveHostPlan(w http.ResponseWriter, r *http.Request) {
	p, err := a.planPeriodsPage(r, r.PostFormValue("email"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.HolderID == 0 {
		p.Error = text.T(p.Lang, "plans.err.no-learner", p.Email)
		a.render(w, r, http.StatusUnprocessableEntity, "admin_plan_periods", p)
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("days")))
	if err != nil || days < 1 || days > maxCaveHostDays {
		p.Error = text.T(p.Lang, "cavehost.err.days")
		a.render(w, r, http.StatusUnprocessableEntity, "admin_plan_periods", p)
		return
	}
	// ponytail: two Admin assignments racing for one Learner could overlap; fine while one Admin assigns by hand.
	if err := giveCaveHostPlan(r.Context(), a.db, p.HolderID, days); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, planPeriodsPath(p.Email), http.StatusSeeOther)
}

// endCaveHostPlanPeriod ends a CaveHost Plan period now. A period that has already ended is left as it is.
func (a *App) endCaveHostPlanPeriod(w http.ResponseWriter, r *http.Request) {
	id, email, ok := a.caveHostPeriodFor(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE cavehost_plan_periods SET ended_early_at = now() WHERE id = $1 AND ended_early_at IS NULL AND ends_at > now()`, id); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, planPeriodsPath(email), http.StatusSeeOther)
}

// confirmEndCaveHostPlanPeriod asks the Admin to confirm ending a CaveHost Plan period now.
func (a *App) confirmEndCaveHostPlanPeriod(w http.ResponseWriter, r *http.Request) {
	if _, email, ok := a.caveHostPeriodFor(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Side: "plans", Heading: "cavehost.period.end", Question: "cavehost.period.end-ask",
			Cancel: planPeriodsPath(email), Action: r.URL.Path}, email)
	}
}

// caveHostPeriodFor loads the CaveHost Plan period named by {id} and its Learner's email. When there is none it writes the
// Not found page and returns false.
func (a *App) caveHostPeriodFor(w http.ResponseWriter, r *http.Request) (id int64, email string, ok bool) {
	id, _ = strconv.ParseInt(r.PathValue("id"), 10, 64)
	err := a.db.QueryRowContext(r.Context(), `SELECT l.email FROM cavehost_plan_periods pp
		JOIN learners l ON l.id = pp.learner_id WHERE pp.id = $1`, id).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		a.adminNotFound(w, r, "cavehost.period.not-found.title", "cavehost.period.not-found")
		return 0, "", false
	}
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", false
	}
	return id, email, true
}

// caveHostPeriods lists the Learner's CaveHost Plan periods in start order, with their status and times in lang.
func (a *App) caveHostPeriods(ctx context.Context, lang string, learnerID int64) ([]caveHostPeriod, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT pp.id, pp.starts_at, pp.ends_at, pp.ended_early_at,
			CASE WHEN pp.ended_early_at IS NOT NULL THEN 'ended-early'
				WHEN pp.ends_at <= now() THEN 'ended'
				WHEN pp.starts_at > now() THEN 'upcoming'
				ELSE 'active' END
		FROM cavehost_plan_periods pp WHERE pp.learner_id = $1 ORDER BY pp.starts_at, pp.id`, learnerID)
	if err != nil {
		return nil, fmt.Errorf("list CaveHost Plan periods: %w", err)
	}
	defer rows.Close()
	var list []caveHostPeriod
	for rows.Next() {
		var pp caveHostPeriod
		var starts, ends time.Time
		var endedEarly sql.NullTime
		if err := rows.Scan(&pp.ID, &starts, &ends, &endedEarly, &pp.Status); err != nil {
			return nil, err
		}
		pp.Starts, pp.Ends = dateTimeIn(lang, starts), dateTimeIn(lang, ends)
		pp.Shown = text.T(lang, "plans.period."+pp.Status)
		if endedEarly.Valid {
			pp.Shown = text.T(lang, "plans.period.ended-early-on", dateTimeIn(lang, endedEarly.Time))
		}
		list = append(list, pp)
	}
	return list, rows.Err()
}
