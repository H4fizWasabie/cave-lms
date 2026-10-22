package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// deployment is a Dashboard's Deployment as its Learner sees it on the Deploy tab.
type deployment struct {
	Path      string // /d/<slug>/
	Link      string // the full link to share
	Deployed  string // formatted as periodTime
	Awake     bool
	TakenDown string // when Cave took it down, formatted as periodTime; empty if it was not
	Reason    string // why Cave took it down
}

// depAwake is true for an awake Deployment, aliased dep: its Dashboard is not deleted, it is not taken down,
// and its Learner is the Admin or has an active Plan period.
const depAwake = `(dep.dashboard_id IS NOT NULL AND dep.taken_down_at IS NULL AND EXISTS (SELECT 1 FROM learners owner
	WHERE owner.id = dep.learner_id AND (owner.is_admin OR EXISTS (SELECT 1 FROM plan_periods pp WHERE pp.learner_id = owner.id
		AND pp.starts_at <= now() AND pp.ends_at > now() AND pp.ended_early_at IS NULL))))`

// deploymentColumns select a deployment from deployments aliased dep; scanDeployment reads them.
const deploymentColumns = `dep.slug, dep.deployed_at, ` + depAwake + ` AS awake, dep.taken_down_at, dep.takedown_reason`

// scanDeployment scans a row whose columns are lead's targets followed by deploymentColumns.
func (a *App) scanDeployment(row interface{ Scan(...any) error }, lead ...any) (deployment, error) {
	var dep deployment
	var slug string
	var deployed time.Time
	var takenDown sql.NullTime
	if err := row.Scan(append(lead, &slug, &deployed, &dep.Awake, &takenDown, &dep.Reason)...); err != nil {
		return dep, err
	}
	dep.Path = "/d/" + slug + "/"
	dep.Link = a.publicBaseURL + dep.Path
	dep.Deployed = deployed.UTC().Format(periodTime)
	if takenDown.Valid {
		dep.TakenDown = takenDown.Time.UTC().Format(periodTime)
	}
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
		a.serverError(w, r, err)
		return
	}
	a.renderDeployTab(w, r, http.StatusOK, d, acc, r.URL.Query().Has("confirm"))
}

