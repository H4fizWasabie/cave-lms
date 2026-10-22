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
// period AND an Enrollment in a Course that includes the Lab, or being the Admin.
type labAccess struct {
	Admin       bool          // the Admin always has the Lab, and deploys without a Plan
	Period      *activePeriod // nil when the Learner has no active Plan period
	InLabCourse bool          // enrolled in at least one Course that includes the Lab
}

// Open reports whether the Learner may use the Lab: build, upload and deploy.
func (l labAccess) Open() bool { return l.Admin || l.Period != nil && l.InLabCourse }

// CountedPeriod is the Plan period a Deploy counts against: nil for the Admin, whose Deploys are unlimited.
func (l labAccess) CountedPeriod() *activePeriod {
	if l.Admin {
		return nil
	}
	return l.Period
}

// activePeriod is the Plan period a Learner is in now.
type activePeriod struct {
	ID              int64
	PlanName        string
	Ends            string // formatted as periodTime
	Deployments     int    // the period's allowance: how many Deployments may be awake at once
	Awake           int    // the Learner's awake Deployments
	DeploymentsLeft int    // the allowance less Awake; below zero after renewing onto a smaller allowance
}

// Full reports whether the Learner already has as many awake Deployments as the period allows.
func (p *activePeriod) Full() bool { return p.Awake >= p.Deployments }

// labAccessOf applies the Lab access rule to a Learner. Every Lab request goes through it.
func (a *App) labAccessOf(ctx context.Context, l *learner) (labAccess, error) {
	acc := labAccess{Admin: l.IsAdmin}
	var p activePeriod
	var ends time.Time
	err := a.db.QueryRowContext(ctx, `
		SELECT pp.id, p.name, pp.ends_at, pp.deployments,
			(SELECT count(*) FROM deployments dep WHERE dep.learner_id = pp.learner_id AND `+depAwake+`)
		FROM plan_periods pp JOIN plans p ON p.id = pp.plan_id
		WHERE pp.learner_id = $1 AND pp.starts_at <= now() AND pp.ends_at > now() AND pp.ended_early_at IS NULL
		ORDER BY pp.starts_at LIMIT 1`, l.ID).Scan(&p.ID, &p.PlanName, &ends, &p.Deployments, &p.Awake)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return acc, err
	default:
		p.Ends = ends.UTC().Format(periodTime)
		p.DeploymentsLeft = p.Deployments - p.Awake
		acc.Period = &p
	}
	err = a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM enrollments e JOIN courses c ON c.id = e.course_id
		WHERE e.learner_id = $1 AND c.includes_lab)`, l.ID).Scan(&acc.InLabCourse)
	return acc, err
}

// labPage is /lab: the demo for Visitors and Learners without access, the Lab itself for those with it.
type labPage struct {
	pageBase
	Access      labAccess
	PaymentLink string   // where Upgrade goes; empty shows "Contact us to upgrade"
	LabCourses  []course // open Courses that include the Lab, offered to a Learner with a Plan but no such Course
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
		switch {
		case p.Access.Open():
		case p.Access.Period == nil:
			// ponytail: Upgrade goes to the first Plan with a payment link; one button per Plan if the Admin sells several.
			err = a.db.QueryRowContext(r.Context(),
				`SELECT COALESCE((SELECT payment_link FROM plans WHERE payment_link <> '' ORDER BY id LIMIT 1), '')`).Scan(&p.PaymentLink)
		case !p.Access.InLabCourse:
			p.LabCourses, err = a.queryCourses(r, `SELECT `+courseColumns+` FROM courses
				WHERE includes_lab AND is_open ORDER BY start_date NULLS LAST, id`)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.render(w, r, http.StatusOK, "lab", p)
}
