package app

import (
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// deployment is a Dashboard's Deployment as its owner sees it on the Deploy tab.
type deployment struct {
	Path     string // /d/<slug>/
	Link     string // the full link to share
	Deployed string // formatted as periodTime
	Until    string // when it sleeps, slept or was taken down
	Awake    bool
}

// deploymentOf loads a Dashboard's Deployment; nil if it was never deployed.
func (a *App) deploymentOf(r *http.Request, dashboardID int64) (*deployment, error) {
	var dep deployment
	var slug string
	var deployed, until time.Time
	err := a.db.QueryRowContext(r.Context(), `SELECT slug, deployed_at, LEAST(sleeps_at, COALESCE(taken_down_at, sleeps_at)),
		sleeps_at > now() AND taken_down_at IS NULL FROM deployments WHERE dashboard_id = $1`, dashboardID).
		Scan(&slug, &deployed, &until, &dep.Awake)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dep.Path = "/d/" + slug + "/"
	dep.Link = a.publicBaseURL + dep.Path
	dep.Deployed, dep.Until = deployed.UTC().Format(periodTime), until.UTC().Format(periodTime)
	return &dep, nil
}

// deployTab is the Deploy tab: the facts and a link to the confirm step (?confirm), or the Deployment.
func (a *App) deployTab(w http.ResponseWriter, r *http.Request, d *dashboard) {
	a.renderDeployTab(w, r, http.StatusOK, d, r.URL.Query().Has("confirm"))
}

func (a *App) renderDeployTab(w http.ResponseWriter, r *http.Request, status int, d *dashboard, confirm bool) {
	p := &dashboardPage{D: d, Tab: "deploy", Confirm: confirm, WouldSleep: time.Now().Add(awakeFor).UTC().Format(periodTime)}
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
func (a *App) deployDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if !a.labOpen(w, r) {
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
	learner, _ := a.currentLearner(r)
	acc, err := a.labAccessOf(r.Context(), learner.ID)
	if err != nil {
		serverError(w, err)
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
	// Locking the period row makes two Deploys at once take turns, so the allowance holds.
	var left int
	var deployed bool
	if err := tx.QueryRowContext(r.Context(), `SELECT pp.deployments - (SELECT count(*) FROM deployments WHERE period_id = pp.id),
		EXISTS (SELECT 1 FROM deployments WHERE dashboard_id = $2) FROM plan_periods pp WHERE pp.id = $1 FOR UPDATE OF pp`,
		acc.Period.ID, d.ID).Scan(&left, &deployed); err != nil {
		serverError(w, err)
		return
	}
	switch {
	case deployed:
	case left <= 0:
		_ = tx.Rollback()
		a.renderDeployTab(w, r, http.StatusUnprocessableEntity, d, false)
		return
	default:
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO deployments (dashboard_id, period_id, slug, sleeps_at)
			VALUES ($1, $2, $3, now() + $4 * interval '1 second')`, d.ID, acc.Period.ID, slug, awakeFor.Seconds()); err != nil {
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
