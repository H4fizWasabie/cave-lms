package app

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// adminResult is one attempt as the Admin's Results page shows it.
type adminResult struct {
	LearnerID, ExamID, CourseID int64
	ID                          int64
	LearnerName, Email          string
	CourseTitle, Format         string
	Number                      int // the Learner's first, second... attempt at the Exam
	Submitted, ReleasedAt       sql.NullTime
	Score                       int // percent, once released
	Passed                      bool
	AdminMarked                 bool // an Admin set a Written mark
}

// Written reports whether the attempt is at a Written Exam.
func (r adminResult) Written() bool { return r.Format == written }

// Released reports whether the attempt's Result is out.
func (r adminResult) Released() bool { return r.ReleasedAt.Valid }

// SubmittedIn and ReleasedIn are the attempt's times in lang, Malaysia time, empty when not yet.
func (r adminResult) SubmittedIn(lang string) string { return malaysiaTime(lang, r.Submitted) }
func (r adminResult) ReleasedIn(lang string) string  { return malaysiaTime(lang, r.ReleasedAt) }

// Status is the catalog key naming where the attempt stands: in progress, waiting for release, passed or failed.
func (r adminResult) Status() string {
	switch {
	case r.Released() && r.Passed:
		return "results.passed"
	case r.Released():
		return "results.failed"
	case r.Submitted.Valid:
		return "results.waiting"
	}
	return "results.in-progress"
}

// MarkedBy is the catalog key naming who marked the attempt: an Admin once they set a Written mark, otherwise the AI
// for a Written attempt and automatically for a Multiple-choice one.
func (r adminResult) MarkedBy() string {
	switch {
	case r.AdminMarked:
		return "results.marked-by.admin"
	case r.Format == written:
		return "results.marked-by.ai"
	}
	return "results.marked-by.auto"
}

type resultsPage struct {
	pageBase
	Results        []adminResult
	Courses        []course // the Courses with an Exam, for the filter
	Formats        []string
	CourseID       int64             // the Course filter, 0 for all
	Format         string            // the Exam filter, empty for all
	Attempt        *adminResult      // the one attempt the detail view shows
	Questions      []attemptQuestion // what that attempt asked and the Learner's answers
	MayGiveAnother bool              // whether that attempt's Learner may be given another attempt
}

// adminResults lists every attempt, newest first, filtered by ?course= and ?format= when given.
func (a *App) adminResults(w http.ResponseWriter, r *http.Request) {
	p := &resultsPage{Format: r.URL.Query().Get("format"), Formats: bankFormats}
	p.CourseID, _ = strconv.ParseInt(r.URL.Query().Get("course"), 10, 64)
	var err error
	if p.Courses, err = a.examCourses(r); err == nil {
		p.Results, err = a.queryResults(r, `($1 = 0 OR x.course_id = $1) AND ($2 = '' OR x.format = $2)`, p.CourseID, p.Format)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_results", p)
}

// adminAttempt shows one attempt: the Learner, its Result, and every question it asked with the Learner's answer and
// the correct option or the model answer and mark.
func (a *App) adminAttempt(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resultFor(w, r)
	if !ok {
		return
	}
	a.showAdminAttempt(w, r, res, http.StatusOK, "")
}

// showAdminAttempt writes the attempt's Admin page with the status and the error message msg, empty for none.
func (a *App) showAdminAttempt(w http.ResponseWriter, r *http.Request, res adminResult, status int, msg string) {
	p := &resultsPage{pageBase: pageBase{Error: msg}, Attempt: &res}
	var raw []byte
	err := a.db.QueryRowContext(r.Context(), `SELECT questions FROM exam_attempts WHERE id = $1`, res.ID).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &p.Questions)
	}
	if err == nil {
		p.MayGiveAnother, err = a.mayGiveAnother(r.Context(), a.db, res)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "admin_attempt", p)
}

