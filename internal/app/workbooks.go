package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"time"
)

// xlsxType is the content type a Workbook is served with.
const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// workbook is a Learner's Workbook as their Folder lists it (ADR 0021).
type workbook struct {
	ID       int64
	Name     string
	Size     int64
	Uploaded string // formatted as periodTime
}

func (w workbook) SizeText() string { return sizeText(w.Size) }

// workbooksOf lists a Learner's Workbooks, newest first.
func (a *App) workbooksOf(ctx context.Context, learnerID int64) ([]workbook, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, size, uploaded_at FROM workbooks
		WHERE learner_id = $1 ORDER BY uploaded_at DESC, id DESC`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []workbook
	for rows.Next() {
		var w workbook
		var at time.Time
		if err := rows.Scan(&w.ID, &w.Name, &w.Size, &at); err != nil {
			return nil, err
		}
		w.Uploaded = at.UTC().Format(periodTime)
		out = append(out, w)
	}
	return out, rows.Err()
}

// keepWorkbook keeps data, the .xlsx a Learner uploaded, as their Workbook called name, before Clean-up reads it, so it
// stays whatever Clean-up finds. With acc's counted period it must fit the Storage allowance beside the Learner's
// Parts, pages and Workbooks and their pending Parts on Dashboards other than dashboardID (0 for a new one),
// whose own pending Part the upload replaces; the Parts roomFor counts as deleted by the confirm count as free, as for
// keepPending: free (every current Part of dashboardID when the upload replaces its data, the one it replaces, else none). If it does not fit, nothing is kept and the error is errStorageFull, with used the bytes
// counted. A store that refuses the file gives errDatasetStore. An empty file is not kept: id is 0. A file the Learner
// already keeps gives that Workbook's id, keeping nothing new.
func (a *App) keepWorkbook(r *http.Request, learnerID, dashboardID int64, acc labAccess, free frees, name string, data []byte) (id, used int64, err error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	// The preview's sheet, header row and month post the file again: the same bytes stay one Workbook.
	// ponytail: two identical uploads racing each other may still keep two; a unique index if that ever matters.
	sum := sha256.Sum256(data)
	err = a.db.QueryRowContext(r.Context(), `SELECT id FROM workbooks WHERE learner_id = $1 AND sha256 = $2 LIMIT 1`,
		learnerID, sum[:]).Scan(&id)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, 0, err
	}
	token, err := newToken()
	if err != nil {
		return 0, 0, err
	}
	key := "workbook-" + token[:22]
	if err := a.datasets.Put(r.Context(), key, data); err != nil {
		a.logError(r, fmt.Errorf("put Workbook %s: %w", key, err))
		return 0, 0, errDatasetStore
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if period := acc.CountedPeriod(); period != nil {
			// Locking the Learner makes uploads and confirms take turns, so none slips under the allowance.
			if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learnerID); err != nil {
				return err
			}
			var pending int64 // the Learner's pending Parts on their other Dashboards
			if err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(sum(ds.saved_size), 0) FROM dataset_parts ds
				JOIN dashboards owner ON owner.id = ds.dashboard_id
				WHERE owner.learner_id = $1 AND ds.state = 'pending' AND ds.dashboard_id <> $2`, learnerID, dashboardID).Scan(&pending); err != nil {
				return err
			}
			if dashboardID != 0 {
				// As keepPending: what confirming deletes from this Dashboard counts as free.
				room, err := roomFor(r.Context(), tx, dashboardID, pending+int64(len(data)), free, acc)
				if err != nil {
					return err
				}
				if used = room.Used + pending; !room.Fits {
					return errStorageFull
				}
			} else {
				var inUse int64
				if err := tx.QueryRowContext(r.Context(), `SELECT `+storageUsedSQL("$1"), learnerID).Scan(&inUse); err != nil {
					return err
				}
				if used = inUse + pending; used+int64(len(data)) > period.Storage() {
					return errStorageFull
				}
			}
		}
		return tx.QueryRowContext(r.Context(), `INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, $2, $3, $4, $5)
			RETURNING id`, learnerID, key, name, len(data), sum[:]).Scan(&id)
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		return 0, used, err
	}
	return id, used, nil
}

// downloadWorkbook sends a Workbook exactly as uploaded to its Learner, whether or not their Plan has ended, or to the
// Admin. Anyone else gets Not found, so another Learner's Workbooks are not revealed.
func (a *App) downloadWorkbook(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	var owner int64
	var name, key string
	err = a.db.QueryRowContext(r.Context(), `SELECT learner_id, name, storage_key FROM workbooks WHERE id = $1`, id).Scan(&owner, &name, &key)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != learner.ID && !learner.IsAdmin) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data, err := a.datasets.Get(r.Context(), key)
	if err != nil {
		a.serverError(w, r, fmt.Errorf("get Workbook %s: %w", key, err))
		return
	}
	w.Header().Set("Content-Type", xlsxType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(data)
}

// deletableWorkbook loads the Workbook named by {id} when it is the logged-in Learner's own, with how many Dashboards
// used it. Anyone else's, the Admin included, as for a Submission's files, gets the Not found page, so another
// Learner's Workbooks are not revealed. The caller checks the Lab is open to the Learner.
func (a *App) deletableWorkbook(w http.ResponseWriter, r *http.Request) (id int64, name string, uses int, ok bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", 0, false
	}
	if id, err = strconv.ParseInt(r.PathValue("id"), 10, 64); err != nil {
		a.notFound(w, r)
		return 0, "", 0, false
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT name,
		(SELECT count(DISTINCT ds.dashboard_id) FROM dataset_parts ds WHERE ds.workbook_id = wb.id)
		FROM workbooks wb WHERE wb.id = $1 AND wb.learner_id = $2`, id, learner.ID).Scan(&name, &uses)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return 0, "", 0, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", 0, false
	}
	return id, name, uses, true
}

// confirmDeleteWorkbook asks the Learner to confirm deleting one of their Workbooks, naming how many Dashboards used it.
func (a *App) confirmDeleteWorkbook(w http.ResponseWriter, r *http.Request) {
	if _, name, uses, ok := a.deletableWorkbook(w, r); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "workbook.delete", Question: "workbook.delete-ask",
			Cancel: "/my-folder", CancelLabel: "workbook.keep", Action: r.URL.Path}, name, uses)
	}
}

// deleteWorkbook deletes a Workbook and its file once its confirmation page is posted, freeing its room in the Storage
// allowance. The Parts cleaned from it, their Dashboards and Deployments stay and no longer name a Workbook. Like
// every change in the Lab it needs an active Plan period (403 without).
func (a *App) deleteWorkbook(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.labOpen(w, r); !ok {
		return
	}
	id, _, _, ok := a.deletableWorkbook(w, r)
	if !ok || unconfirmed(w, r) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM workbooks WHERE id = $1`, id); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, "/my-folder", http.StatusSeeOther)
}
