package app

import (
	"context"
	"database/sql"
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

// deleteCaveHost deletes the CaveHost records of the Deployment with this id, then its files from the Disk store, and notes on its
// row of deployments who deleted it, by: "learner" or "lapsed" (the 90 days). A file that will not go is logged and stays, as the
// rows that named it are gone.
func (a *App) deleteCaveHost(ctx context.Context, id int64, by string) error {
	var keys []string
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT xlsx_key FROM cavehost_deployments WHERE deployment_id = $1 AND xlsx_key IS NOT NULL
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
		if _, err = tx.ExecContext(ctx, `UPDATE deployments SET deleted_by = $2 WHERE id = $1`, id, by); err != nil {
			return err
		}
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
	ids, err := a.caveHostIDs(ctx, `SELECT dep.id FROM deployments dep JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		JOIN learners owner ON owner.id = dep.learner_id
		WHERE NOT owner.is_admin AND EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = owner.id)
			AND NOT EXISTS (SELECT 1 FROM cavehost_plan_periods pp WHERE pp.learner_id = owner.id
				AND LEAST(pp.ends_at, COALESCE(pp.ended_early_at, pp.ends_at)) > now() - make_interval(secs => $1))
		ORDER BY dep.id`, caveHostLapsedAfter.Seconds())
	if err != nil {
		a.logError(nil, err)
		return
	}
	for _, id := range ids {
		if err := a.deleteCaveHost(ctx, id, "lapsed"); err != nil {
			a.logError(nil, err)
		}
	}
}

// confirmDeleteCaveHost asks before deleting a CaveHost Deployment. Deleting needs no CaveHost Plan, so a Learner can delete a
// sleeping one; it is still only their own.
func (a *App) confirmDeleteCaveHost(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.caveHostViewOf(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "cavehost.delete", Question: "cavehost.delete-ask",
			Cancel: "/cavehost/" + v.Name, Action: r.URL.Path}, v.Name)
	}
}

// deleteCaveHostDeployment deletes a CaveHost Deployment, its data and its pages, once its confirmation page is posted. Its link
// then answers "not found" and its name stays taken.
func (a *App) deleteCaveHostDeployment(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if err := a.deleteCaveHost(r.Context(), v.ID, "learner"); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost", http.StatusSeeOther)
}
