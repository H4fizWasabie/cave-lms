package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// examGraceSeconds is how long after the time limit a submission is still accepted, for network delay.
const examGraceSeconds = 30

// examLateSQL is true once an attempt is past its time limit plus the grace period. It is only meaningful
// for an attempt with a time limit.
var examLateSQL = fmt.Sprintf("now() > started_at + make_interval(mins => time_limit_minutes, secs => %d)", examGraceSeconds)

// attemptQuestion is one question as an attempt showed it. The attempt keeps its own copy, so later Exam
// edits never change it. Chosen is the option slot the Learner picked, or -1 when unanswered.
type attemptQuestion struct {
	ID      int64
	Prompt  string
	Options [5]examOption
	Chosen  int
}

// Right reports whether the Learner's answer is the correct option.
func (q attemptQuestion) Right() bool {
	return q.Chosen >= 0 && q.Options[q.Chosen].Correct
}

// Answer is the text of the option the Learner picked, empty when unanswered.
func (q attemptQuestion) Answer() string {
	if q.Chosen < 0 {
		return ""
	}
	return q.Options[q.Chosen].Text
}

// examAttempt is one sit of an Exam, from start to finish.
type examAttempt struct {
	ID        int64
	ExamID    int64
	TimeLimit int // minutes, 0 for none
	PassMark  int
	Questions []attemptQuestion
	Finished  bool
	TimedOut  bool // unfinished and past the time limit: it can no longer be marked
	Score     int  // percent, set when Finished
	Passed    bool
}

// attemptColumns must be scanned by scanAttempt.
var attemptColumns = `id, exam_id, COALESCE(time_limit_minutes, 0), pass_mark, questions,
	finished_at IS NOT NULL, COALESCE(score, 0), COALESCE(passed, false),
	time_limit_minutes IS NOT NULL AND ` + examLateSQL

func scanAttempt(row interface{ Scan(dest ...any) error }) (examAttempt, error) {
	var att examAttempt
	var questions []byte
	err := row.Scan(&att.ID, &att.ExamID, &att.TimeLimit, &att.PassMark, &questions,
		&att.Finished, &att.Score, &att.Passed, &att.TimedOut)
	if err != nil {
		return examAttempt{}, err
	}
	return att, json.Unmarshal(questions, &att.Questions)
}

// examStatus is what the Learner's Exam page needs to decide what they may do next.
type examStatus struct {
	Enrolled bool
	Exam     exam
	Done     int
	Total    int
	Passed   bool
	Used     int // attempts started, finished or not
	Open     *examAttempt
	Best     int
	HasBest  bool
}

// maxAttempts is the attempt limit, 0 for unlimited.
func (s examStatus) maxAttempts() int {
	n, _ := strconv.Atoi(s.Exam.MaxAttempts)
	return n
}

// AttemptsLeft is the number of attempts still allowed, or "Unlimited" in lang.
func (s examStatus) AttemptsLeft(lang string) string {
	if s.maxAttempts() == 0 {
		return text.T(lang, "exam.unlimited")
	}
	return strconv.Itoa(max(s.maxAttempts()-s.Used, 0))
}

// Blocker is the first reason, in lang, the Learner may not start an attempt, empty when they may.
func (s examStatus) Blocker(lang string) string {
	switch {
	case !s.Enrolled:
		return text.T(lang, "exam.blocked.join")
	case !s.Exam.Usable:
		return text.T(lang, "exam.blocked.not-ready")
	case s.Total == 0:
		return text.T(lang, "exam.blocked.no-lessons")
	case s.Done < s.Total:
		return text.T(lang, "exam.blocked.lessons", s.Done, s.Total)
	case s.Passed:
		return text.T(lang, "exam.blocked.passed")
	case s.maxAttempts() > 0 && s.Used >= s.maxAttempts():
		return text.T(lang, "exam.blocked.no-attempts")
	}
	return ""
}

// examStatusFor gathers the Learner's standing for an Exam.
func (a *App) examStatusFor(ctx context.Context, l *learner, e exam) (examStatus, error) {
	s := examStatus{Exam: e}
	var err error
	if s.Enrolled, err = a.enrolled(ctx, l.ID, e.CourseID); err != nil {
		return s, err
	}
	if s.Done, s.Total, err = a.courseProgress(ctx, l.ID, e.CourseID); err != nil {
		return s, err
	}
	if err = s.readAttempts(ctx, a.db, l.ID, e.ID); err != nil {
		return s, err
	}
	var best sql.NullInt64
	err = a.db.QueryRowContext(ctx,
		`SELECT max(score) FROM exam_attempts WHERE learner_id = $1 AND exam_id = $2`, l.ID, e.ID).Scan(&best)
	s.Best, s.HasBest = int(best.Int64), best.Valid
	return s, err
}

