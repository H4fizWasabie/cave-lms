package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
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
	Ends             time.Time // shown with dateTime
	Deployments      int       // the period's allowance: how many Deployments may be awake at once
	Awake            int       // the Learner's awake Deployments
	storageAllowance           // the period's, copied from its Plan when it was given
	Used             int64     // bytes the Learner keeps against the allowance: see storageUsedSQL
}

// DeploymentsLeft is how many more Deployments may be awake: never below zero, even after renewing onto a smaller
// allowance.
func (p *activePeriod) DeploymentsLeft() int { return max(p.Deployments-p.Awake, 0) }

// Full reports whether the Learner already has as many awake Deployments as the period allows.
func (p *activePeriod) Full() bool { return p.Awake >= p.Deployments }

// labMemoKey keys the per-request memo of Lab access that ServeHTTP installs.
type labMemoKey struct{}

// labMemo holds what a request has read of Lab access, by Learner.
type labMemo struct {
	mu  sync.Mutex
	acc map[int64]labAccess
}

// forgetLabAccess drops what the request read of Lab access, after a save has changed the storage used.
func forgetLabAccess(ctx context.Context) {
	if m, ok := ctx.Value(labMemoKey{}).(*labMemo); ok {
		m.mu.Lock()
		clear(m.acc)
		m.mu.Unlock()
	}
}

// labAccessOf applies the Lab access rule to a Learner. Every Lab request goes through it, and reads the access once:
// later calls in the request get the same answer until a save calls forgetLabAccess. A failed read is not kept.
// A Visitor (nil) has no access.
func (a *App) labAccessOf(ctx context.Context, l *learner) (labAccess, error) {
	if l == nil {
		return labAccess{}, nil
	}
	m, ok := ctx.Value(labMemoKey{}).(*labMemo)
	if !ok {
		return a.readLabAccess(ctx, l)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if acc, ok := m.acc[l.ID]; ok {
		return acc, nil
	}
	acc, err := a.readLabAccess(ctx, l)
	if err == nil {
		m.acc[l.ID] = acc
	}
	return acc, err
}

func (a *App) readLabAccess(ctx context.Context, l *learner) (labAccess, error) {
	a.labReads.Add(1)
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
		p.Ends = ends
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
	if p.Access, err = a.labAccessOf(r.Context(), learner); err != nil {
		a.serverError(w, r, err)
		return
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
