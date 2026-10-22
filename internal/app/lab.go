package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
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
	PlanName         string
	Ends             string // formatted as periodTime
	Deployments      int    // the period's allowance: how many Deployments may be awake at once
	Awake            int    // the Learner's awake Deployments
	storageAllowance        // the period's, copied from its Plan when it was given
	Used             int64  // bytes: the uploaded file sizes of the Learner's current Datasets
}

// DeploymentsLeft is how many more Deployments may be awake: never below zero, even after renewing onto a smaller
// allowance.
func (p *activePeriod) DeploymentsLeft() int { return max(p.Deployments-p.Awake, 0) }

// Full reports whether the Learner already has as many awake Deployments as the period allows.
func (p *activePeriod) Full() bool { return p.Awake >= p.Deployments }

// labAccessOf applies the Lab access rule to a Learner. Every Lab request goes through it.
func (a *App) labAccessOf(ctx context.Context, l *learner) (labAccess, error) {
	acc := labAccess{Admin: l.IsAdmin}
	var p activePeriod
	var ends time.Time
	err := a.db.QueryRowContext(ctx, `
		SELECT pp.id, p.name, pp.ends_at, pp.deployments, pp.storage_mb,
			(SELECT count(*) FROM deployments dep WHERE dep.learner_id = pp.learner_id AND `+deploymentAwakeSQL+`),
			(SELECT COALESCE(sum(dataset_size), 0) FROM dashboards WHERE learner_id = pp.learner_id)
		FROM plan_periods pp JOIN plans p ON p.id = pp.plan_id
		WHERE pp.learner_id = $1 AND `+activePeriodSQL+`
		ORDER BY pp.starts_at LIMIT 1`, l.ID).Scan(&p.ID, &p.PlanName, &ends, &p.Deployments, &p.StorageMB, &p.Awake, &p.Used)
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

// labPage is /lab: the demo for Visitors and Learners without access, the Lab itself for those with it.
type labPage struct {
	pageBase
	Access      labAccess
	PaymentLink string // the Lab Plan's, where Upgrade goes; empty shows "Contact us to upgrade"
	Dashboards  []dashboardRow
	Templates   []*lab.Template // previewed on their Samples in the demo
}

func (a *App) lab(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := &labPage{pageBase: pageBase{Learner: learner, Title: "Lab"}, Templates: lab.Templates}
	if learner != nil {
		if p.Access, err = a.labAccessOf(r.Context(), learner); err != nil {
			a.serverError(w, r, err)
			return
		}
		// Only a Learner who once had access has Dashboards, so without access they are listed read-only.
		if p.Dashboards, err = a.labDashboards(r, learner.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		if !p.Access.Open() {
			var lab price
			lab, err = a.priceOf(r.Context(), labPlanProduct)
			p.PaymentLink = lab.PaymentLink
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.render(w, r, http.StatusOK, "lab", p)
}