// readAttempts fills in what the Learner's attempts at an Exam decide: how many, whether one passed and the one
// still open, if any.
func (s *examStatus) readAttempts(ctx context.Context, q rowQueryer, learnerID, examID int64) error {
	if err := q.QueryRowContext(ctx, `SELECT count(*), COALESCE(bool_or(passed), false) FROM exam_attempts
		WHERE learner_id = $1 AND exam_id = $2`, learnerID, examID).Scan(&s.Used, &s.Passed); err != nil {
		return fmt.Errorf("count attempts: %w", err)
	}
	var err error
	s.Open, err = openAttempt(ctx, q, learnerID, examID)
	return err
}

// examPassed reports whether the Learner has passed the Exam on any attempt.
func (a *App) examPassed(ctx context.Context, learnerID, examID int64) (bool, error) {
	var passed bool
	err := a.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM exam_attempts WHERE learner_id = $1 AND exam_id = $2 AND passed)`,
		learnerID, examID).Scan(&passed)
	return passed, err
}

// bestExamScore returns the Learner's highest finished score on any of a Course's Exams, and false when there is none.
func (a *App) bestExamScore(ctx context.Context, learnerID, courseID int64) (int, bool, error) {
	var best sql.NullInt64
	err := a.db.QueryRowContext(ctx, `SELECT max(t.score) FROM exam_attempts t JOIN exams x ON x.id = t.exam_id
		WHERE t.learner_id = $1 AND x.course_id = $2`, learnerID, courseID).Scan(&best)
	return int(best.Int64), best.Valid, err
}

// openAttempt returns the Learner's latest unfinished attempt if it is still within its time limit, or nil.
func openAttempt(ctx context.Context, q rowQueryer, learnerID, examID int64) (*examAttempt, error) {
	att, err := scanAttempt(q.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM exam_attempts
		WHERE learner_id = $1 AND exam_id = $2 AND finished_at IS NULL ORDER BY id DESC LIMIT 1`, learnerID, examID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load open attempt: %w", err)
	}
	if att.TimedOut {
		return nil, nil
	}
	return &att, nil
}

// learnerExam is the Learner's page for an Exam.
func (a *App) learnerExam(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, e)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "learner_exam", page{Course: &c, Learner: learner, Status: &st})
}

// startAttempt begins an attempt, or returns to the one still open.
func (a *App) startAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, e)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// Locking the Enrollment row serialises starts by one Learner on one Course, so two quick clicks can never
	// both pass the open-attempt and attempt-limit checks. The lock is held until the new attempt is committed.
	// The Enrollment and the attempts, which another tab can change while this one waits, are read again under the
	// lock, on the transaction: a read on a second connection while this one holds the lock stalls once a burst of
	// starts has taken the whole pool.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer tx.Rollback()
	var locked int
	err = tx.QueryRowContext(r.Context(), `SELECT 1 FROM enrollments WHERE learner_id = $1 AND course_id = $2 FOR UPDATE`,
		learner.ID, c.ID).Scan(&locked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, r, err)
		return
	}
	st.Enrolled = err == nil
	if err = st.readAttempts(r.Context(), tx, learner.ID, e.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if st.Open != nil {
		http.Redirect(w, r, attemptPath(e, st.Open.ID), http.StatusSeeOther)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if msg := st.Blocker(lang); msg != "" {
		a.render(w, r, http.StatusConflict, "learner_exam", page{Course: &c, Learner: learner, Status: &st, Error: msg})
		return
	}
	id, err := insertAttempt(r.Context(), tx, learner.ID, e)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, attemptPath(e, id), http.StatusSeeOther)
}

