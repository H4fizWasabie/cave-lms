package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/dataset"
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

// deletePart deletes a Part once its confirmation page is posted, as the Dataset module does (Delete): its rows leave
// data.json, its room is free in the Storage allowance and its rows are queued for deletion from the store.
func (a *App) deletePart(w http.ResponseWriter, r *http.Request, d *dashboard) {
	p, _, ok := a.partFor(w, r, d)
	if !ok || unconfirmed(w, r) {
		return
	}
	err := a.dataset.Delete(r.Context(), d.ID, p.ID)
	if errors.Is(err, dataset.ErrPartGone) {
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
