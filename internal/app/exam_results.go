package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// resultDelayMinutes is how long after an attempt is submitted its Result is released, at most; the Exam page says so.
// failedChoiceDelayMinutes is the delay of a failed Multiple-choice attempt, so the Learner can retake sooner. A Written
// attempt keeps the full delay, win or lose: the Admin may override its marks in that time (ADR 0009).
const (
	resultDelayMinutes       = 10
	failedChoiceDelayMinutes = 1
)

// resultLatestSQL is the longest delay, the one the Learner is told: what they see before release must not show a
// pass or a fail.
var resultLatestSQL = fmt.Sprintf("interval '%d minutes'", resultDelayMinutes)

// answersPassSQL holds for an attempt whose answers reach its pass mark, as markQuestions works it out. Nothing is
// saved as passed until release, so the delay reads the answers.
var answersPassSQL = `(SELECT count(*) FILTER (WHERE (q->>'Chosen')::int >= 0 AND (q->'Options'->((q->>'Chosen')::int)->>'Correct')::bool) * 100
	/ greatest(jsonb_array_length(questions), 1) FROM jsonb_array_elements(questions) q) >= pass_mark`

// resultDelaySQL is how long after it is submitted an attempt's Result is released. Every release and every check of
// one goes through it.
var resultDelaySQL = fmt.Sprintf(`CASE WHEN (SELECT format FROM exams WHERE exams.id = exam_id) = 'multiple-choice' AND NOT `+answersPassSQL+`
	THEN interval '%d minutes' ELSE %s END`, failedChoiceDelayMinutes, resultLatestSQL)

// submittedAtSQL is when an attempt was submitted, NULL while it is open. An attempt left open past its time limit
// and grace counts as submitted at its limit, with the answers sent so far, which is none.
var submittedAtSQL = `COALESCE(finished_at, CASE WHEN time_limit_minutes IS NOT NULL AND ` + examLateSQL + `
	THEN started_at + make_interval(mins => time_limit_minutes) END)`

// malaysia is the time zone release times are shown in. Malaysia has no daylight saving time.
var malaysia = time.FixedZone("MYT", 8*60*60)

// ReadyAt is the latest time the attempt's Result is released, in lang, as "2 January 2099, 11:14" Malaysia time.
func (att examAttempt) ReadyAt(lang string) string {
	return dateTimeIn(lang, att.ReleaseAt)
}

// markQuestions is the percentage of questions answered right.
func markQuestions(questions []attemptQuestion) int {
	if len(questions) == 0 {
		return 0
	}
	right := 0
	for _, q := range questions {
		if q.Right() {
			right++
		}
	}
	return right * 100 / len(questions)
}

// markAttempt is an attempt's mark as a percentage, and false while a Written answer still waits for its mark. A
// Written question is worth writtenMarks; a blank Written answer needs no marking and scores 0.
func markAttempt(format string, questions []attemptQuestion) (int, bool) {
	if format != written {
		return markQuestions(questions), true
	}
	if len(questions) == 0 {
		return 0, true
	}
	got := 0
	for _, q := range questions {
		switch {
		case q.Mark != nil:
			got += *q.Mark
		case q.Answer != "":
			return 0, false
		}
	}
	return got * 100 / (writtenMarks * len(questions)), true
}

// dueResult is a submitted attempt whose Result is due, with what releasing it needs.
type dueResult struct {
	attemptID, learnerID          int64
	exam                          exam
	passMark                      int
	questions                     []attemptQuestion
	name, email, language, course string
	released, passed              bool
}

// unmarkedSQL holds for a submitted attempt not yet released with a Written answer still waiting for its mark.
var unmarkedSQL = `released_at IS NULL AND ` + submittedAtSQL + ` IS NOT NULL
	AND EXISTS (SELECT 1 FROM jsonb_array_elements(questions) q WHERE q->>'Answer' <> '' AND q->'Mark' IS NULL)`

// markRetrySQL is how long after the AI was asked to mark an attempt it is asked again.
const markRetrySQL = "interval '5 minutes'"

// maxBadShapes is how many runs the AI's mark for an attempt may come back in the wrong shape before the AI is asked
// no more and the attempt's unmarked answers wait for the Admin. An outage does not count.
const maxBadShapes = 3

