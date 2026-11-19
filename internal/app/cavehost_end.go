package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// The end of a CaveHost Deployment (spec #536). It is deleted when its Learner deletes it, and 90 days after their last CaveHost
// Plan period ends unless a new one has started. Either way its CaveHost rows, sheets, pages and stored files go, but the
// deployments row stays, as when a Learner deletes a Lab Dashboard: its name and Link key stay taken and its link says "not found".
// ponytail: a Sync or a page upload running as its Deployment is deleted can leave one stored file behind; a sweep of the
// cavehost- keys the records do not name if that ever shows.

// caveHostLapsedAfter is how long a Learner keeps their CaveHost Deployments after their last CaveHost Plan period ends.
const caveHostLapsedAfter = 90 * 24 * time.Hour

// deleteCaveHost deletes the CaveHost records of the Deployment with this id, then its files from the Disk store. A file that
// will not go is logged and stays, as the rows that named it are gone.
func (a *App) deleteCaveHost(ctx context.Context, id int64) error {
	var keys []string
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT workbook_key FROM cavehost_deployments WHERE deployment_id = $1 AND workbook_key IS NOT NULL
			UNION ALL SELECT rows_key FROM cavehost_sheets WHERE deployment_id = $1 AND rows_key IS NOT NULL
			UNION ALL SELECT storage_key FROM cavehost_pages WHERE deployment_id = $1`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys = append(keys, key)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		_, err = tx.ExecContext(ctx, `DELETE FROM cavehost_deployments WHERE deployment_id = $1`, id) // its sheets and pages cascade
		return err
	})
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := a.store.Delete(context.WithoutCancel(ctx), key); err != nil {
			a.logError(nil, err)
		}
	}
	return nil
}

// deleteLapsedCaveHost, a step of the daily prune, deletes every CaveHost Deployment whose Learner's last CaveHost Plan period ended more than 90 days ago,
// or was ended early that long ago. A Learner with a period running or queued, and the Admin, who has none, keep theirs; so does
// one with no period at all, as nothing says when theirs ended. One that fails does not stop the others.
func (a *App) deleteLapsedCaveHost(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `SELECT dep.id FROM deployments dep JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		JOIN learners owner ON owner.id = dep.learner_id
		WHERE NOT owner.is_admin AND EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = owner.id)
			AND NOT EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = owner.id
				AND LEAST(pp.ends_at, COALESCE(pp.ended_early_at, pp.ends_at)) > now() - make_interval(secs => $1))
		ORDER BY dep.id`, caveHostLapsedAfter.Seconds())
	if err != nil {
		a.logError(nil, err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		a.logError(nil, err)
		return
	}
	for _, id := range ids {
		if err := a.deleteCaveHost(ctx, id); err != nil {
			a.logError(nil, err)
		}
	}
}

// ownCaveHostDeployment is the id of the CaveHost Deployment called {name} that belongs to the Learner asking, and its name; when
// there is none it writes the Not found page and returns false.
func (a *App) ownCaveHostDeployment(w http.ResponseWriter, r *http.Request) (id int64, name string, ok bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", false
	}
	name = r.PathValue("name")
	err = a.db.QueryRowContext(r.Context(), `SELECT dep.id FROM deployments dep JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		WHERE dep.name = $1 AND dep.learner_id = $2`, name, learner.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return 0, "", false
	}
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", false
	}
	return id, name, true
}

// confirmDeleteCaveHost asks before deleting a CaveHost Deployment.
func (a *App) confirmDeleteCaveHost(w http.ResponseWriter, r *http.Request) {
	if _, name, ok := a.ownCaveHostDeployment(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "cavehost.delete", Question: "cavehost.delete-ask",
			Cancel: "/cavehost/" + name, Action: r.URL.Path}, name)
	}
}

// deleteCaveHostDeployment deletes a CaveHost Deployment, its data and its pages, once its confirmation page is posted. Its link
// then answers "not found" and its name stays taken.
func (a *App) deleteCaveHostDeployment(w http.ResponseWriter, r *http.Request) {
	id, _, ok := a.ownCaveHostDeployment(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if err := a.deleteCaveHost(r.Context(), id); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost", http.StatusSeeOther)
}
