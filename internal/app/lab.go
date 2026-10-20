package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// labAccess is what the Lab access rule finds for one Learner: access needs an active Plan
// period AND an Enrollment in a Course that includes the Lab.
type labAccess struct {
	Period      *activePeriod // nil when the Learner has no active Plan period
	InLabCourse bool          // enrolled in at least one Course that includes the Lab
}

// Open reports whether the Learner may use the Lab: build, upload and deploy.
func (l labAccess) Open() bool { return l.Period != nil && l.InLabCourse }

// activePeriod is the Plan period a Learner is in now.
type activePeriod struct {
	ID              int64
	PlanName        string
	Ends            string // formatted as periodTime
	Deployments     int    // the period's allowance
	DeploymentsLeft int    // the allowance less Deployments counted against this period
}

// labAccessOf applies the Lab access rule to a Learner. Every Lab request goes through it.
func (a *App) labAccessOf(ctx context.Context, learnerID int64) (labAccess, error) {
	var acc labAccess
	var p activePeriod
	var ends time.Time
	err := a.db.QueryRowContext(ctx, `
		SELECT pp.id, p.name, pp.ends_at, pp.deployments,
			pp.deployments - (SELECT count(*) FROM deployments d WHERE d.period_id = pp.id) FROM plan_periods pp JOIN plans p ON p.id = pp.plan_id
		WHERE pp.learner_id = $1 AND pp.starts_at <= now() AND pp.ends_at > now() AND pp.ended_early_at IS NULL
		ORDER BY pp.starts_at LIMIT 1`, learnerID).Scan(&p.ID, &p.PlanName, &ends, &p.Deployments, &p.DeploymentsLeft)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return acc, err
	default:
		p.Ends = ends.UTC().Format(periodTime)
		acc.Period = &p
	}
	err = a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM enrollments e JOIN courses c ON c.id = e.course_id
		WHERE e.learner_id = $1 AND c.includes_lab)`, learnerID).Scan(&acc.InLabCourse)
	return acc, err
}

// labPage is /lab: the demo for Visitors and Learners without access, the Lab itself for those with it.
type labPage struct {
	pageBase
	Access      labAccess
	PaymentLink string   // where Upgrade goes; empty shows "Contact us to upgrade"
	LabCourses  []course // open Courses that include the Lab, offered to a Learner with a Plan but no such Course
	Dashboards  []dashboardRow
}

func (a *App) lab(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	p := &labPage{pageBase: pageBase{Learner: learner, Title: "Lab"}}
	if learner != nil {
		if p.Access, err = a.labAccessOf(r.Context(), learner.ID); err != nil {
			serverError(w, err)
			return
		}
		switch {
		case p.Access.Open():
			p.Dashboards, err = a.labDashboards(r, learner.ID)
		case p.Access.Period == nil:
			// ponytail: Upgrade goes to the first Plan with a payment link; one button per Plan if the Admin sells several.
			err = a.db.QueryRowContext(r.Context(),
				`SELECT COALESCE((SELECT payment_link FROM plans WHERE payment_link <> '' ORDER BY id LIMIT 1), '')`).Scan(&p.PaymentLink)
		case !p.Access.InLabCourse:
			p.LabCourses, err = a.queryCourses(r, `SELECT `+courseColumns+` FROM courses
				WHERE includes_lab AND is_open ORDER BY start_date NULLS LAST, id`)
		}
		if err != nil {
			serverError(w, err)
			return
		}
	}
	a.render(w, r, http.StatusOK, "lab", p)
}
