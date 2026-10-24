package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// assignment is a task in a Course that Learners answer by uploading files. Drafts (Published false) are shown to
// the Admin only. Instructions are plain text, shown with their line breaks.
type assignment struct {
	ID           int64
	CourseID     int64
	Position     int
	Title        string
	Instructions string
	Published    bool
}

const assignmentColumns = `id, course_id, position, title, instructions, published`

func scanAssignment(row interface{ Scan(...any) error }) (assignment, error) {
	var as assignment
	err := row.Scan(&as.ID, &as.CourseID, &as.Position, &as.Title, &as.Instructions, &as.Published)
	return as, err
}

// assignmentPage is what the Assignment pages read, the Admin's and the Learner's.
type assignmentPage struct {
	pageBase
	Course      course
	Assignment  assignment
	Assignments []assignment
	Enrolled    bool             // the Learner may change their Submission
	Files       []submissionFile // the Learner's Submission
	Comment     string           // the Admin's comment on it
}

// path is the Assignment's page for Learners.
func (p *assignmentPage) path() string { return assignmentPath(p.Course.ID, p.Assignment.ID) }

func assignmentPath(courseID, assignmentID int64) string {
	return "/courses/" + strconv.FormatInt(courseID, 10) + "/assignments/" + strconv.FormatInt(assignmentID, 10)
}

// assignmentsOf lists a Course's Assignments in order. Learners get only the published ones.
func (a *App) assignmentsOf(ctx context.Context, courseID int64, includeDrafts bool) ([]assignment, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+assignmentColumns+` FROM assignments
		WHERE course_id = $1 AND (published OR $2) ORDER BY position, id`, courseID, includeDrafts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []assignment
	for rows.Next() {
		as, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, as)
	}
	return list, rows.Err()
}

// assignmentByPath loads the Assignment named by the {aid} path value. When there is none it writes the Not found
// page and returns false.
func (a *App) assignmentByPath(w http.ResponseWriter, r *http.Request) (assignment, bool) {
	id, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return assignment{}, false
	}
	as, err := scanAssignment(a.db.QueryRowContext(r.Context(), `SELECT `+assignmentColumns+` FROM assignments WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return assignment{}, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return assignment{}, false
	}
	return as, true
}

// adminAssignmentFor loads the Assignment named by {aid} and its Course, for the Admin's pages.
func (a *App) adminAssignmentFor(w http.ResponseWriter, r *http.Request) (assignmentPage, bool) {
	as, ok := a.assignmentByPath(w, r)
	if !ok {
		return assignmentPage{}, false
	}
	c, err := a.courseByID(r.Context(), as.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return assignmentPage{}, false
	}
	return assignmentPage{Course: c, Assignment: as}, true
}

func assignmentsPath(courseID int64) string {
	return "/admin/courses/" + strconv.FormatInt(courseID, 10) + "/assignments"
}

// adminAssignments lists every Assignment of a Course, drafts included, for the Admin to arrange.
func (a *App) adminAssignments(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	list, err := a.assignmentsOf(r.Context(), c.ID, true)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_assignments", &assignmentPage{Course: c, Assignments: list})
}

func (a *App) newAssignmentForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "assignment_form", &assignmentPage{Course: c})
}

// assignmentFromForm reads the Assignment form. msg is the first problem, in lang, empty when valid.
func assignmentFromForm(r *http.Request, lang string) (as assignment, msg string) {
	as.Title = strings.TrimSpace(r.PostFormValue("title"))
	as.Instructions = strings.TrimSpace(r.PostFormValue("instructions"))
	switch {
	case as.Title == "" || utf8.RuneCountInString(as.Title) > 200:
		msg = text.T(lang, "adm.err.title-200")
	case as.Instructions == "" || utf8.RuneCountInString(as.Instructions) > 20000:
		msg = text.T(lang, "adm.assignments.err.instructions")
	}
	return as, msg
}

// createAssignment adds an Assignment at the end of the Course, as a Draft.
func (a *App) createAssignment(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	as, msg := assignmentFromForm(r, lang)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "assignment_form", &assignmentPage{Course: c, Assignment: as, pageBase: pageBase{Error: msg}})
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `INSERT INTO assignments (course_id, position, title, instructions)
		VALUES ($1, (SELECT COALESCE(max(position), 0) + 1 FROM assignments WHERE course_id = $1), $2, $3)`,
		c.ID, as.Title, as.Instructions); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, assignmentsPath(c.ID), http.StatusSeeOther)
}

func (a *App) editAssignmentForm(w http.ResponseWriter, r *http.Request) {
	p, ok := a.adminAssignmentFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "assignment_form", &p)
}

func (a *App) updateAssignment(w http.ResponseWriter, r *http.Request) {
	p, ok := a.adminAssignmentFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	as, msg := assignmentFromForm(r, lang)
	p.Assignment.Title, p.Assignment.Instructions = as.Title, as.Instructions
	if msg != "" {
		p.Error = msg
		a.render(w, r, http.StatusUnprocessableEntity, "assignment_form", &p)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE assignments SET title = $2, instructions = $3 WHERE id = $1`,
		p.Assignment.ID, as.Title, as.Instructions); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, assignmentsPath(p.Course.ID), http.StatusSeeOther)
}

