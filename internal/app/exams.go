package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// exam is the Exam page's data. The numeric settings stay strings so a rejected form shows what was typed.
type exam struct {
	CourseID    int64
	PassMark    string
	TimeLimit   string
	MaxAttempts string
	Usable      bool
	Questions   []examQuestion
}

// examQuestion is one Exam question. Options always has five slots; a blank slot is an unused option.
type examQuestion struct {
	ID      int64
	Prompt  string
	Draft   bool
	Options [5]examOption
}

type examOption struct {
	Text    string
	Correct bool
}

func (a *App) adminExam(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	e, err := a.loadExam(r.Context(), c.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, r, http.StatusOK, "exam", page{Course: &c, Exam: &e})
}

// saveExam sets the pass mark, time limit and attempt limit. The first save creates the Exam.
func (a *App) saveExam(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	e := exam{
		CourseID:    c.ID,
		PassMark:    strings.TrimSpace(r.PostFormValue("pass_mark")),
		TimeLimit:   strings.TrimSpace(r.PostFormValue("time_limit")),
		MaxAttempts: strings.TrimSpace(r.PostFormValue("max_attempts")),
	}
	if msg := e.settingsProblem(); msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam", page{Course: &c, Exam: &e, Error: msg})
		return
	}
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO exams (course_id, pass_mark, time_limit_minutes, max_attempts)
		VALUES ($1, $2, NULLIF($3, '')::integer, NULLIF($4, '')::integer)
		ON CONFLICT (course_id) DO UPDATE SET
			pass_mark = EXCLUDED.pass_mark,
			time_limit_minutes = EXCLUDED.time_limit_minutes,
			max_attempts = EXCLUDED.max_attempts`,
		c.ID, e.PassMark, e.TimeLimit, e.MaxAttempts)
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
}

// settingsProblem returns the first problem with the settings form, empty when it is valid.
func (e exam) settingsProblem() string {
	if n, err := strconv.Atoi(e.PassMark); err != nil || n < 0 || n > 100 {
		return "Enter the pass mark as a whole number from 0 to 100."
	}
	if e.TimeLimit != "" {
		if n, err := strconv.Atoi(e.TimeLimit); err != nil || n < 1 {
			return "Enter the time limit in minutes, or leave it empty."
		}
	}
	if e.MaxAttempts != "" {
		if n, err := strconv.Atoi(e.MaxAttempts); err != nil || n < 1 {
			return "Enter the maximum attempts as a whole number, or leave it empty."
		}
	}
	return ""
}

func (a *App) newExamQuestionForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_question_form", page{Course: &c, Question: &examQuestion{}})
}

func (a *App) createExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	q, msg := questionFromForm(r)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam_question_form", page{Course: &c, Question: &q, Error: msg})
		return
	}
	if err := a.insertQuestion(r.Context(), c.ID, q); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
}

// insertQuestion adds a question at the end of the Exam, creating the Exam with default settings if it has none.
func (a *App) insertQuestion(ctx context.Context, courseID int64, q examQuestion) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO exams (course_id) VALUES ($1) ON CONFLICT (course_id) DO NOTHING`, courseID); err != nil {
		return err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exam_questions (course_id, position, prompt, is_draft)
		VALUES ($1, (SELECT COALESCE(MAX(position), 0) + 1 FROM exam_questions WHERE course_id = $1), $2, $3)
		RETURNING id`, courseID, q.Prompt, q.Draft).Scan(&id)
	if err != nil {
		return err
	}
	if err := insertOptions(ctx, tx, id, q.Options); err != nil {
		return err
	}
	return tx.Commit()
}

// questionFromForm reads the question form. msg is the first problem found, empty when the form is valid.
func questionFromForm(r *http.Request) (q examQuestion, msg string) {
	q.Prompt = strings.TrimSpace(r.PostFormValue("prompt"))
	q.Draft = r.PostFormValue("draft") == "on"
	raw := r.PostForm["option"]
	filled := 0
	for i := 0; i < len(raw) && i < len(q.Options); i++ {
		q.Options[i].Text = strings.TrimSpace(raw[i])
		if q.Options[i].Text != "" {
			filled++
		}
	}
	correct, err := strconv.Atoi(r.PostFormValue("correct"))
	switch {
	case q.Prompt == "":
		msg = "Enter the question."
	case filled < 2 || len(raw) > len(q.Options):
		msg = "Enter between 2 and 5 options."
	case err != nil || correct < 0 || correct >= len(q.Options) || q.Options[correct].Text == "":
		msg = "Mark the correct option, and make sure it has text."
	default:
		q.Options[correct].Correct = true
	}
	return q, msg
}

// loadExam reads the Exam for a Course with its questions. A Course with no Exam yet gets the defaults.
func (a *App) loadExam(ctx context.Context, courseID int64) (exam, error) {
	e := exam{CourseID: courseID, PassMark: "50"}
	err := a.db.QueryRowContext(ctx, `
		SELECT pass_mark, COALESCE(time_limit_minutes::text, ''), COALESCE(max_attempts::text, '')
		FROM exams WHERE course_id = $1`, courseID).
		Scan(&e.PassMark, &e.TimeLimit, &e.MaxAttempts)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return exam{}, fmt.Errorf("load exam: %w", err)
	}
	e.Questions, err = a.loadQuestions(ctx, courseID, 0)
	if err != nil {
		return exam{}, err
	}
	for _, q := range e.Questions {
		if !q.Draft {
			e.Usable = true
		}
	}
	return e, nil
}

// loadQuestions reads the Exam questions of a Course in order, or only question onlyID when it is not 0.
func (a *App) loadQuestions(ctx context.Context, courseID, onlyID int64) ([]examQuestion, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT q.id, q.prompt, q.is_draft, o.position, o.text, o.is_correct
		FROM exam_questions q
		LEFT JOIN exam_options o ON o.question_id = q.id
		WHERE q.course_id = $1 AND ($2::bigint = 0 OR q.id = $2)
		ORDER BY q.position, q.id, o.position`, courseID, onlyID)
	if err != nil {
		return nil, fmt.Errorf("load questions: %w", err)
	}
	defer rows.Close()
	var list []examQuestion
	for rows.Next() {
		var (
			id      int64
			prompt  string
			draft   bool
			slot    sql.NullInt64
			text    sql.NullString
			correct sql.NullBool
		)
		if err := rows.Scan(&id, &prompt, &draft, &slot, &text, &correct); err != nil {
			return nil, fmt.Errorf("load questions: %w", err)
		}
		if len(list) == 0 || list[len(list)-1].ID != id {
			list = append(list, examQuestion{ID: id, Prompt: prompt, Draft: draft})
		}
		if slot.Valid {
			list[len(list)-1].Options[slot.Int64] = examOption{Text: text.String, Correct: correct.Bool}
		}
	}
	return list, rows.Err()
}