func (a *App) renderDeployTab(w http.ResponseWriter, r *http.Request, status int, d *dashboard, acc labAccess, confirm bool) {
	p := &dashboardPage{Dashboard: d, Tab: "deploy", Access: acc, Confirm: confirm}
	var err error
	if p.Deployment, err = a.deploymentOf(r, d.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderDashboard(w, r, status, p)
}

// deployDashboard puts a Dashboard online once, if the Learner has fewer awake Deployments than their active
// Plan period allows. The Admin's Deployments count against nothing.
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
		a.serverError(w, r, err)
		return
	}
	slug = slug[:22] // 132 random bits, short enough to share

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	// Locking the Dashboard row makes two Deploys of it take turns, so it deploys once.
	var deployed bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM deployments WHERE dashboard_id = d.id)
		FROM dashboards d WHERE d.id = $1 FOR UPDATE`, d.ID).Scan(&deployed); err != nil {
		a.serverError(w, r, err)
		return
	}
	counted := acc.CountedPeriod()
	if counted != nil && !deployed {
		// Locking the period row makes two Deploys at once take turns, so the allowance holds.
		if err := tx.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM deployments dep WHERE dep.learner_id = pp.learner_id AND `+depAwake+`)
			FROM plan_periods pp WHERE pp.id = $1 FOR UPDATE`, counted.ID).Scan(&counted.Awake); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	switch {
	case deployed:
	case counted != nil && counted.Full():
		_ = tx.Rollback()
		a.renderDeployTab(w, r, http.StatusUnprocessableEntity, d, acc, false)
		return
	default:
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO deployments (dashboard_id, learner_id, slug)
			SELECT $1, learner_id, $2 FROM dashboards WHERE id = $1`, d.ID, slug); err != nil {
			a.serverError(w, r, err)
			return
		}
		if err := tx.Commit(); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, dashboardURL(d)+"/deploy", http.StatusSeeOther)
}

// deploymentFile serves a Deployment's files at /d/<slug>/ to anyone while it is awake. The page is
// sandboxed (ADR 0007), so its null origin needs CORS to load its data. Asleep or taken down: an
// inactive page; deleted Dashboard or unknown slug: not found.
func (a *App) deploymentFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		w.Header().Set("Cache-Control", "no-store")
		d, err := a.loadDashboard(r.Context(), `id = (SELECT dashboard_id FROM deployments dep
			WHERE dep.slug = $1 AND `+depAwake+`)`, slug)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if d != nil {
			if name == "data" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			a.serveDashboardFile(w, r, d, name, "sandbox allow-scripts", "public, max-age=31536000, immutable")
			return
		}
		var takenDown bool
		err = a.db.QueryRowContext(r.Context(), `SELECT taken_down_at IS NOT NULL FROM deployments
			WHERE slug = $1 AND dashboard_id IS NOT NULL`, slug).Scan(&takenDown)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return
		}
		lang, langErr := a.language(r)
		if langErr != nil {
			a.serverError(w, r, langErr)
			return
		}
		switch {
		case err != nil:
			a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "lab.dep.gone.title"), Message: text.T(lang, "lab.dep.gone")})
		case takenDown:
			a.render(w, r, http.StatusOK, "message", page{Title: text.T(lang, "lab.dep.sleeping.title"), Message: text.T(lang, "lab.dep.sleeping-taken")})
		default:
			a.render(w, r, http.StatusOK, "message", page{Title: text.T(lang, "lab.dep.sleeping.title"), Message: text.T(lang, "lab.dep.sleeping")})
		}
	}
}

// adminDeployment is one row of the Admin's Deployments list. Dashboard is empty when the Learner deleted it.
type adminDeployment struct {
	deployment
	ID        int64
	Dashboard string
	Template  string
	Learner   string
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
// renderAdminDeployments shows the list, with the error under msgKey ("" for none) in the page's language.
func (a *App) renderAdminDeployments(w http.ResponseWriter, r *http.Request, status int, msgKey string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	msg := ""
	if msgKey != "" {
		msg = text.T(lang, msgKey)
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT dep.id, COALESCE(d.name, ''), COALESCE(d.template, ''), l.name, l.email,
		`+deploymentColumns+`
		FROM deployments dep
		JOIN learners l ON l.id = dep.learner_id
		LEFT JOIN dashboards d ON d.id = dep.dashboard_id
		ORDER BY awake DESC, dep.id DESC`)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	p := &adminDeploymentsPage{pageBase: pageBase{Error: msg}}
	for rows.Next() {
		var dep adminDeployment
		var err error
		if dep.deployment, err = a.scanDeployment(rows, &dep.ID, &dep.Dashboard, &dep.Template, &dep.Learner, &dep.Email); err != nil {
			a.serverError(w, r, err)
			return
		}
		if dep.Template != "" {
			dep.Template = templateName(dep.Template)
		}
		p.Deployments = append(p.Deployments, dep)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "admin_deployments", p)
}

// takeDown puts a Deployment to sleep for good and records why; the Learner's Dashboard stays.
func (a *App) takeDown(w http.ResponseWriter, r *http.Request) {
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		a.renderAdminDeployments(w, r, http.StatusUnprocessableEntity, "deployments.err.reason")
		return
	}
	if len(reason) > 1000 {
		a.renderAdminDeployments(w, r, http.StatusUnprocessableEntity, "deployments.err.reason-long")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := a.db.ExecContext(r.Context(), `UPDATE deployments SET taken_down_at = now(), takedown_reason = $2
		WHERE id = $1 AND dashboard_id IS NOT NULL AND taken_down_at IS NULL`, id, reason); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/deployments", http.StatusSeeOther)
}
