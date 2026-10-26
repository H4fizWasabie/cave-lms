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
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// examGraceSeconds is how long after the time limit a submission is still accepted, for network delay.
const examGraceSeconds = 30

// examLateSQL is true once an attempt is past its time limit plus the grace period. It is only meaningful
// for an attempt with a time limit.
var examLateSQL = fmt.Sprintf("now() > started_at + make_interval(mins => time_limit_minutes, secs => %d)", examGraceSeconds)

// attemptQuestion is one question as an attempt showed it. The attempt keeps its own copy, so later Exam
// edits never change it. Chosen is the option slot the Learner picked, or -1 when unanswered. A Written question
// has a ModelAnswer, the Learner's Answer in their own words, and its Mark out of writtenMarks once marked.
type attemptQuestion struct {
	ID          int64
	Prompt      string
	Options     [5]examOption
	Chosen      int
	ModelAnswer string `json:",omitempty"`
	Answer      string `json:",omitempty"`
	Mark        *int   `json:",omitempty"`
	Reason      string `json:",omitempty"` // the AI's one-line reason for Mark, empty once an Admin sets it
}

// Blank reports whether the Learner left the question unanswered.
func (q attemptQuestion) Blank() bool { return q.Chosen < 0 && q.Answer == "" }

// Marked reports whether the Written question has a mark.
func (q attemptQuestion) Marked() bool { return q.Mark != nil }

// MarkValue is the Written question's mark, 0 while unmarked.
func (q attemptQuestion) MarkValue() int {
	if q.Mark == nil {
		return 0
	}
	return *q.Mark
}

// maxWrittenAnswer is the most characters a Written answer keeps.
const maxWrittenAnswer = 5000

// Right reports whether the Learner's answer is the correct option.
func (q attemptQuestion) Right() bool {
	return q.Chosen >= 0 && q.Options[q.Chosen].Correct
}

// examAttempt is one sit of an Exam, from start to its released Result.
type examAttempt struct {
	ID          int64
	ExamID      int64
	TimeLimit   int // minutes, 0 for none
	SecondsLeft int // of the time limit, while open
	PassMark    int
	Questions   []attemptQuestion
	Finished    bool      // submitted
	TimedOut    bool      // unsubmitted and past the time limit: it counts as submitted at its limit, with no answers
	ReleaseAt   time.Time // when the Result is released, once Finished or TimedOut
	Released    bool
	Score       int // percent, set when Released
	Passed      bool
	Blank       int // questions left unanswered, set only while asking the Learner to confirm submitting them
}

// FirstBlank is the number, from 1, of the first question left unanswered, 0 when none is.
func (att examAttempt) FirstBlank() int {
	for i, q := range att.Questions {
		if q.Blank() {
			return i + 1
		}
	}
	return 0
}

// blanks counts the questions left unanswered.
func (att examAttempt) blanks() (n int) {
	for _, q := range att.Questions {
		if q.Blank() {
			n++
		}
	}
	return n
}

// attemptColumns must be scanned by scanAttempt.
var attemptColumns = `id, exam_id, COALESCE(time_limit_minutes, 0), pass_mark, questions,
	finished_at IS NOT NULL, COALESCE(score, 0), COALESCE(passed, false),
	finished_at IS NULL AND time_limit_minutes IS NOT NULL AND ` + examLateSQL + `,
	COALESCE(` + submittedAtSQL + `, now()) + ` + resultDelaySQL + `,
	released_at IS NOT NULL,
	GREATEST(EXTRACT(EPOCH FROM started_at + make_interval(mins => COALESCE(time_limit_minutes, 0)) - now()), 0)::int`

func scanAttempt(row interface{ Scan(dest ...any) error }) (examAttempt, error) {
	var att examAttempt
	var questions []byte
	err := row.Scan(&att.ID, &att.ExamID, &att.TimeLimit, &att.PassMark, &questions,
		&att.Finished, &att.Score, &att.Passed, &att.TimedOut, &att.ReleaseAt, &att.Released, &att.SecondsLeft)
	if err != nil {
		return examAttempt{}, err
	}
	return att, json.Unmarshal(questions, &att.Questions)
}

// examStatus is what the Learner's Exam page needs to decide what they may do next.
type examStatus struct {
	Enrolled bool
	Course   course
	Exam     exam
	Done     int
	Total    int
	Passed   bool
	Used     int // attempts started, finished or not
	Extra    int // attempts the Admin gave on top of the Exam's limit
	Open     *examAttempt
	Pending  *examAttempt // submitted, its Result not yet released
	Best     int
	HasBest  bool
	Asked    int // questions an attempt asks: the first attempt's, or the draw count of the published pool before one
}

