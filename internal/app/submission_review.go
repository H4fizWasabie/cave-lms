package app

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// submitter is one row of the Admin's Assignment page: an enrolled Learner, or one who left with files.
type submitter struct {
	ID       int64
	Name     string
	Email    string
	Enrolled bool
	Comment  string
	Files    []submissionFile
}

type submissionsPage struct {
	pageBase
	Course     course
	Assignment assignment
	Submitters []submitter
}

// adminSubmissions is an Assignment's page for the Admin: every enrolled Learner and anyone who left with files,
// with their files and the Admin's comment.
func (a *App) adminSubmissions(w http.ResponseWriter, r *http.Request) {
	p, ok := a.adminAssignmentFor(w, r)
	if !ok {
		return
	}
	list, err := a.submitters(r.Context(), p.Assignment.ID, p.Course.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_submissions", &submissionsPage{Course: p.Course, Assignment: p.Assignment, Submitters: list})
}

func (a *App) submitters(ctx context.Context, assignmentID, courseID int64) ([]submitter, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT l.id, l.name, l.email, e.learner_id IS NOT NULL, COALESCE(c.body, '')
		FROM learners l
		LEFT JOIN enrollments e ON e.learner_id = l.id AND e.course_id = $2
		LEFT JOIN submission_comments c ON c.assignment_id = $1 AND c.learner_id = l.id
		WHERE e.learner_id IS NOT NULL
			OR EXISTS (SELECT 1 FROM submission_files f WHERE f.assignment_id = $1 AND f.learner_id = l.id)
		ORDER BY lower(l.name), l.id`, assignmentID, courseID)
	if err != nil {
		return nil, err
	}
	var list []submitter
	at := map[int64]int{}
	for rows.Next() {
		var s submitter
		if err := rows.Scan(&s.ID, &s.Name, &s.Email, &s.Enrolled, &s.Comment); err != nil {
			rows.Close()
			return nil, err
		}
		at[s.ID] = len(list)
		list = append(list, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = a.db.QueryContext(ctx, `SELECT learner_id, id, name, size, uploaded_at FROM submission_files
		WHERE assignment_id = $1 ORDER BY uploaded_at, id`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var learnerID int64
		var f submissionFile
		if err := rows.Scan(&learnerID, &f.ID, &f.Name, &f.Size, &f.UploadedAt); err != nil {
			return nil, err
		}
		if i, ok := at[learnerID]; ok {
			list[i].Files = append(list[i].Files, f)
		}
	}
	return list, rows.Err()
}

// maxComment is the longest comment the Admin may leave on a Submission, in characters.
const maxComment = 5000

// saveSubmissionComment writes, replaces or, when empty, removes the Admin's comment on a Learner's Submission.
func (a *App) saveSubmissionComment(w http.ResponseWriter, r *http.Request) {
	as, ok := a.assignmentByPath(w, r)
	if !ok {
		return
	}
	learnerID, err := strconv.ParseInt(r.PathValue("learnerID"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	var listed bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM enrollments WHERE course_id = $1 AND learner_id = $3)
		OR EXISTS (SELECT 1 FROM submission_files WHERE assignment_id = $2 AND learner_id = $3)`, as.CourseID, as.ID, learnerID).Scan(&listed); err != nil {
		a.serverError(w, r, err)
		return
	}
	if !listed { // only a Learner on the Assignment's page gets a comment
		a.notFound(w, r)
		return
	}
	body := strings.TrimSpace(r.PostFormValue("comment"))
	if utf8.RuneCountInString(body) > maxComment {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusUnprocessableEntity, "message", page{Title: text.T(lang, "adm.submissions.title"),
			Message: text.T(lang, "adm.submissions.err.comment", maxComment)})
		return
	}
	if body == "" {
		_, err = a.db.ExecContext(r.Context(), `DELETE FROM submission_comments WHERE assignment_id = $1 AND learner_id = $2`, as.ID, learnerID)
	} else {
		_, err = a.db.ExecContext(r.Context(), `INSERT INTO submission_comments (assignment_id, learner_id, body) VALUES ($1, $2, $3)
			ON CONFLICT (assignment_id, learner_id) DO UPDATE SET body = EXCLUDED.body, updated_at = now()`, as.ID, learnerID, body)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/assignments/"+strconv.FormatInt(as.ID, 10)+"#learner-"+strconv.FormatInt(learnerID, 10), http.StatusSeeOther)
}

// submissionComment is the Admin's comment on the Learner's Submission, or "" when none.
func (a *App) submissionComment(ctx context.Context, assignmentID, learnerID int64) (string, error) {
	var body string
	err := a.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT body FROM submission_comments
		WHERE assignment_id = $1 AND learner_id = $2), '')`, assignmentID, learnerID).Scan(&body)
	return body, err
}
