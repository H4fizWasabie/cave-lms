package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
)

// partRow is a current Part as the Data tab lists it, in the Dataset's order.
type partRow struct {
	ID    int64
	File  string
	Date  string // the Part date
	Rows  int
	Size  int64
	Added string // formatted as periodTime
}

func (p partRow) SizeText() string { return sizeText(p.Size) }

// partsOf lists the Dashboard's current Parts in order.
func (a *App) partsOf(ctx context.Context, dashboardID int64) ([]partRow, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, file_name, part_date::text, row_count, saved_size, confirmed_at
		FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current' ORDER BY position`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []partRow
	for rows.Next() {
		var p partRow
		var added sql.NullTime
		if err := rows.Scan(&p.ID, &p.File, &p.Date, &p.Rows, &p.Size, &added); err != nil {
			return nil, err
		}
		p.Added = added.Time.UTC().Format(periodTime)
		list = append(list, p)
	}
	return list, rows.Err()
}

// partFor applies the Lab access rule and loads the file name of Dashboard d's current Part named by {part}. ok is
// false when it has answered.
func (a *App) partFor(w http.ResponseWriter, r *http.Request, d *dashboard) (id int64, file string, ok bool) {
	if _, ok := a.labOpen(w, r); !ok {
		return 0, "", false
	}
	id, err := strconv.ParseInt(r.PathValue("part"), 10, 64)
	if err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT file_name FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`,
			id, d.ID).Scan(&file)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
		a.notFound(w, r)
		return 0, "", false
	}
	if err != nil {
		a.serverError(w, r, err)
		return 0, "", false
	}
	return id, file, true
}

// confirmDeletePart asks before deleting a Part.
func (a *App) confirmDeletePart(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if _, file, ok := a.partFor(w, r, d); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "lab.d.part-delete", Question: "lab.d.part-delete-ask", Cancel: dataURL(d),
			Action: r.URL.Path}, file)
	}
}

// deletePart deletes a Part once its confirmation page is posted: its rows leave data.json, its room is free in the
// Storage allowance and its file is queued for deletion. The other Parts keep their order. Deleting the last Part
// leaves the Dataset empty: its columns are cleared to the empty list (NULL is a Dataset never uploaded, which
// data.json answers 404), so the next upload sets new ones. An Add waiting to be confirmed was put together from the
// Parts as they were, so it goes too.
func (a *App) deletePart(w http.ResponseWriter, r *http.Request, d *dashboard) {
	id, _, ok := a.partFor(w, r, d)
	if !ok || unconfirmed(w, r) {
		return
	}
	gone := errors.New("Part already deleted")
	err := a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Locking the Learner makes deletes, confirms and uploads take turns.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		res, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`, id, d.ID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return errors.Join(err, gone)
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND state = 'pending' AND cleanup ? 'added'`, d.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET updated_at = now(),
				dataset_columns = CASE WHEN EXISTS (SELECT 1 FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current') THEN dataset_columns ELSE '[]' END
			WHERE id = $1`, d.ID)
		return err
	})
	if errors.Is(err, gone) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, dataURL(d), http.StatusSeeOther)
}
