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

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
)

// xlsxType is the content type a Workbook is served with.
const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// workbook is a Learner's Workbook as their Folder lists it (ADR 0021).
type workbook struct {
	ID       int64
	Name     string
	Size     int64
	Uploaded string        // Malaysia time, in the page's language
	UsedOn   []dataset.Use // the Dashboards that have a Part cleaned from it
}

// SizeText is the Workbook's size as people read it, such as 12 KB.
func (w workbook) SizeText() string { return sizeText(w.Size) }

// workbooksOf lists a Learner's Workbooks, newest first, each with the Dashboards that currently have a Part cleaned
// from it, by name.
func (a *App) workbooksOf(ctx context.Context, lang string, learnerID int64) ([]workbook, error) {
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
		w.Uploaded = dateTimeIn(lang, at)
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	uses, err := a.dataset.Uses(ctx, learnerID)
	for i := range out { // a Workbook kept between the two queries waits for the next visit
		out[i].UsedOn = uses[out[i].ID]
	}
	return out, err
}

// keepWorkbook keeps data, the .xlsx a Learner uploaded, as their Workbook called name, before Clean-up reads it, so it
// stays whatever Clean-up finds. With acc's counted period it must fit the Storage allowance (see reserve); what
// confirming the upload deletes from dashboardID (0 for a new one) counts as free, as for the Part's own check: the
// Parts an upload of kind deletes when confirmed (replaces is the Part a ReplacePart one replaces) and the Dashboard's own
// Pending Part. If it does not fit, nothing is kept and the error is a *storageFull. A store that refuses the file gives
// errDatasetStore. An empty file is not kept: id is 0. A file the Learner already keeps gives that Workbook's id, keeping
// nothing new.
//
// borrowed is true when the Workbook was newly kept and fits only on that credit: the Parts it counts as free are still
// there until a confirm deletes them, so if the upload comes to nothing the caller gives the Workbook back (dropBorrowed).
func (a *App) keepWorkbook(r *http.Request, learnerID, dashboardID int64, acc labAccess, kind dataset.Kind, replaces int64, name string, data []byte) (id int64, borrowed bool, err error) {
	if len(data) == 0 {
		return 0, false, nil
	}
	// The preview's sheet, header row and month post the file again: the same bytes stay one Workbook.
	// ponytail: two identical uploads racing each other may still keep two; a unique index if that ever matters.
	sum := sha256.Sum256(data)
	err = a.db.QueryRowContext(r.Context(), `SELECT id FROM workbooks WHERE learner_id = $1 AND sha256 = $2 LIMIT 1`,
		learnerID, sum[:]).Scan(&id)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, false, err
	}
	token, err := newToken()
	if err != nil {
		return 0, false, err
	}
	key := "workbook-" + token[:22]
	if err := a.datasets.Put(r.Context(), key, data); err != nil {
		a.logError(r, fmt.Errorf("put Workbook %s: %w", key, err))
		return 0, false, errDatasetStore
	}
	err = a.inTx(r.Context(), func(tx *sql.Tx) error {
		if err := reserve(r.Context(), tx, acc, learnerID, int64(len(data)), replacedParts(dashboardID, kind, replaces)); err != nil {
			return err
		}
		if err := tx.QueryRowContext(r.Context(), `INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, $2, $3, $4, $5)
			RETURNING id`, learnerID, key, name, len(data), sum[:]).Scan(&id); err != nil {
			return err
		}
		// Over the allowance with the Workbook in, so it got in on the credit alone.
		borrowed = checkStorage(r.Context(), tx, acc, learnerID, 0, nil) != nil
		return nil
	})
	if err != nil {
		a.dropDataset(r.Context(), r, key)
		return 0, false, err
	}
	return id, borrowed, nil
}

// dropBorrowed deletes the Workbook keepWorkbook kept on credit, when the upload it was kept for comes to nothing, so
// the Learner never holds more than their Storage allowance and the Parts counted as free. The credit is only good
// for a Workbook that a confirm will follow (ADR 0021 keeps a Workbook whatever Clean-up finds, as long as it fits).
func (a *App) dropBorrowed(ctx context.Context, r *http.Request, id int64) {
	if _, err := a.db.ExecContext(context.WithoutCancel(ctx), `DELETE FROM workbooks WHERE id = $1`, id); err != nil {
		a.logError(r, fmt.Errorf("delete borrowed Workbook %d: %w", id, err))
		return
	}
	a.dropDeletedFiles(ctx, r)
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
	err = a.db.QueryRowContext(r.Context(), `SELECT name FROM workbooks WHERE id = $1 AND learner_id = $2`, id, learner.ID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return 0, "", 0, false
	}
	var used map[int64][]dataset.Use
	if err == nil {
		used, err = a.dataset.Uses(r.Context(), learner.ID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", 0, false
	}
	return id, name, len(used[id]), true
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
