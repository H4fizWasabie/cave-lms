package app

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// deploymentPrefix is the path Deployments live under, /<prefix>/<name>/ (ADR 0007). The client picks the word.
const deploymentPrefix = "d"

// deploymentName is what a Learner may pick: 3 to 40 lowercase letters, digits and single dashes, no dash at
// either end. Deployments made before names keep their random slug as their name, outside these rules.
var deploymentName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedNames look like LMS routes or official pages, so no Learner can impersonate the site with them.
var reservedNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`about account accounts admin administrator api app billing cave cave-consult
		cavelms cave-lms cavetext certificate certificates contact course courses dashboard dashboards deploy
		deployment deployments forgot-password gallery help home index lab language learner learners login logout
		my-certificates my-courses official pay payment payments plan plans privacy pro register reset-password
		security settings signin sign-in signup sign-up static support terms the-cave-consult thecaveconsult verify
		wasabie wasabietech www`) {
		reservedNames[n] = true
	}
}

// nameError is the catalog key saying why a Learner may not pick name, or "" if they may (taken aside).
func nameError(name string) string {
	switch {
	case len(name) < 3 || len(name) > 40:
		return "lab.dep.err.name-length"
	case !deploymentName.MatchString(name):
		return "lab.dep.err.name-chars"
	case reservedNames[name]:
		return "lab.dep.err.name-reserved"
	}
	return ""
}

// deployment is a Dashboard's Deployment as its Learner sees it on the Deploy tab.
type deployment struct {
	Name      string
	Path      string // /<prefix>/<name>/
	Link      string // the full link to share
	Deployed  string // formatted as periodTime
	Awake     bool
	TakenDown string // when Cave took it down, formatted as periodTime; empty if it was not
	Reason    string // why Cave took it down
}

// deploymentAwakeSQL is an SQL condition, true for an awake Deployment aliased dep: its Dashboard is not deleted,
// it is not taken down, and its Learner is the Admin or has an active Plan period.
const deploymentAwakeSQL = `(dep.dashboard_id IS NOT NULL AND dep.taken_down_at IS NULL AND EXISTS (SELECT 1 FROM learners owner
	WHERE owner.id = dep.learner_id AND (owner.is_admin OR EXISTS (SELECT 1 FROM plan_periods pp WHERE pp.learner_id = owner.id
		AND ` + activePeriodSQL + `))))`

// deploymentColumns select a deployment from deployments aliased dep; scanDeployment reads them.
const deploymentColumns = `dep.name, dep.deployed_at, ` + deploymentAwakeSQL + ` AS awake, dep.taken_down_at, dep.takedown_reason`

// scanDeployment scans a row whose columns are lead's targets followed by deploymentColumns.
func (a *App) scanDeployment(row interface{ Scan(...any) error }, lead ...any) (deployment, error) {
	var dep deployment
	var deployed time.Time
	var takenDown sql.NullTime
	if err := row.Scan(append(lead, &dep.Name, &deployed, &dep.Awake, &takenDown, &dep.Reason)...); err != nil {
		return dep, err
	}
	dep.Path = "/" + deploymentPrefix + "/" + dep.Name + "/"
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
	a.renderDeployTab(w, r, http.StatusOK, &dashboardPage{Dashboard: d, Access: acc, Confirm: r.URL.Query().Has("confirm")})
}

