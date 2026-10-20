package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// deployment is a Dashboard's Deployment as its owner sees it on the Deploy tab.
type deployment struct {
	Path      string // /d/<slug>/
	Link      string // the full link to share
	Deployed  string // formatted as periodTime
	Until     string // when it sleeps, slept or was taken down
	Awake     bool
	TakenDown bool
	Reason    string // why Cave took it down
}

// deploymentColumns select a deployment from deployments aliased dep; scanDeployment reads them.
const deploymentColumns = `dep.slug, dep.deployed_at, LEAST(dep.sleeps_at, COALESCE(dep.taken_down_at, dep.sleeps_at)) AS until,
	dep.sleeps_at > now() AND dep.taken_down_at IS NULL AS awake, dep.taken_down_at IS NOT NULL, dep.takedown_reason`

// scanDeployment scans a row whose columns are lead's targets followed by deploymentColumns.
func (a *App) scanDeployment(row interface{ Scan(...any) error }, lead ...any) (deployment, error) {
	var dep deployment
	var slug string
	var deployed, until time.Time
	if err := row.Scan(append(lead, &slug, &deployed, &until, &dep.Awake, &dep.TakenDown, &dep.Reason)...); err != nil {
		return dep, err
	}
	dep.Path = "/d/" + slug + "/"
	dep.Link = a.publicBaseURL + dep.Path
	dep.Deployed, dep.Until = deployed.UTC().Format(periodTime), until.UTC().Format(periodTime)
	return dep, nil
}

// deploymentOf loads a Dashboard's Deployment; nil if it was never deployed.
func (a *App) deploymentOf(r *http.Request, dashboardID int64) (*deployment, error) {
	dep, err := a.scanDeployment(a.db.QueryRowContext(r.Context(), `SELECT `+deploymentColumns+`
		FROM deployments dep WHERE dep.dashboard_id = $1`, dashboardID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

// deployTab is the Deploy tab: the facts and a link to the confirm step (?confirm), or the Deployment.
func (a *App) deployTab(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, err := a.learnerAccess(r)
	if err != nil {
		serverError(w, err)
		return
	}
	a.renderDeployTab(w, r, http.StatusOK, d, acc, r.URL.Query().Has("confirm"))
}

func (a *App) renderDeployTab(w http.ResponseWriter, r *http.Request, status int, d *dashboard, acc labAccess, confirm bool) {
	p := &dashboardPage{Dashboard: d, Tab: "deploy", Access: acc, Confirm: confirm, WouldSleep: time.Now().Add(awakeFor).UTC().Format(periodTime)}
	var err error
	if p.Deployment, err = a.deploymentOf(r, d.ID); err != nil {
		serverError(w, err)
		return
	}
	a.renderDashboard(w, r, status, p)
}

// awakeFor is how long a Deployment stays online.
const awakeFor = 7 * 24 * time.Hour

// deployDashboard puts a Dashboard online once, counting it against the active Plan period's allowance.
// The Admin's Deployments count against no period.
func (a *App) deployDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	if d.Dataset == nil {
		http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
		return
	}
	if r.FormValue("confirm") != "yes" {
		http.Redirect(w, r, dashboardURL(d)+"/deploy?confirm", http.StatusSeeOther)
		return
	}
	slug, err := newToken()
	if err != nil {
		serverError(w, err)
		return
	}
	slug = slug[:22] // 132 random bits, short enough to share

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	// Locking the Dashboard row makes two Deploys of it take turns, so it deploys once.
	var deployed bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM deployments WHERE dashboard_id = d.id)
		FROM dashboards d WHERE d.id = $1 FOR UPDATE`, d.ID).Scan(&deployed); err != nil {
		serverError(w, err)
		return
	}
	var period *int64 // nil for the Admin, whose Deployments count against no period
	var left int
	counted := acc.CountedPeriod()
	if counted != nil && !deployed {
		period = &counted.ID
		// Locking the period row makes two Deploys at once take turns, so the allowance holds.
		if err := tx.QueryRowContext(r.Context(), `SELECT pp.deployments - (SELECT count(*) FROM deployments WHERE period_id = pp.id)
			FROM plan_periods pp WHERE pp.id = $1 FOR UPDATE`, *period).Scan(&left); err != nil {
			serverError(w, err)
			return
		}
	}
	switch {
	case deployed:
	case period != nil && left <= 0:
		_ = tx.Rollback()
		counted.DeploymentsLeft = left
		a.renderDeployTab(w, r, http.StatusUnprocessableEntity, d, acc, false)
		return
	default:
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO deployments (dashboard_id, learner_id, period_id, slug, sleeps_at)
			SELECT $1, learner_id, $2, $3, now() + $4 * interval '1 second' FROM dashboards WHERE id = $1`, d.ID, period, slug, awakeFor.Seconds()); err != nil {
			serverError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			serverError(w, err)
			return
		}
	}
	http.Redirect(w, r, dashboardURL(d)+"/deploy", http.StatusSeeOther)
}