// insertAttempt stores a new attempt at the Exam with its published questions in a shuffled order.
func insertAttempt(ctx context.Context, tx *sql.Tx, learnerID int64, e exam) (int64, error) {
	questions := []attemptQuestion{}
	for _, q := range e.Questions {
		if !q.Draft {
			questions = append(questions, attemptQuestion{ID: q.ID, Prompt: q.Prompt, Options: q.Options, Chosen: -1})
		}
	}
	rand.Shuffle(len(questions), func(i, j int) { questions[i], questions[j] = questions[j], questions[i] })
	raw, err := json.Marshal(questions)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exam_attempts (learner_id, exam_id, time_limit_minutes, pass_mark, questions)
		SELECT $1, id, time_limit_minutes, pass_mark, $3::jsonb FROM exams WHERE id = $2
		RETURNING id`, learnerID, e.ID, string(raw)).Scan(&id)
	return id, err
}

// showAttempt shows an attempt to its Learner: the questions while it is open, the result once it is marked.
func (a *App) showAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	att, ok := a.attemptFor(w, r, learner.ID, e.ID)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_attempt", page{Course: &c, Exam: &e, Learner: learner, Attempt: &att})
}

// submitAttempt marks an open attempt. A submission after the time limit and grace is not marked: the
// attempt then shows as timed out.
func (a *App) submitAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	att, ok := a.attemptFor(w, r, learner.ID, e.ID)
	if !ok {
		return
	}
	if att.Finished {
		http.Redirect(w, r, attemptPath(e, att.ID), http.StatusSeeOther)
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !enrolled {
		http.Redirect(w, r, e.Path(""), http.StatusSeeOther)
		return
	}
	correct := 0
	for i := range att.Questions {
		q := &att.Questions[i]
		q.Chosen = -1
		if slot, err := strconv.Atoi(r.PostFormValue("answer-" + strconv.FormatInt(q.ID, 10))); err == nil &&
			slot >= 0 && slot < len(q.Options) && q.Options[slot].Text != "" {
			q.Chosen = slot
		}
		if q.Right() {
			correct++
		}
	}
	score := 0
	if n := len(att.Questions); n > 0 {
		score = correct * 100 / n
	}
	raw, err := json.Marshal(att.Questions)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The time check is in the same UPDATE, so a late submission marks nothing.
	_, err = a.db.ExecContext(r.Context(), `
		UPDATE exam_attempts SET questions = $2::jsonb, finished_at = now(), score = $3, passed = $4
		WHERE id = $1 AND finished_at IS NULL AND (time_limit_minutes IS NULL OR NOT (`+examLateSQL+`))`,
		att.ID, string(raw), score, score >= att.PassMark)
	if err == nil {
		err = a.issueCertificateIfEarned(r.Context(), learner.ID, c.ID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, attemptPath(e, att.ID), http.StatusSeeOther)
}

// attemptFor loads the Learner's attempt named by {attemptID} on the Exam. Other Learners' attempts are not found.
func (a *App) attemptFor(w http.ResponseWriter, r *http.Request, learnerID, examID int64) (examAttempt, bool) {
	notFound := func() (examAttempt, bool) {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return examAttempt{}, false
		}
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "exam.attempt-not-found.title"), Message: text.T(lang, "exam.attempt-not-found")})
		return examAttempt{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("attemptID"), 10, 64)
	if err != nil {
		return notFound()
	}
	att, err := scanAttempt(a.db.QueryRowContext(r.Context(), `SELECT `+attemptColumns+` FROM exam_attempts
		WHERE id = $1 AND learner_id = $2 AND exam_id = $3`, id, learnerID, examID))
	if errors.Is(err, sql.ErrNoRows) {
		return notFound()
	}
	if err != nil {
		a.serverError(w, r, err)
		return examAttempt{}, false
	}
	return att, true
}

func attemptPath(e exam, attemptID int64) string {
	return e.Path("/attempts/" + strconv.FormatInt(attemptID, 10))
}

// myExam is one row of My exams: an enrolled Course's Exam and how the Learner stands on it.
type myExam struct {
	CourseID    int64
	Path        string // the Exam's page
	CourseTitle string
	PassMark    string
	Attempted   bool // a finished attempt exists, so Best is meaningful
	Best        int
	Passed      bool
}

// myExamsPage lists the Exams of the Learner's Courses.
type myExamsPage struct {
	pageBase
	Exams []myExam
}

// myExams lists the Exams of every Course the logged-in Learner has joined. An Exam is listed once it has a
// published question.
func (a *App) myExams(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT c.id, x.format, c.title, x.pass_mark, max(t.score) IS NOT NULL, COALESCE(max(t.score), 0), COALESCE(bool_or(t.passed), false)
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		JOIN exams x ON x.course_id = c.id
		LEFT JOIN exam_attempts t ON t.exam_id = x.id AND t.learner_id = e.learner_id
		WHERE e.learner_id = $1
		  AND EXISTS (SELECT 1 FROM exam_questions q WHERE q.exam_id = x.id AND NOT q.is_draft)
		GROUP BY c.id, c.title, x.id
		ORDER BY c.title, c.id, x.format`, learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	p := myExamsPage{pageBase: pageBase{Learner: learner}}
	for rows.Next() {
		var e myExam
		var x exam
		if err := rows.Scan(&x.CourseID, &x.Format, &e.CourseTitle, &e.PassMark, &e.Attempted, &e.Best, &e.Passed); err != nil {
			a.serverError(w, r, err)
			return
		}
		e.CourseID, e.Path = x.CourseID, x.Path("")
		p.Exams = append(p.Exams, e)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_exams", &p)
}
