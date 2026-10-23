package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// resultDelaySQL is how long after an attempt is submitted its Result is released.
const resultDelaySQL = "interval '10 minutes'"

// submittedAtSQL is when an attempt was submitted, NULL while it is open. An attempt left open past its time limit
// and grace counts as submitted at its limit, with the answers sent so far, which is none.
var submittedAtSQL = `COALESCE(finished_at, CASE WHEN time_limit_minutes IS NOT NULL AND ` + examLateSQL + `
	THEN started_at + make_interval(mins => time_limit_minutes) END)`

// malaysia is the time zone release times are shown in. Malaysia has no daylight saving time.
var malaysia = time.FixedZone("MYT", 8*60*60)

// ReadyAt is when the attempt's Result is released, in lang, as "2 January 2099, 11:14" Malaysia time.
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

// dueResult is a submitted attempt whose Result is due, with what releasing it needs.
type dueResult struct {
	attemptID, learnerID          int64
	exam                          exam
	passMark                      int
	questions                     []attemptQuestion
	name, email, language, course string
}

// releaseResults releases every Result submitted at least 10 minutes ago, an attempt left open past its time limit
// included. It reads only the database, so a restart loses nothing.
func (a *App) releaseResults(ctx context.Context) {
	a.releasing.Lock()
	defer a.releasing.Unlock()
	due, err := a.dueResults(ctx)
	if err != nil {
		a.logError(nil, fmt.Errorf("find due Results: %w", err))
		return
	}
	for _, d := range due {
		if err := a.releaseResult(ctx, d); err != nil {
			a.logError(nil, fmt.Errorf("release Result of attempt %d: %w", d.attemptID, err))
		}
	}
}

func (a *App) dueResults(ctx context.Context) ([]dueResult, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT t.id, t.learner_id, x.course_id, x.format, t.pass_mark, t.questions, l.name, l.email,
			COALESCE(l.language, ''), c.title
		FROM (SELECT * FROM exam_attempts WHERE released_at IS NULL AND `+submittedAtSQL+` <= now() - `+resultDelaySQL+`) t
		JOIN exams x ON x.id = t.exam_id
		JOIN learners l ON l.id = t.learner_id
		JOIN courses c ON c.id = x.course_id
		ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []dueResult
	for rows.Next() {
		var d dueResult
		var questions []byte
		if err := rows.Scan(&d.attemptID, &d.learnerID, &d.exam.CourseID, &d.exam.Format, &d.passMark, &questions,
			&d.name, &d.email, &d.language, &d.course); err != nil {
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
// already released. Then it issues any Certificate it earns and emails the Result in the Learner's saved language.
// A failed email is only logged: the Result is on the site.
func (a *App) releaseResult(ctx context.Context, d dueResult) error {
	score := markQuestions(d.questions)
	passed := score >= d.passMark
	res, err := a.db.ExecContext(ctx, `UPDATE exam_attempts SET finished_at = `+submittedAtSQL+`,
		released_at = now(), score = $2, passed = $3 WHERE id = $1 AND released_at IS NULL`, d.attemptID, score, passed)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	if err := a.issueCertificateIfEarned(ctx, d.learnerID, d.exam.CourseID); err != nil {
		a.logError(nil, fmt.Errorf("issue Certificate on releasing attempt %d: %w", d.attemptID, err))
	}
	lang := d.language
	if !text.Supported(lang) {
		lang = text.Languages[0]
	}
	verdict := text.T(lang, "exam.failed", d.passMark)
	if passed {
		verdict = text.T(lang, "exam.passed", d.passMark)
	}
	if err := a.mailer.Send(ctx, mailer.Message{
		To:      d.email,
		Subject: text.T(lang, "email.result.subject", d.course),
		Body:    text.T(lang, "email.result.body", d.name, d.course, score, verdict, a.publicBaseURL+attemptPath(d.exam, d.attemptID)),
	}); err != nil {
		a.logError(nil, fmt.Errorf("send Result email for attempt %d: %w", d.attemptID, err))
	}
	return nil
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
			a.releaseResults(context.Background())
		}
	}
}