// deploymentFile serves a Deployment's files at /d/<slug>/ to anyone while it is awake. The page is
// sandboxed (ADR 0004), so its null origin needs CORS to load data.json. Asleep or taken down: an
// inactive page; deleted Dashboard or unknown slug: not found.
func (a *App) deploymentFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		w.Header().Set("Cache-Control", "no-store")
		d, err := a.loadDashboard(r.Context(), `id = (SELECT dashboard_id FROM deployments
			WHERE slug = $1 AND sleeps_at > now() AND taken_down_at IS NULL)`, slug)
		if err != nil {
			serverError(w, err)
			return
		}
		if d != nil {
			if name == "data.json" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			serveDashboardFile(w, r, d, name, "sandbox allow-scripts")
			return
		}
		var sleeps time.Time
		var takenDown bool
		err = a.db.QueryRowContext(r.Context(), `SELECT sleeps_at, taken_down_at IS NOT NULL FROM deployments
			WHERE slug = $1 AND dashboard_id IS NOT NULL`, slug).Scan(&sleeps, &takenDown)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			a.render(w, r, http.StatusNotFound, "message", page{Title: "Not found",
				Message: "There is no Dashboard at this link. It may have been deleted, or the link was copied wrongly."})
		case err != nil:
			serverError(w, err)
		case takenDown:
			a.render(w, r, http.StatusOK, "message", page{Title: "This Dashboard is asleep",
				Message: "Cave took it offline early. If you think this is a mistake, contact The Cave Consult."})
		default:
			a.render(w, r, http.StatusOK, "message", page{Title: "This Dashboard is asleep",
				Message: "It was online for 7 days, until " + sleeps.UTC().Format(periodTime) + ". Nothing has been deleted; ask the person who shared the link."})
		}
	}
}

// adminDeployment is one row of the Admin's Deployments list. Dashboard is empty when the Learner deleted it.
type adminDeployment struct {
	deployment
	ID        int64
	Dashboard string
	Template  string
	Owner     string
	Email     string
}

type adminDeploymentsPage struct {
	pageBase
	Deployments []adminDeployment
}

func (a *App) adminDeployments(w http.ResponseWriter, r *http.Request) {
	a.renderAdminDeployments(w, r, http.StatusOK, "")
}

// ponytail: lists every Deployment on one page; add paging when the list gets long.
func (a *App) renderAdminDeployments(w http.ResponseWriter, r *http.Request, status int, msg string) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT dep.id, COALESCE(d.name, ''), COALESCE(d.template, ''), l.name, l.email,
		`+deploymentColumns+`
		FROM deployments dep
		JOIN learners l ON l.id = dep.learner_id
		LEFT JOIN dashboards d ON d.id = dep.dashboard_id
		ORDER BY awake DESC, until DESC, dep.id DESC`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	p := &adminDeploymentsPage{pageBase: pageBase{Error: msg}}
	for rows.Next() {
		var dep adminDeployment
		var err error
		if dep.deployment, err = a.scanDeployment(rows, &dep.ID, &dep.Dashboard, &dep.Template, &dep.Owner, &dep.Email); err != nil {
			serverError(w, err)
			return
		}
		if dep.Template != "" {
			dep.Template = templateName(dep.Template)
		}
		p.Deployments = append(p.Deployments, dep)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	a.render(w, r, status, "admin_deployments", p)
}

// takeDown puts an awake Deployment to sleep at once and records why; the Learner's Dashboard stays.
func (a *App) takeDown(w http.ResponseWriter, r *http.Request) {
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		a.renderAdminDeployments(w, r, http.StatusUnprocessableEntity, "Give a reason for the Takedown. The Learner sees it in their Lab.")
		return
	}
	if len(reason) > 1000 {
		a.renderAdminDeployments(w, r, http.StatusUnprocessableEntity, "Keep the reason under 1,000 characters.")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := a.db.ExecContext(r.Context(), `UPDATE deployments SET taken_down_at = now(), takedown_reason = $2
		WHERE id = $1 AND sleeps_at > now() AND taken_down_at IS NULL`, id, reason); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/deployments", http.StatusSeeOther)
}
