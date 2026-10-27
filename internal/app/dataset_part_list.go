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

// SizeText is the Part's saved size as people read it, such as 12 KB.
func (p partRow) SizeText() string { return sizeText(p.Size) }

// partRef is a current Part named by a request, such as the one a Replace link or a delete names.
type partRef struct {
	ID   int64
	File string
	Rows int
}

// samePart is the current Part a refused upload is the exact same file as.
type samePart struct {
	ID   int64
	File string
	Date string // the Part date
}

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

// partFor applies the Lab access rule, which it returns, and loads the ID, file name and rows of Dashboard d's current
// Part named by {part}. ok is false when it has answered.
func (a *App) partFor(w http.ResponseWriter, r *http.Request, d *dashboard) (p *partRef, acc labAccess, ok bool) {
	acc, ok = a.labOpen(w, r)
	if !ok {
		return nil, acc, false
	}
	p = &partRef{}
	var err error
	if p.ID, err = strconv.ParseInt(r.PathValue("part"), 10, 64); err == nil {
		err = a.db.QueryRowContext(r.Context(), `SELECT file_name, row_count FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`,
			p.ID, d.ID).Scan(&p.File, &p.Rows)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, strconv.ErrSyntax) || errors.Is(err, strconv.ErrRange) {
		a.notFound(w, r)
		return nil, acc, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return nil, acc, false
	}
	return p, acc, true
}

// confirmDeletePart asks before deleting a Part.
func (a *App) confirmDeletePart(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if p, _, ok := a.partFor(w, r, d); ok {
		a.confirm(w, r, http.StatusOK, confirmation{Heading: "lab.d.part-delete", Question: "lab.d.part-delete-ask", Cancel: dataURL(d),
			Action: r.URL.Path}, p.File)
	}
}

// replacePartForm shows the Data tab's upload form for replacing one Part.
func (a *App) replacePartForm(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if p, acc, ok := a.partFor(w, r, d); ok {
		a.renderDashboard(w, r, http.StatusOK, &dashboardPage{Dashboard: d, Tab: dataTab(d), Access: acc, Replacing: p})
	}
}

// uploadPartReplacement checks an uploaded .xlsx's name and size, then hands it to uploadWork to clean and keep pending
// as the replacement of the Part named by {part}.
func (a *App) uploadPartReplacement(w http.ResponseWriter, r *http.Request, d *dashboard) {
	if p, acc, ok := a.partFor(w, r, d); ok {
		a.uploadXlsx(w, r, d, acc, p)
	}
}

// deletePart deletes a Part once its confirmation page is posted: its rows leave data.json, its room is free in the
// Storage allowance and its rows are queued for deletion from the store. The other Parts keep their order, and any
// Dataset from before Parts goes, as the Dashboard's data changed. Deleting the last Part leaves the Dataset empty: its
// columns are cleared to the empty list (NULL is a Dataset never uploaded, which data.json answers 404) and so is its
// recipe, so the next upload sets new ones. An Add waiting to be confirmed was put together from the Parts as they
// were, so it goes too, and so does a Replace of this Part; a Replace of the data, or of another Part, stays.
func (a *App) deletePart(w http.ResponseWriter, r *http.Request, d *dashboard) {
	p, _, ok := a.partFor(w, r, d)
	if !ok || unconfirmed(w, r) {
		return
	}
	gone := errors.New("Part already deleted")
	err := a.inTx(r.Context(), func(tx *sql.Tx) error {
		// Locking the Learner makes deletes, confirms and uploads take turns.
		if _, err := tx.ExecContext(r.Context(), `SELECT 1 FROM learners WHERE id = (SELECT learner_id FROM dashboards WHERE id = $1) FOR UPDATE`, d.ID); err != nil {
			return err
		}
		res, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE id = $1 AND dashboard_id = $2 AND state = 'current'`, p.ID, d.ID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return errors.Join(err, gone)
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dataset_parts WHERE dashboard_id = $1 AND (state = 'earlier'
				OR state = 'pending' AND (cleanup ? 'added' OR cleanup->'replaces'->>'part' = $2::bigint::text))`, d.ID, p.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE dashboards SET updated_at = now(),
				dataset_columns = CASE WHEN left_over.none THEN '[]' ELSE dataset_columns END,
				upload_settings = CASE WHEN left_over.none THEN upload_settings - 'recipe' ELSE upload_settings END
			FROM (SELECT NOT EXISTS (SELECT 1 FROM dataset_parts WHERE dashboard_id = $1 AND state = 'current') AS none) left_over
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