// logLateMarking writes a System log line, once, for each attempt still unmarked an hour after its release time.
func (a *App) logLateMarking(ctx context.Context) error {
	rows, err := a.db.QueryContext(ctx, `UPDATE exam_attempts SET marking_late_logged = true
		WHERE NOT marking_late_logged AND `+unmarkedSQL+` AND `+submittedAtSQL+` <= now() - `+resultDelaySQL+` - interval '1 hour'
		RETURNING id`)
	if err != nil {
		return err
	}
	var late []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		late = append(late, id)
	}
	rows.Close()
	for _, id := range late {
		a.logError(nil, fmt.Errorf("attempt %d still has unmarked Written answers an hour after its release time; mark them on /admin/results/%d", id, id))
	}
	return rows.Err()
}

// markWritten asks the AI writer to mark, one by one, each unmarked Written answer of every attempt it has not been
// asked about in the last 5 minutes, and saves each mark as it comes. Only the question, model answer and answer go
// out (ADR 0009). A mark that fails is left for a later run; when the AI service fails, the run stops, so a service
// that is down costs one call a run. An attempt whose marks came back in the wrong shape in maxBadShapes runs is
// left for the Admin.
func (a *App) markWritten(ctx context.Context) {
	if err := a.logLateMarking(ctx); err != nil {
		a.logError(nil, fmt.Errorf("find Written attempts unmarked an hour after release: %w", err))
	}
	due, err := a.queryDue(ctx, unmarkedSQL+` AND marking_bad_shapes < `+fmt.Sprint(maxBadShapes)+` AND (marking_tried_at IS NULL OR marking_tried_at <= now() - `+markRetrySQL+`)`)
	if err != nil {
		a.logError(nil, fmt.Errorf("find Written answers to mark: %w", err))
		return
	}
	for _, d := range due {
		if _, err := a.db.ExecContext(ctx, `UPDATE exam_attempts SET marking_tried_at = now() WHERE id = $1`, d.attemptID); err != nil {
			a.logError(nil, fmt.Errorf("note marking attempt %d: %w", d.attemptID, err))
			return
		}
		badShape := false
		for i, q := range d.questions {
			if q.Mark != nil || q.Answer == "" {
				continue
			}
			m, err := aiwriter.Mark(ctx, a.writer, aiwriter.MarkRequest{Question: q.Prompt, ModelAnswer: q.ModelAnswer, Answer: q.Answer})
			if errors.Is(err, aiwriter.ErrBadShape) {
				if !badShape {
					badShape = true
					if _, err := a.db.ExecContext(ctx, `UPDATE exam_attempts SET marking_bad_shapes = marking_bad_shapes + 1 WHERE id = $1`, d.attemptID); err != nil {
						a.logError(nil, fmt.Errorf("count a wrong-shaped mark on attempt %d: %w", d.attemptID, err))
					}
				}
				continue
			}
			if err != nil {
				return
			}
			if err := a.saveAIMark(ctx, d.attemptID, i, m); err != nil {
				a.logError(nil, fmt.Errorf("save the AI's mark on attempt %d: %w", d.attemptID, err))
			}
		}
	}
}

// saveAIMark sets the mark of the attempt's question at index i, unless it has one by now: an Admin's mark wins.
func (a *App) saveAIMark(ctx context.Context, attemptID int64, i int, m aiwriter.Marking) error {
	// Holding the release lock keeps this apart from an Admin saving marks read before it.
	a.releasing.Lock()
	defer a.releasing.Unlock()
	_, err := a.db.ExecContext(ctx, `UPDATE exam_attempts
		SET questions = jsonb_set(questions, ARRAY[$2::text], questions->$2::int || jsonb_build_object('Mark', $3::int, 'Reason', $4::text))
		WHERE id = $1 AND released_at IS NULL AND questions->$2::int->'Mark' IS NULL`, attemptID, i, m.Mark, m.Reason)
	return err
}

// releaseResults has the AI mark Written answers waiting for a mark, then releases every Result submitted longer ago than its
// delay (resultDelaySQL) and fully marked, an attempt left open past its time limit included. It reads only the database, so
// a restart loses nothing. The Certificates and Result emails of what it released go out in one background run, in
// order, so a slow mail service never holds up a release or bursts sends at once; Close waits for the run.
// ponytail: marking runs before releasing in one goroutine, so a slow AI service delays releases; mark in its own
// goroutine if that bites.
func (a *App) releaseResults(ctx context.Context) {
	a.markWritten(ctx)
	a.releasing.Lock()
	defer a.releasing.Unlock()
	due, err := a.dueResults(ctx)
	if err != nil {
		a.logError(nil, fmt.Errorf("find due Results: %w", err))
		return
	}
	var emails []func(context.Context)
	for _, d := range due {
		send, err := a.releaseResult(ctx, d)
		if err != nil {
			a.logError(nil, fmt.Errorf("release Result of attempt %d: %w", d.attemptID, err))
		}
		if send != nil {
			emails = append(emails, send)
		}
	}
	if len(emails) == 0 {
		return
	}
	a.emailing.Add(1)
	go func() {
		defer a.emailing.Done()
		ctx := context.WithoutCancel(ctx)
		for _, send := range emails {
			a.survive("Result emails", func() { send(ctx) })
		}
	}()
}