// maxAttempts is the Learner's attempt limit, the Exam's plus any the Admin gave, 0 for unlimited.
func (s examStatus) maxAttempts() int {
	n, _ := strconv.Atoi(s.Exam.MaxAttempts)
	if n == 0 {
		return 0
	}
	return n + s.Extra
}

// usedAll reports whether the Learner has used every attempt they are allowed.
func (s examStatus) usedAll() bool {
	return s.maxAttempts() > 0 && s.Used >= s.maxAttempts()
}

// AttemptsLeft is the number of attempts still allowed, or "Unlimited" in lang.
func (s examStatus) AttemptsLeft(lang string) string {
	if s.maxAttempts() == 0 {
		return text.T(lang, "exam.unlimited")
	}
	return strconv.Itoa(max(s.maxAttempts()-s.Used, 0))
}

// StartNote says, in lang, how many attempts the Learner has used and what starting one more uses, empty for a first
// attempt at an Exam with unlimited attempts.
func (s examStatus) StartNote(lang string) string {
	limit := s.maxAttempts()
	switch {
	case limit == 0 && s.Used == 1:
		return text.T(lang, "exam.used-one")
	case limit == 0 && s.Used > 1:
		return text.T(lang, "exam.used-many", s.Used)
	case limit == 0:
		return ""
	case s.Used == 0:
		return text.T(lang, "exam.starting-uses", limit)
	case limit-s.Used == 1:
		return text.T(lang, "exam.used-last", s.Used, limit)
	}
	return text.T(lang, "exam.used-of", s.Used, limit, limit-s.Used)
}

// CanStart reports whether the Learner may start an attempt now: Blocker has no reason against it.
func (s examStatus) CanStart() bool { return s.Blocker(text.Languages[0]) == "" }

// Blocker is the first reason, in lang, the Learner may not start an attempt, empty when they may.
func (s examStatus) Blocker(lang string) string {
	switch {
	case !s.Enrolled:
		return text.T(lang, "exam.blocked.join")
	case s.Pending != nil:
		return text.T(lang, "exam.blocked.pending", s.Pending.ReadyAt(lang))
	case s.Passed: // before the window, so a Learner who passed still sees it once the Live Course ends
		return text.T(lang, "exam.blocked.passed")
	case !s.Exam.Usable:
		return text.T(lang, "exam.blocked.not-ready")
	case s.Course.Live() && s.Course.Upcoming:
		return text.T(lang, "exam.blocked.not-open", s.Course.StartOn(lang))
	case s.Course.Live() && s.Course.Ended:
		return text.T(lang, "exam.blocked.closed", s.Course.EndOn(lang))
	// A Live Course's Exam is open between its dates without the Lessons first (ADR 0012).
	case !s.Course.Live() && s.Total == 0:
		return text.T(lang, "exam.blocked.no-lessons")
	case !s.Course.Live() && s.Done < s.Total:
		return text.T(lang, "exam.blocked.lessons", s.Done, s.Total)
	case s.usedAll():
		return text.T(lang, "exam.blocked.no-attempts")
	}
	return ""
}

// examStatusFor gathers the Learner's standing for an Exam.
func (a *App) examStatusFor(ctx context.Context, l *learner, c course, e exam) (examStatus, error) {
	s := examStatus{Course: c, Exam: e}
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
	if err != nil {
		return s, err
	}
	err = a.db.QueryRowContext(ctx, `SELECT jsonb_array_length(questions) FROM exam_attempts
		WHERE learner_id = $1 AND exam_id = $2 ORDER BY id LIMIT 1`, l.ID, e.ID).Scan(&s.Asked)
	if errors.Is(err, sql.ErrNoRows) {
		s.Asked, err = e.drawSize(), nil
	}
	return s, err
}