// renderDeployTab renders p, filled in up to its Deploy tab fields, on the Deploy tab.
func (a *App) renderDeployTab(w http.ResponseWriter, r *http.Request, status int, p *dashboardPage) {
	p.Tab = "deploy"
	p.Prefix = a.publicBaseURL + "/" + deploymentPrefix + "/"
	var err error
	if p.Deployment, err = a.deploymentOf(r, p.Dashboard.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderDashboard(w, r, status, p)
}

// deployDashboard puts a Dashboard online once, under the name its Learner picked and with their word that they
// have the right to put its data online, if they have fewer awake Deployments than their active Plan period allows. The Admin's
// Deployments count against nothing.
func (a *App) deployDashboard(w http.ResponseWriter, r *http.Request, d *dashboard) {
	acc, ok := a.labOpen(w, r)
	if !ok {
		return
	}
	if d.Parts == 0 && d.Page == nil {
		http.Redirect(w, r, dashboardURL(d), http.StatusSeeOther)
		return
	}
	if r.FormValue("confirm") != "yes" {
		http.Redirect(w, r, dashboardURL(d)+"/deploy?confirm", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	refuse := func(msgKey string) {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p := &dashboardPage{Dashboard: d, Access: acc, Confirm: true, DeployName: name}
		p.Error = text.T(lang, msgKey)
		a.renderDeployTab(w, r, http.StatusUnprocessableEntity, p)
	}
	if key := nameError(name); key != "" {
		refuse(key)
		return
	}
	if r.PostFormValue("right") != "yes" {
		refuse("lab.dep.err.right")
		return
	}

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
	if deployed {
		http.Redirect(w, r, dashboardURL(d)+"/deploy", http.StatusSeeOther)
		return
	}
	counted := acc.CountedPeriod()
	if counted != nil {
		// Locking the period row makes two Deploys at once take turns, so the allowance holds.
		if err := tx.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM deployments dep WHERE dep.learner_id = pp.learner_id AND `+deploymentAwakeSQL+`)
			FROM plan_periods pp WHERE pp.id = $1 FOR UPDATE`, counted.ID).Scan(&counted.Awake); err != nil {
			a.serverError(w, r, err)
			return
		}
		if counted.Full() {
			_ = tx.Rollback()
			a.renderDeployTab(w, r, http.StatusUnprocessableEntity, &dashboardPage{Dashboard: d, Access: acc})
			return
		}
	}
	// The row outlives its Dashboard, so the unique name is never given to another Deployment.
	_, err = tx.ExecContext(r.Context(), `INSERT INTO deployments (dashboard_id, learner_id, name)
		SELECT $1, learner_id, $2 FROM dashboards WHERE id = $1`, d.ID, name)
	if isUniqueViolation(err) {
		refuse("lab.dep.err.name-taken")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, dashboardURL(d)+"/deploy", http.StatusSeeOther)
}

// deploymentCSP sandboxes a Deployment's page (ADR 0007) and keeps it to Cave's own scripts and data, unframed.
// The sandbox gives the page an opaque origin, where some browsers match nothing to 'self', so Cave's address
// is named too, by its origin alone, as a source with a path would match only that exact path.
func (a *App) deploymentCSP() string {
	origin := a.origin()
	return "sandbox allow-scripts; default-src 'none'; script-src 'self' " + origin + " 'unsafe-inline'; " +
		"style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'self' " + origin + "; frame-ancestors 'none'"
}

// origin is Cave's address by its origin alone: scheme and host.
func (a *App) origin() string {
	u, _ := url.Parse(a.publicBaseURL) // config.Load has checked it
	return u.Scheme + "://" + u.Host
}

// deploymentFile serves a Deployment's files at /<prefix>/<name>/ to anyone while it is awake. The page is
// sandboxed (ADR 0007), so its null origin needs CORS to load its data. Asleep or taken down: an
// inactive page; deleted Dashboard or unknown name: not found.
func (a *App) deploymentFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		depName := r.PathValue("name")
		w.Header().Set("Cache-Control", "no-store")
		d, err := a.loadDashboard(r.Context(), `id = (SELECT dashboard_id FROM deployments dep
			WHERE dep.name = $1 AND `+deploymentAwakeSQL+`)`, depName)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if d != nil && d.Page != nil && name == "index.html" {
			a.servePage(w, r, d.Page)
			return
		}
		if d != nil && name == "data.json" {
			if d.Page == nil { // a built Dashboard's data is at data/<key>
				a.notFound(w, r)
				return
			}
			a.servePageData(w, r, d)
			return
		}
		if d != nil {
			if name == "data" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			// Five minutes, not for good: a Deployment that sleeps or is taken down must stop showing its data soon.
			a.serveDashboardFile(w, r, d, name, a.deploymentCSP(), "public, max-age=300")
			return
		}
		var takenDown bool
		err = a.db.QueryRowContext(r.Context(), `SELECT taken_down_at IS NOT NULL FROM deployments
			WHERE name = $1 AND dashboard_id IS NOT NULL`, depName).Scan(&takenDown)
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
	rows, err := a.db.QueryContext(r.Context(), `SELECT dep.id, COALESCE(d.name, ''), l.name, l.email,
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
		if dep.deployment, err = a.scanDeployment(rows, &dep.ID, &dep.Dashboard, &dep.Learner, &dep.Email); err != nil {
			a.serverError(w, r, err)
			return
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