func examPath(courseID int64) string {
	return "/admin/courses/" + strconv.FormatInt(courseID, 10) + "/exam"
}

func (a *App) editExamQuestionForm(w http.ResponseWriter, r *http.Request) {
	c, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_question_form", page{Course: &c, Question: &q})
}

// updateExamQuestion replaces the prompt and options. Draft state is changed only by publishing.
func (a *App) updateExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, existing, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	q, msg := questionFromForm(r)
	q.ID, q.Draft = existing.ID, existing.Draft
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam_question_form", page{Course: &c, Question: &q, Error: msg})
		return
	}
	if err := a.replaceQuestion(r.Context(), c.ID, q); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
}

// replaceQuestion rewrites a question's prompt and options in one transaction.
func (a *App) replaceQuestion(ctx context.Context, courseID int64, q examQuestion) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE exam_questions SET prompt = $3 WHERE id = $1 AND course_id = $2`, q.ID, courseID, q.Prompt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM exam_options WHERE question_id = $1`, q.ID); err != nil {
		return err
	}
	if err := insertOptions(ctx, tx, q.ID, q.Options); err != nil {
		return err
	}
	return tx.Commit()
}

// insertOptions writes the filled option slots of a question.
func insertOptions(ctx context.Context, tx *sql.Tx, questionID int64, options [5]examOption) error {
	for slot, o := range options {
		if o.Text == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO exam_options (question_id, position, text, is_correct) VALUES ($1, $2, $3, $4)`,
			questionID, slot, o.Text, o.Correct); err != nil {
			return err
		}
	}
	return nil
}

// moveExamQuestion returns the handler that swaps a question with its neighbour above or below it.
func (a *App) moveExamQuestion(down bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, q, ok := a.examQuestionFor(w, r)
		if !ok {
			return
		}
		if err := a.swapQuestion(r.Context(), c.ID, q.ID, down); err != nil {
			serverError(w, err)
			return
		}
		http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
	}
}

// swapQuestion exchanges the positions of a question and its neighbour. A question already at the end does nothing.
func (a *App) swapQuestion(ctx context.Context, courseID, questionID int64, down bool) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM exam_questions WHERE id = $1 AND course_id = $2 FOR UPDATE`, questionID, courseID).Scan(&pos); err != nil {
		return err
	}
	op, order := "<", "DESC"
	if down {
		op, order = ">", "ASC"
	}
	var otherID int64
	var otherPos int
	err = tx.QueryRowContext(ctx,
		`SELECT id, position FROM exam_questions WHERE course_id = $1 AND position `+op+` $2 ORDER BY position `+order+` LIMIT 1`,
		courseID, pos).Scan(&otherID, &otherPos)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE exam_questions SET position = CASE WHEN id = $1 THEN $3::integer ELSE $4::integer END WHERE id IN ($1, $2)`,
		questionID, otherID, otherPos, pos); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteExamQuestion removes a question, Draft or published, with its options.
func (a *App) deleteExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM exam_questions WHERE id = $1 AND course_id = $2`, q.ID, c.ID)
	if err == nil {
		err = a.issueEarnedCertificates(r.Context(), c.ID) // the Exam may have no published questions left
	}
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
}

// publishExamQuestion turns a Draft question into a published one. Learners see it from then on.
func (a *App) publishExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE exam_questions SET is_draft = false WHERE id = $1 AND course_id = $2`, q.ID, c.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, examPath(c.ID), http.StatusSeeOther)
}

// examQuestionFor loads the question named by {qid} on the Course named by {id}. When there is none it writes
// the Not found page and returns false.
func (a *App) examQuestionFor(w http.ResponseWriter, r *http.Request) (course, examQuestion, bool) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return course{}, examQuestion{}, false
	}
	notFound := func() (course, examQuestion, bool) {
		a.render(w, r, http.StatusNotFound, "message", page{Title: "Question not found", Message: "There is no Exam question at this address."})
		return course{}, examQuestion{}, false
	}
	qid, err := strconv.ParseInt(r.PathValue("qid"), 10, 64)
	if err != nil || qid == 0 { // 0 would mean "all questions" to loadQuestions
		return notFound()
	}
	list, err := a.loadQuestions(r.Context(), c.ID, qid)
	if err != nil {
		serverError(w, err)
		return course{}, examQuestion{}, false
	}
	if len(list) == 0 {
		return notFound()
	}
	return c, list[0], true
}
