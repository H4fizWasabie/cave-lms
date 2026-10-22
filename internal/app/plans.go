package app

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// plan is a Plan as the Admin pages show it.
type plan struct {
	ID          int64
	Name        string
	Days        int
	Deployments int
	storageAllowance
	Lab bool // the Lab Plan, the one Plan sold on its own and with the Pro Plan
}

type plansPage struct {
	pageBase
	Plans []plan
}

type planFormPage struct {
	pageBase
	Plan plan
}

func (a *App) adminPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := a.listPlans(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_plans", &plansPage{Plans: plans})
}

func (a *App) newPlanForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "plan_form", &planFormPage{Plan: plan{Days: 30, storageAllowance: storageAllowance{StorageMB: 1024}}})
}

func (a *App) editPlanForm(w http.ResponseWriter, r *http.Request) {
	p, ok := a.planFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "plan_form", &planFormPage{Plan: p})
}

func (a *App) createPlan(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p, msg := planFromForm(lang, r)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "plan_form", &planFormPage{pageBase{Error: msg}, p})
		return
	}
	_, err = a.db.ExecContext(r.Context(),
		`INSERT INTO plans (name, days, deployments, storage_mb) VALUES ($1, $2, $3, $4)`,
		p.Name, p.Days, p.Deployments, p.StorageMB)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/plans", http.StatusSeeOther)
}

func (a *App) updatePlan(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.planFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p, msg := planFromForm(lang, r)
	p.ID = existing.ID
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "plan_form", &planFormPage{pageBase{Error: msg}, p})
		return
	}
	_, err = a.db.ExecContext(r.Context(),
		`UPDATE plans SET name = $2, days = $3, deployments = $4, storage_mb = $5 WHERE id = $1`,
		p.ID, p.Name, p.Days, p.Deployments, p.StorageMB)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/plans", http.StatusSeeOther)
}

// markLabPlan makes the Plan named by the {id} path value the Lab Plan, taking the mark from any other.
func (a *App) markLabPlan(w http.ResponseWriter, r *http.Request) {
	p, ok := a.planFor(w, r)
	if !ok {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		if _, err = tx.ExecContext(r.Context(), `UPDATE plans SET lab = false WHERE lab AND id <> $1`, p.ID); err == nil {
			if _, err = tx.ExecContext(r.Context(), `UPDATE plans SET lab = true WHERE id = $1`, p.ID); err == nil {
				err = tx.Commit()
			}
		}
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/plans", http.StatusSeeOther)
}

// planFor loads the Plan named by the {id} path value. When there is none it writes
// the Not found page and returns false.
func (a *App) planFor(w http.ResponseWriter, r *http.Request) (plan, bool) {
	var p plan
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT id, name, days, deployments, storage_mb FROM plans WHERE id = $1`, id).
			Scan(&p.ID, &p.Name, &p.Days, &p.Deployments, &p.StorageMB)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return p, false
		}
	}
	if err != nil {
		a.adminNotFound(w, r, "plans.not-found.title", "plans.not-found")
		return p, false
	}
	return p, true
}

func (a *App) listPlans(r *http.Request) ([]plan, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id, name, days, deployments, storage_mb, lab FROM plans ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []plan
	for rows.Next() {
		var p plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Days, &p.Deployments, &p.StorageMB, &p.Lab); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// planFromForm reads the Plan form. msg is the first problem found, in lang, empty when the form is valid.
func planFromForm(lang string, r *http.Request) (p plan, msg string) {
	p.Name = strings.TrimSpace(r.PostFormValue("name"))
	days, daysErr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("days")))
	deployments, deploymentsErr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("deployments")))
	storage, storageErr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("storage")))
	p.Days, p.Deployments, p.StorageMB = days, deployments, storage
	switch {
	case p.Name == "":
		msg = text.T(lang, "plans.err.name")
	case daysErr != nil || days < 1:
		msg = text.T(lang, "plans.err.days")
	case deploymentsErr != nil || deployments < 0:
		msg = text.T(lang, "plans.err.deployments")
	case storageErr != nil || storage < 0:
		msg = text.T(lang, "plans.err.storage")
	}
	return p, msg
}

// storageAllowance is a Plan's or a Plan period's Storage allowance, kept in MB.
type storageAllowance struct{ StorageMB int }

// Storage is the Storage allowance in bytes.
func (s storageAllowance) Storage() int64 { return int64(s.StorageMB) << 20 }

// isWebLink reports whether s is an absolute http or https address, safe to put in a link.
func isWebLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// planPeriod is one assignment of a Plan to a Learner, as the Admin sees it.
type planPeriod struct {
	ID          int64
	PlanName    string
	Starts      string
	Ends        string
	Deployments int
	Status      string // active, upcoming, ended or ended-early
	Shown       string // Status as the page says it, with when it ended early
}

// planPeriodsPage shows one Learner's Plan periods. Email is the one searched for;
// HolderID is zero when no Learner has it.
type planPeriodsPage struct {
	pageBase
	Email      string
	HolderID   int64
	HolderName string
	Periods    []planPeriod
	Plans      []plan
}

const periodTime = "2006-01-02 15:04 UTC"

func (a *App) adminPlanPeriods(w http.ResponseWriter, r *http.Request) {
	p, err := a.planPeriodsPage(r, r.URL.Query().Get("email"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	status := http.StatusOK
	if p.Email != "" && p.HolderID == 0 {
		status = http.StatusNotFound
		p.Error = text.T(p.Lang, "plans.err.no-learner", p.Email)
	}
	a.render(w, r, status, "admin_plan_periods", p)
}

// nextPlanStart is s.at, when the next Plan period of the Learner $1 starts: now, or when their last Plan period
// ends if that is later. assignPlan and giveProQuery both queue after it, so a Learner's periods never overlap.
const nextPlanStart = `(SELECT GREATEST(now(), (
	SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM plan_periods WHERE learner_id = $1
)) AS at) s`

// assignPlan gives a Learner a new period of a Plan. It starts now, or when the Learner's
// last period ends if that is later, so stacked periods lose no days.
func (a *App) assignPlan(w http.ResponseWriter, r *http.Request) {
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
	planID, _ := strconv.ParseInt(r.PostFormValue("plan_id"), 10, 64)
	// ponytail: two Admin assignments racing for one Learner could overlap; fine while one Admin assigns by hand.
	res, err := a.db.ExecContext(r.Context(), `
		INSERT INTO plan_periods (learner_id, plan_id, starts_at, ends_at, deployments, storage_mb)
		SELECT $1, p.id, s.at, s.at + make_interval(days => p.days), p.deployments, p.storage_mb
		FROM plans p, `+nextPlanStart+`
		WHERE p.id = $2`, p.HolderID, planID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		p.Error = text.T(p.Lang, "plans.err.choose")
		a.render(w, r, http.StatusUnprocessableEntity, "admin_plan_periods", p)
		return
	}
	http.Redirect(w, r, "/admin/plans/periods?email="+url.QueryEscape(p.Email), http.StatusSeeOther)
}

// endPlanPeriod ends a period now. A period that has already ended is left as it is.
func (a *App) endPlanPeriod(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var email string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT l.email FROM plan_periods pp JOIN learners l ON l.id = pp.learner_id WHERE pp.id = $1`, id).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		a.adminNotFound(w, r, "plans.period.not-found.title", "plans.period.not-found")
		return
	}
	if err == nil {
		_, err = a.db.ExecContext(r.Context(),
			`UPDATE plan_periods SET ended_early_at = now() WHERE id = $1 AND ended_early_at IS NULL AND ends_at > now()`, id)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/plans/periods?email="+url.QueryEscape(email), http.StatusSeeOther)
}

