package app

import (
	"context"
	"errors"
	"net/http"
	"slices"
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
	parts, err := a.dataset.Parts(ctx, dashboardID)
	var list []partRow
	for _, p := range parts {
		list = append(list, partRow{ID: p.ID, File: p.File, Date: p.Date, Rows: p.Rows, Size: p.Size, Added: p.Confirmed.UTC().Format(periodTime)})
	}
	return list, err
}

// partFor applies the Lab access rule, which it returns, and loads the ID, file name and rows of Dashboard d's current
// Part named by {part}. ok is false when it has answered.
func (a *App) partFor(w http.ResponseWriter, r *http.Request, d *dashboard) (p *partRef, acc labAccess, ok bool) {
	acc, ok = a.labOpen(w, r)
	if !ok {
		return nil, acc, false
	}
	id, err := strconv.ParseInt(r.PathValue("part"), 10, 64)
	if err != nil { // not a number: no such Part
		a.notFound(w, r)
		return nil, acc, false
	}
	parts, err := a.dataset.Parts(r.Context(), d.ID)
	if err != nil {
		a.serverError(w, r, err)
		return nil, acc, false
	}
	i := slices.IndexFunc(parts, func(p dataset.Part) bool { return p.ID == id })
	if i < 0 {
		a.notFound(w, r)
		return nil, acc, false
	}
	return &partRef{ID: id, File: parts[i].File, Rows: parts[i].Rows}, acc, true
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