// readAttempts fills in what the Learner's attempts at an Exam decide: how many, any extra ones the Admin gave,
// whether one passed, and the one still open or waiting for its Result, if any.
func (s *examStatus) readAttempts(ctx context.Context, q rowQueryer, learnerID, examID int64) error {
	if err := q.QueryRowContext(ctx, `SELECT count(*), COALESCE(bool_or(passed), false),
		COALESCE((SELECT extra FROM exam_extra_attempts WHERE learner_id = $1 AND exam_id = $2), 0)
		FROM exam_attempts WHERE learner_id = $1 AND exam_id = $2`, learnerID, examID).Scan(&s.Used, &s.Passed, &s.Extra); err != nil {
		return fmt.Errorf("count attempts: %w", err)
	}
	att, err := scanAttempt(q.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM exam_attempts
		WHERE learner_id = $1 AND exam_id = $2 AND released_at IS NULL ORDER BY id DESC LIMIT 1`, learnerID, examID))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("load unreleased attempt: %w", err)
	case att.Finished || att.TimedOut:
		s.Pending = &att
	default:
		s.Open = &att
	}
	return nil
}

// bestExamScore returns the Learner's highest finished score on any of a Course's Exams, and false when there is none.
func (a *App) bestExamScore(ctx context.Context, learnerID, courseID int64) (int, bool, error) {
	var best sql.NullInt64
	err := a.db.QueryRowContext(ctx, `SELECT max(t.score) FROM exam_attempts t JOIN exams x ON x.id = t.exam_id
		WHERE t.learner_id = $1 AND x.course_id = $2`, learnerID, courseID).Scan(&best)
	return int(best.Int64), best.Valid, err
}

// learnerExam is the Learner's page for an Exam.
func (a *App) learnerExam(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.publishedExamFor(w, r, learner)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, c, e)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := page{Course: &c, Learner: learner, Status: &st}
	ic, err := a.learnerIC(r.Context(), learner.ID) // starting sends a Learner without one to the IC step first
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p.NeedsIC = ic == ""
	if st.Passed && st.Enrolled { // the Exam is behind them: their next step on the Course comes first
		step, err := a.nextStep(r.Context(), learner, c, nil)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Step = &step
	}
	a.render(w, r, http.StatusOK, "learner_exam", p)
}

// startAttempt begins an attempt, or returns to the one still open.
func (a *App) startAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.publishedExamFor(w, r, learner)
	if !ok {
		return
	}
	st, err := a.examStatusFor(r.Context(), learner, c, e)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ic, err := a.learnerIC(r.Context(), learner.ID) // read before the lock below, for the same reason as there
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
	// Every attempt starts here, so this is the one gate: a Learner who could start but has no IC sets it first, so
	// a pass can always be issued its Certificate. An attempt already open (above) is let through.
	if ic == "" {
		http.Redirect(w, r, e.Path("/ic"), http.StatusSeeOther)
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

// insertAttempt stores a new attempt at the Exam. The Learner's first attempt draws the draw count of the published
// questions at random; every later one asks the first attempt's questions in the same order, as the pool has them
// now, or from the first attempt's own copy when a question has left the pool.
func insertAttempt(ctx context.Context, tx *sql.Tx, learnerID int64, e exam) (int64, error) {
	questions, err := firstDraw(ctx, tx, learnerID, e.ID)
	if err != nil {
		return 0, err
	}
	for i, asked := range questions {
		for _, q := range e.Questions {
			if q.ID == asked.ID {
				questions[i].Prompt, questions[i].Options, questions[i].ModelAnswer = q.Prompt, q.Options, q.ModelAnswer
			}
		}
	}
	if questions == nil {
		questions = []attemptQuestion{}
		for _, q := range e.Questions {
			if !q.Draft {
				questions = append(questions, attemptQuestion{ID: q.ID, Prompt: q.Prompt, Options: q.Options, Chosen: -1, ModelAnswer: q.ModelAnswer})
			}
		}
		rand.Shuffle(len(questions), func(i, j int) { questions[i], questions[j] = questions[j], questions[i] })
		questions = questions[:e.drawSize()]
	}
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

// drawSize is how many questions a first attempt at the Exam draws: its draw count of the published questions, or
// all of them when there are fewer.
func (e exam) drawSize() int {
	published := 0
	for _, q := range e.Questions {
		if !q.Draft {
			published++
		}
	}
	n, _ := strconv.Atoi(e.DrawCount)
	return min(n, published)
}

// firstDraw returns the questions of the Learner's first attempt at the Exam, unanswered and unmarked, or nil when
// there is none.
func firstDraw(ctx context.Context, tx *sql.Tx, learnerID, examID int64) ([]attemptQuestion, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT questions FROM exam_attempts WHERE learner_id = $1 AND exam_id = $2
		ORDER BY id LIMIT 1`, learnerID, examID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load first draw: %w", err)
	}
	var questions []attemptQuestion
	if err := json.Unmarshal(raw, &questions); err != nil {
		return nil, err
	}
	for i := range questions {
		questions[i].Chosen, questions[i].Answer, questions[i].Mark, questions[i].Reason = -1, "", nil, ""
	}
	return questions, nil
}