// planPeriodsPage loads the Learner with the given email, their periods in start order, and every Plan.
func (a *App) planPeriodsPage(r *http.Request, email string) (*planPeriodsPage, error) {
	lang, err := a.language(r)
	if err != nil {
		return nil, err
	}
	p := &planPeriodsPage{Email: strings.TrimSpace(email)}
	p.Lang = lang // render sets it again; the messages and statuses need it first
	if p.Email == "" {
		return p, nil
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT id, name FROM learners WHERE lower(email) = lower($1)`, p.Email).
		Scan(&p.HolderID, &p.HolderName)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if p.Plans, err = a.listPlans(r); err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT pp.id, pl.name, pp.starts_at, pp.ends_at, pp.deployments, pp.ended_early_at,
			CASE WHEN pp.ended_early_at IS NOT NULL THEN 'ended-early'
				WHEN pp.ends_at <= now() THEN 'ended'
				WHEN pp.starts_at > now() THEN 'upcoming'
				ELSE 'active' END
		FROM plan_periods pp JOIN plans pl ON pl.id = pp.plan_id
		WHERE pp.learner_id = $1 ORDER BY pp.starts_at, pp.id`, p.HolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pp planPeriod
		var starts, ends time.Time
		var endedEarly sql.NullTime
		if err := rows.Scan(&pp.ID, &pp.PlanName, &starts, &ends, &pp.Deployments, &endedEarly, &pp.Status); err != nil {
			return nil, err
		}
		pp.Starts, pp.Ends = starts.UTC().Format(periodTime), ends.UTC().Format(periodTime)
		pp.Shown = text.T(lang, "plans.period."+pp.Status)
		if endedEarly.Valid {
			pp.Shown = text.T(lang, "plans.period.ended-early-on", endedEarly.Time.UTC().Format(periodTime))
		}
		p.Periods = append(p.Periods, pp)
	}
	return p, rows.Err()
}

// adminNotFound writes the Not found page with the title and message under these keys, in the page's language.
func (a *App) adminNotFound(w http.ResponseWriter, r *http.Request, titleKey, messageKey string) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, titleKey), Message: text.T(lang, messageKey)})
}