// moveAssignment returns the handler that swaps an Assignment with its neighbour: dir -1 is up, +1 is down.
func (a *App) moveAssignment(dir int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		as, ok := a.assignmentByPath(w, r)
		if !ok {
			return
		}
		if err := a.swapWithNeighbour(r.Context(), "assignments", as.ID, as.CourseID, as.Position, dir); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, assignmentsPath(as.CourseID), http.StatusSeeOther)
	}
}

// setAssignmentPublished returns the handler that publishes an Assignment to Learners or turns it back into a Draft.
func (a *App) setAssignmentPublished(published bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		as, ok := a.assignmentByPath(w, r)
		if !ok {
			return
		}
		if _, err := a.db.ExecContext(r.Context(), `UPDATE assignments SET published = $2 WHERE id = $1`, as.ID, published); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, assignmentsPath(as.CourseID), http.StatusSeeOther)
	}
}

// confirmDeleteAssignment asks before deleting, warning that the Learners' Submissions go with it.
func (a *App) confirmDeleteAssignment(w http.ResponseWriter, r *http.Request) {
	p, ok := a.adminAssignmentFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "delete_assignment", &p)
}

func (a *App) deleteAssignment(w http.ResponseWriter, r *http.Request) {
	as, ok := a.assignmentByPath(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM assignments WHERE id = $1`, as.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, assignmentsPath(as.CourseID), http.StatusSeeOther)
}

// learnerAssignmentFor loads the Course and Assignment named by the path for the logged-in Learner: the Admin sees
// any, an enrolled Learner a published one. Other Learners are asked to join; a Draft is Not found to Learners.
func (a *App) learnerAssignmentFor(w http.ResponseWriter, r *http.Request) (assignmentPage, bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return assignmentPage{}, false
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return assignmentPage{}, false
	}
	as, ok := a.assignmentByPath(w, r)
	if !ok {
		return assignmentPage{}, false
	}
	if as.CourseID != c.ID || (!as.Published && !learner.IsAdmin) {
		a.notFound(w, r)
		return assignmentPage{}, false
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return assignmentPage{}, false
	}
	if !enrolled && !learner.IsAdmin {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return assignmentPage{}, false
		}
		a.render(w, r, http.StatusForbidden, "members_only", page{Learner: learner, Course: &c,
			Message: text.T(lang, "assignment.join-to-open")})
		return assignmentPage{}, false
	}
	return assignmentPage{pageBase: pageBase{Learner: learner}, Course: c, Assignment: as, Enrolled: enrolled}, true
}

// showAssignment is an Assignment's page for an enrolled Learner or the Admin.
func (a *App) showAssignment(w http.ResponseWriter, r *http.Request) {
	p, ok := a.learnerAssignmentFor(w, r)
	if !ok {
		return
	}
	a.renderAssignment(w, r, http.StatusOK, &p)
}

// renderAssignment renders the Assignment page with the Learner's Submission.
func (a *App) renderAssignment(w http.ResponseWriter, r *http.Request, status int, p *assignmentPage) {
	var err error
	if p.Files, err = a.submissionFiles(r.Context(), p.Assignment.ID, p.Learner.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if p.Comment, err = a.submissionComment(r.Context(), p.Assignment.ID, p.Learner.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "assignment", p)
}