// showAttempt shows an attempt to its Learner: the questions while it is open, when its Result is ready once
// submitted, and the Result once released.
func (a *App) showAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.publishedExamFor(w, r, learner)
	if !ok {
		return
	}
	att, ok := a.attemptFor(w, r, learner.ID, e.ID)
	if !ok {
		return
	}
	p := page{Course: &c, Exam: &e, Learner: learner, Attempt: &att}
	if att.Released {
		// A released Result leads on: a pass to the Learner's next step on the Course, a fail to its retake.
		enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if enrolled {
			step, err := a.nextStep(r.Context(), learner, c, nil)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			if att.Passed || (step.Key == "next.exam-retake" && step.URL == e.Path("")) {
				p.Step = &step
			}
		}
	}
	a.render(w, r, http.StatusOK, "exam_attempt", p)
}

// submitAttempt saves an open attempt's answers and submits it; the release job marks it. A submission after the
// time limit and grace saves nothing: the attempt counts as submitted at its limit with no answers.
func (a *App) submitAttempt(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.publishedExamFor(w, r, learner)
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
	for i := range att.Questions {
		q := &att.Questions[i]
		q.Chosen = -1
		if e.Written() {
			q.Answer = strings.TrimSpace(r.PostFormValue("answer-" + strconv.FormatInt(q.ID, 10)))
			if runes := []rune(q.Answer); len(runes) > maxWrittenAnswer {
				q.Answer = string(runes[:maxWrittenAnswer])
			}
			continue
		}
		if slot, err := strconv.Atoi(r.PostFormValue("answer-" + strconv.FormatInt(q.ID, 10))); err == nil &&
			slot >= 0 && slot < len(q.Options) && q.Options[slot].Text != "" {
			q.Chosen = slot
		}
	}
	// Blank answers are asked about first, the answers given shown again. With under a minute left before the grace
	// runs out (the timer submits at zero), the attempt is submitted as it is: the extra round trip could lose it.
	if !confirmed(r) && (att.TimeLimit == 0 || att.SecondsLeft+examGraceSeconds >= 60) {
		if att.Blank = att.blanks(); att.Blank > 0 {
			a.render(w, r, http.StatusOK, "exam_attempt", page{Course: &c, Exam: &e, Learner: learner, Attempt: &att})
			return
		}
	}
	raw, err := json.Marshal(att.Questions)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The time check is in the same UPDATE, so a late submission saves nothing.
	_, err = a.db.ExecContext(r.Context(), `
		UPDATE exam_attempts SET questions = $2::jsonb, finished_at = now()
		WHERE id = $1 AND finished_at IS NULL AND (time_limit_minutes IS NULL OR NOT (`+examLateSQL+`))`,
		att.ID, string(raw))
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
	Format      string
	CourseTitle string
	PassMark    string
	Attempted   bool // a finished attempt exists, so Best is meaningful
	Best        int
	Passed      bool
	Awaiting    bool // an attempt is submitted and its Result not yet released
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
		SELECT c.id, x.format, c.title, x.pass_mark, max(t.score) IS NOT NULL, COALESCE(max(t.score), 0), COALESCE(bool_or(t.passed), false),
			EXISTS (SELECT 1 FROM exam_attempts WHERE exam_id = x.id AND learner_id = e.learner_id
				AND released_at IS NULL AND `+submittedAtSQL+` IS NOT NULL)
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		JOIN exams x ON x.course_id = c.id
		LEFT JOIN exam_attempts t ON t.exam_id = x.id AND t.learner_id = e.learner_id
		WHERE e.learner_id = $1
		  AND EXISTS (SELECT 1 FROM exam_questions q WHERE q.exam_id = x.id AND NOT q.is_draft)
		GROUP BY c.id, c.title, x.id, e.learner_id
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
		if err := rows.Scan(&x.CourseID, &x.Format, &e.CourseTitle, &e.PassMark, &e.Attempted, &e.Best, &e.Passed, &e.Awaiting); err != nil {
			a.serverError(w, r, err)
			return
		}
		e.CourseID, e.Path, e.Format = x.CourseID, x.Path(""), x.Format
		p.Exams = append(p.Exams, e)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_exams", &p)
}