func (a *App) dueResults(ctx context.Context) ([]dueResult, error) {
	return a.queryDue(ctx, `released_at IS NULL AND `+submittedAtSQL+` <= now() - `+resultDelaySQL)
}

// queryDue reads the attempts matching the SQL condition where, on exam_attempts, with what releasing them needs.
func (a *App) queryDue(ctx context.Context, where string, args ...any) ([]dueResult, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT t.id, t.learner_id, x.course_id, x.format, t.pass_mark, t.questions, l.name, l.email,
			COALESCE(l.language, ''), c.title, t.released_at IS NOT NULL, COALESCE(t.passed, false)
		FROM (SELECT * FROM exam_attempts WHERE `+where+`) t
		JOIN exams x ON x.id = t.exam_id
		JOIN learners l ON l.id = t.learner_id
		JOIN courses c ON c.id = x.course_id
		ORDER BY t.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []dueResult
	for rows.Next() {
		var d dueResult
		var questions []byte
		if err := rows.Scan(&d.attemptID, &d.learnerID, &d.exam.CourseID, &d.exam.Format, &d.passMark, &questions,
			&d.name, &d.email, &d.language, &d.course, &d.released, &d.passed); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(questions, &d.questions); err != nil {
			return nil, err
		}
		due = append(due, d)
	}
	return due, rows.Err()
}

// releaseResult marks the attempt, closes it if it was left open, and saves its mark and pass or fail, unless it is
// already released or a Written answer is still unmarked. Then it issues any Certificate it earns. It returns what
// emails the Certificate and the Result, for the caller to run when it likes, nil when nothing was released.
func (a *App) releaseResult(ctx context.Context, d dueResult) (sendEmails func(context.Context), err error) {
	score, marked := markAttempt(d.exam.Format, d.questions)
	if !marked {
		return nil, nil
	}
	passed := score >= d.passMark
	res, err := a.db.ExecContext(ctx, `UPDATE exam_attempts SET finished_at = `+submittedAtSQL+`,
		released_at = now(), score = $2, passed = $3 WHERE id = $1 AND released_at IS NULL`, d.attemptID, score, passed)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}
	certificateID, err := a.issueCertificateIfEarnedUnsent(ctx, d.learnerID, d.exam.CourseID)
	if err != nil {
		a.logError(nil, fmt.Errorf("issue Certificate on releasing attempt %d: %w", d.attemptID, err))
	}
	return func(ctx context.Context) {
		if certificateID != "" {
			a.emailCertificate(ctx, certificateID)
		}
		a.emailResult(ctx, d, score, passed, "email.result.subject")
	}, nil
}

// emailResult emails the attempt's Result in the Learner's saved language, under the subject the key names, naming
// the Exam's format, with the Answer sheet when it is a pass. A failed email is only logged: the Result is on the site.
func (a *App) emailResult(ctx context.Context, d dueResult, score int, passed bool, subject string) {
	lang := d.language
	if !text.Supported(lang) {
		lang = text.Languages[0]
	}
	verdict := text.T(lang, "exam.failed", d.passMark)
	if passed {
		verdict = text.T(lang, "exam.passed", d.passMark)
	}
	format := text.T(lang, "bank.format."+d.exam.Format)
	msg := mailer.Message{
		To:      d.email,
		Subject: text.T(lang, subject, d.course, format),
		Body:    text.T(lang, "email.result.body", d.name, d.course, score, verdict, a.publicBaseURL+attemptPath(d.exam, d.attemptID), format),
	}
	if passed {
		if sheet, err := answerSheetPDF(lang, d.name, d.course, d.exam.Format, score, d.questions); err != nil {
			a.logError(nil, fmt.Errorf("make Answer sheet for attempt %d: %w", d.attemptID, err))
		} else {
			msg.Body += "\n" + text.T(lang, "email.result.answer-sheet") + "\n"
			msg.Attachments = []mailer.Attachment{{Filename: "answer-sheet.pdf", Content: sheet}}
		}
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		a.logError(nil, fmt.Errorf("send Result email for attempt %d: %w", d.attemptID, err))
	}
}

// releaseEveryMinute runs releaseResults once a minute until done is closed.
func (a *App) releaseEveryMinute(done <-chan struct{}) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			a.survive("Result release", func() { a.releaseResults(context.Background()) })
		}
	}
}
