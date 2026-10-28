package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// labAccess is what the Lab access rule finds for one Learner: access needs an active Plan
// period, or being the Admin.
type labAccess struct {
	Admin  bool          // the Admin always has the Lab, and deploys without a Plan
	Period *activePeriod // nil when the Learner has no active Plan period
}

// Open reports whether the Learner may use the Lab: build, upload and deploy.
func (l labAccess) Open() bool { return l.Admin || l.Period != nil }

// CountedPeriod is the Plan period a Deploy counts against: nil for the Admin, whose Deploys are unlimited.
func (l labAccess) CountedPeriod() *activePeriod {
	if l.Admin {
		return nil
	}
	return l.Period
}

// activePeriodSQL is an SQL condition, true for a period aliased pp that is running now: started, not ended and not
// ended early. Plan periods and CaveText Pro periods share these columns.
const activePeriodSQL = `pp.starts_at <= now() AND pp.ends_at > now() AND pp.ended_early_at IS NULL`

// activePeriod is the Plan period a Learner is in now.
type activePeriod struct {
	ID               int64
	Ends             string // formatted as periodTime
	Deployments      int    // the period's allowance: how many Deployments may be awake at once
	Awake            int    // the Learner's awake Deployments
	storageAllowance        // the period's, copied from its Plan when it was given
	Used             int64  // bytes the Learner's Parts, pages and Workbooks take: see storageUsedSQL
}

// DeploymentsLeft is how many more Deployments may be awake: never below zero, even after renewing onto a smaller
// allowance.
func (p *activePeriod) DeploymentsLeft() int { return max(p.Deployments-p.Awake, 0) }

// Full reports whether the Learner already has as many awake Deployments as the period allows.
func (p *activePeriod) Full() bool { return p.Awake >= p.Deployments }

// storageUsedSQL is an SQL expression for the bytes a Learner's Parts, pages and Workbooks take against the Storage
// allowance, learner being an SQL expression for their id: their Dashboards' current Parts (and any earlier Dataset
// from before Parts, until their Dashboard's data next changes) by saved size, their pages and their Workbooks.
// Submissions do not count. Pending Parts count only when pendingReserve or keepWorkbook checks an upload.
func storageUsedSQL(learner string) string {
	return `((SELECT COALESCE(sum(ds.saved_size), 0) FROM dataset_parts ds JOIN dashboards owner ON owner.id = ds.dashboard_id
			WHERE owner.learner_id = ` + learner + ` AND ds.state <> 'pending') +
		(SELECT COALESCE(sum(pg.size), 0) FROM dashboard_pages pg JOIN dashboards pg_owner ON pg_owner.id = pg.dashboard_id
			WHERE pg_owner.learner_id = ` + learner + `) +
		(SELECT COALESCE(sum(wb.size), 0) FROM workbooks wb WHERE wb.learner_id = ` + learner + `))`
}

// labAccessOf applies the Lab access rule to a Learner. Every Lab request goes through it.
func (a *App) labAccessOf(ctx context.Context, l *learner) (labAccess, error) {
	acc := labAccess{Admin: l.IsAdmin}
	var p activePeriod
	var ends time.Time
	err := a.db.QueryRowContext(ctx, `
		SELECT pp.id, pp.ends_at, pp.deployments, pp.storage_mb,
			(SELECT count(*) FROM deployments dep WHERE dep.learner_id = pp.learner_id AND `+deploymentAwakeSQL+`),
			`+storageUsedSQL("pp.learner_id")+`
		FROM plan_periods pp
		WHERE pp.learner_id = $1 AND `+activePeriodSQL+`
		ORDER BY pp.starts_at LIMIT 1`, l.ID).Scan(&p.ID, &ends, &p.Deployments, &p.StorageMB, &p.Awake, &p.Used)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return acc, err
	default:
		p.Ends = ends.UTC().Format(periodTime)
		acc.Period = &p
	}
	return acc, nil
}

// labPage is /lab: the Learner's Plan and Dashboards.
type labPage struct {
	pageBase
	Access     labAccess
	Dashboards []dashboardRow
}

// lab is the Lab for those it is open to; Visitors and Learners without a Plan go to /pricing (ADR 0018).
func (a *App) lab(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &labPage{pageBase: pageBase{Learner: learner, Title: "Lab"}}
	if learner != nil {
		if p.Access, err = a.labAccessOf(r.Context(), learner); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if !p.Access.Open() {
		http.Redirect(w, r, "/pricing", http.StatusSeeOther)
		return
	}
	if p.Dashboards, err = a.labDashboards(r, learner.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "lab", p)
}