// saveMarks sets the Admin's marks, 0 to writtenMarks, on a submitted Written attempt's answers; a blank field leaves
// its mark as it is. The attempt records that an Admin marked it. Once released, its mark and pass or fail are
// recalculated, and a corrected Result email goes out only when pass or fail flips. A new pass issues any Certificate
// it earns, as releasing does.
func (a *App) saveMarks(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resultFor(w, r)
	if !ok {
		return
	}
	back := "/admin/results/" + strconv.FormatInt(res.ID, 10)
	if res.Format != written || !res.Submitted.Valid {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	// Holding the release lock means the release job never releases on marks read before these are saved.
	a.releasing.Lock()
	defer a.releasing.Unlock()
	due, err := a.queryDue(r.Context(), `id = $1`, res.ID)
	if err != nil || len(due) == 0 {
		a.serverError(w, r, cmp.Or(err, errors.New("attempt gone")))
		return
	}
	d := due[0]
	changed := false
	for i := range d.questions {
		q := &d.questions[i]
		v := strings.TrimSpace(r.PostFormValue("mark-" + strconv.FormatInt(q.ID, 10)))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > writtenMarks {
			lang, err := a.language(r)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			a.showAdminAttempt(w, r, res, http.StatusUnprocessableEntity, text.T(lang, "results.err.mark", writtenMarks))
			return
		}
		if q.Mark == nil || *q.Mark != n {
			q.Mark, q.Reason, changed = &n, "", true
		}
	}
	if !changed {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	raw, err := json.Marshal(d.questions)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	score, _ := markAttempt(written, d.questions)
	passed := score >= d.passMark
	// Before release only the answers change; the release job saves the mark.
	_, err = a.db.ExecContext(r.Context(), `UPDATE exam_attempts SET questions = $2::jsonb, admin_marked = true,
		score = CASE WHEN released_at IS NULL THEN NULL ELSE $3::integer END,
		passed = CASE WHEN released_at IS NULL THEN NULL ELSE $4::boolean END WHERE id = $1`, d.attemptID, string(raw), score, passed)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if d.released && passed && !d.passed {
		if err := a.issueCertificateIfEarned(r.Context(), d.learnerID, d.exam.CourseID); err != nil {
			a.logError(r, fmt.Errorf("issue Certificate on correcting attempt %d: %w", d.attemptID, err))
		}
	}
	if d.released && passed != d.passed {
		a.emailResult(r.Context(), d, score, passed, "email.result.corrected-subject")
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// mayGiveAnother reports whether the attempt's Learner has used every attempt at its Exam, each Result released and
// none passed, so the Admin may give them another.
func (a *App) mayGiveAnother(ctx context.Context, q rowQueryer, res adminResult) (bool, error) {
	e, err := a.loadExam(ctx, res.CourseID, res.Format)
	if err != nil {
		return false, err
	}
	st := examStatus{Exam: e}
	if err := st.readAttempts(ctx, q, res.LearnerID, res.ExamID); err != nil {
		return false, err
	}
	return st.usedAll() && !st.Passed && st.Open == nil && st.Pending == nil, nil
}

// giveAnotherAttempt adds one attempt to the attempt's Learner's limit on its Exam, when they may be given one.
func (a *App) giveAnotherAttempt(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resultFor(w, r)
	if !ok {
		return
	}
	// Locking the Exam row serialises gives, so a double click adds one attempt, not two. NO KEY UPDATE leaves
	// Learners free to start attempts meanwhile.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `SELECT 1 FROM exams WHERE id = $1 FOR NO KEY UPDATE`, res.ExamID)
	var may bool
	if err == nil {
		may, err = a.mayGiveAnother(r.Context(), tx, res)
	}
	if err == nil && may {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO exam_extra_attempts (learner_id, exam_id, extra) VALUES ($1, $2, 1)
			ON CONFLICT (learner_id, exam_id) DO UPDATE SET extra = exam_extra_attempts.extra + 1`, res.LearnerID, res.ExamID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/results/"+strconv.FormatInt(res.ID, 10), http.StatusSeeOther)
}

// resultFor loads the attempt named by {attemptID}. When there is none it writes the Not found page and returns false.
func (a *App) resultFor(w http.ResponseWriter, r *http.Request) (adminResult, bool) {
	id, err := strconv.ParseInt(r.PathValue("attemptID"), 10, 64)
	var list []adminResult
	if err == nil {
		if list, err = a.queryResults(r, `t.id = $1`, id); err != nil {
			a.serverError(w, r, err)
			return adminResult{}, false
		}
	}
	if len(list) == 0 {
		a.notFound(w, r)
		return adminResult{}, false
	}
	return list[0], true
}

// examCourses lists the Courses that have an Exam, by title.
func (a *App) examCourses(r *http.Request) ([]course, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT DISTINCT c.id, c.title FROM courses c JOIN exams x ON x.course_id = c.id
		ORDER BY c.title, c.id`)
	if err != nil {
		return nil, fmt.Errorf("load Courses with an Exam: %w", err)
	}
	defer rows.Close()
	var list []course
	for rows.Next() {
		var c course
		if err := rows.Scan(&c.ID, &c.Title); err != nil {
			return nil, fmt.Errorf("load Courses with an Exam: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// queryResults reads the attempts matching the SQL condition where, newest first.
// ponytail: every attempt is numbered before where filters, a full scan; number within the filtered Exams if it gets slow.
func (a *App) queryResults(r *http.Request, where string, args ...any) ([]adminResult, error) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT t.learner_id, t.exam_id, x.course_id, t.id, l.name, l.email, c.title, x.format, t.number, t.submitted, t.released_at,
			COALESCE(t.score, 0), COALESCE(t.passed, false), t.admin_marked
		FROM (SELECT *, row_number() OVER (PARTITION BY learner_id, exam_id ORDER BY id) AS number,
			`+submittedAtSQL+` AS submitted FROM exam_attempts) t
		JOIN exams x ON x.id = t.exam_id
		JOIN learners l ON l.id = t.learner_id
		JOIN courses c ON c.id = x.course_id
		WHERE `+where+`
		ORDER BY t.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("load Results: %w", err)
	}
	defer rows.Close()
	var list []adminResult
	for rows.Next() {
		var res adminResult
		if err := rows.Scan(&res.LearnerID, &res.ExamID, &res.CourseID, &res.ID, &res.LearnerName, &res.Email, &res.CourseTitle, &res.Format, &res.Number,
			&res.Submitted, &res.ReleasedAt, &res.Score, &res.Passed, &res.AdminMarked); err != nil {
			return nil, fmt.Errorf("load Results: %w", err)
		}
		list = append(list, res)
	}
	return list, rows.Err()
}

// malaysiaTime is at in lang, as "2 January 2099, 11:14" Malaysia time, empty when not set.
func malaysiaTime(lang string, at sql.NullTime) string {
	if !at.Valid {
		return ""
	}
	return dateTimeIn(lang, at.Time)
}

// dateTimeIn is at in lang, as "2 January 2099, 11:14" Malaysia time.
func dateTimeIn(lang string, at time.Time) string {
	at = at.In(malaysia)
	return text.Date(lang, at) + ", " + at.Format("15:04")
}
