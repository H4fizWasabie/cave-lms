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
	CourseID  int64
	TimeLimit int // minutes, 0 for none
	PassMark  int
	Questions []attemptQuestion
	Finished  bool
	TimedOut  bool // unfinished and past the time limit: it can no longer be marked
	Score     int  // percent, set when Finished
	Passed    bool
}

// attemptColumns must be scanned by scanAttempt.
var attemptColumns = `id, course_id, COALESCE(time_limit_minutes, 0), pass_mark, questions,
	finished_at IS NOT NULL, COALESCE(score, 0), COALESCE(passed, false),
	time_limit_minutes IS NOT NULL AND ` + examLateSQL

func scanAttempt(row interface{ Scan(dest ...any) error }) (examAttempt, error) {
	var att examAttempt
	var questions []byte
	err := row.Scan(&att.ID, &att.CourseID, &att.TimeLimit, &att.PassMark, &questions,
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

// examStatusFor gathers the Learner's standing for a Course's Exam.
func (a *App) examStatusFor(ctx context.Context, l *learner, c course) (examStatus, error) {
	var s examStatus
	var err error
	if s.Enrolled, err = a.enrolled(ctx, l.ID, c.ID); err != nil {
		return s, err
	}
	if s.Exam, err = a.loadExam(ctx, c.ID); err != nil {
		return s, err
	}
	if s.Done, s.Total, err = a.courseProgress(ctx, l.ID, c.ID); err != nil {
		return s, err
	}
	if err = s.readAttempts(ctx, a.db, l.ID, c.ID); err != nil {
		return s, err
	}
	s.Best, s.HasBest, err = a.bestExamScore(ctx, l.ID, c.ID)
	return s, err
}

// readAttempts fills in what the Learner's attempts at a Course's Exam decide: how many, whether one passed and
// the one still open, if any.
func (s *examStatus) readAttempts(ctx context.Context, q rowQueryer, learnerID, courseID int64) error {
	if err := q.QueryRowContext(ctx, `SELECT count(*), COALESCE(bool_or(passed), false) FROM exam_attempts
		WHERE learner_id = $1 AND course_id = $2`, learnerID, courseID).Scan(&s.Used, &s.Passed); err != nil {
		return fmt.Errorf("count attempts: %w", err)
	}
	var err error
	s.Open, err = openAttempt(ctx, q, learnerID, courseID)
	return err
}

// examPassed reports whether the Learner has passed the Course's Exam on any attempt.
func (a *App) examPassed(ctx context.Context, learnerID, courseID int64) (bool, error) {
	var passed bool
	err := a.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM exam_attempts WHERE learner_id = $1 AND course_id = $2 AND passed)`,
		learnerID, courseID).Scan(&passed)
	return passed, err
}

// bestExamScore returns the Learner's highest finished score on a Course's Exam, and false when there is none.
func (a *App) bestExamScore(ctx context.Context, learnerID, courseID int64) (int, bool, error) {
	var best sql.NullInt64
	err := a.db.QueryRowContext(ctx,
		`SELECT max(score) FROM exam_attempts WHERE learner_id = $1 AND course_id = $2`, learnerID, courseID).Scan(&best)
	return int(best.Int64), best.Valid, err
}

// openAttempt returns the Learner's latest unfinished attempt if it is still within its time limit, or nil.
func openAttempt(ctx context.Context, q rowQueryer, learnerID, courseID int64) (*examAttempt, error) {
	att, err := scanAttempt(q.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM exam_attempts
		WHERE learner_id = $1 AND course_id = $2 AND finished_at IS NULL ORDER BY id DESC LIMIT 1`, learnerID, courseID))
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

// learnerExam is the Learner's Exam page for a Course.
func (a *App) learnerExam(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, c)
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
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, c)
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
	if err = st.readAttempts(r.Context(), tx, learner.ID, c.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if st.Open != nil {
		http.Redirect(w, r, attemptPath(c.ID, st.Open.ID), http.StatusSeeOther)
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
	id, err := insertAttempt(r.Context(), tx, learner.ID, c.ID, st.Exam.Questions)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, attemptPath(c.ID, id), http.StatusSeeOther)
}

// insertAttempt stores a new attempt with the published questions of list in a shuffled order.
func insertAttempt(ctx context.Context, tx *sql.Tx, learnerID, courseID int64, list []examQuestion) (int64, error) {
	questions := []attemptQuestion{}
	for _, q := range list {
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
		INSERT INTO exam_attempts (learner_id, course_id, time_limit_minutes, pass_mark, questions)
		SELECT $1, course_id, time_limit_minutes, pass_mark, $3::jsonb FROM exams WHERE course_id = $2
		RETURNING id`, learnerID, courseID, string(raw)).Scan(&id)
	return id, err
}

// showAttempt shows an attempt to its Learner: the questions while it is open, the result once it is marked.
func (a *App) showAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	att, ok := a.attemptFor(w, r, learner.ID, c.ID)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_attempt", page{Course: &c, Learner: learner, Attempt: &att})
}

// submitAttempt marks an open attempt. A submission after the time limit and grace is not marked: the
// attempt then shows as timed out.
func (a *App) submitAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	att, ok := a.attemptFor(w, r, learner.ID, c.ID)
	if !ok {
		return
	}
	if att.Finished {
		http.Redirect(w, r, attemptPath(c.ID, att.ID), http.StatusSeeOther)
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !enrolled {
		http.Redirect(w, r, "/courses/"+strconv.FormatInt(c.ID, 10)+"/exam", http.StatusSeeOther)
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
	http.Redirect(w, r, attemptPath(c.ID, att.ID), http.StatusSeeOther)
}

// attemptFor loads the Learner's attempt named by {attemptID} on the Course. Other Learners' attempts are not found.
func (a *App) attemptFor(w http.ResponseWriter, r *http.Request, learnerID, courseID int64) (examAttempt, bool) {
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
		WHERE id = $1 AND learner_id = $2 AND course_id = $3`, id, learnerID, courseID))
	if errors.Is(err, sql.ErrNoRows) {
		return notFound()
	}
	if err != nil {
		a.serverError(w, r, err)
		return examAttempt{}, false
	}
	return att, true
}

func attemptPath(courseID, attemptID int64) string {
	return "/courses/" + strconv.FormatInt(courseID, 10) + "/exam/attempts/" + strconv.FormatInt(attemptID, 10)
}
