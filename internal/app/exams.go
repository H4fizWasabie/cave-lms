package app

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// multipleChoice is the format of an Exam whose questions each have options and one correct answer.
const multipleChoice = "multiple-choice"

// exam is the Exam page's data. The numeric settings stay strings so a rejected form shows what was typed.
// A Course has at most one Exam per format; ID is 0 until the Exam is first saved.
type exam struct {
	ID          int64
	CourseID    int64
	Format      string
	PassMark    string
	TimeLimit   string
	MaxAttempts string
	Usable      bool
	Questions   []examQuestion
	Topics      []string // the Question bank's topics, on the Admin page
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

// examFor loads the Exam a request names: the Course named by {id} and the format named by ?format=, Multiple-choice
// when there is none. When there is no such Course or format it writes the Not found page and returns false.
func (a *App) examFor(w http.ResponseWriter, r *http.Request) (course, exam, bool) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return course{}, exam{}, false
	}
	format := cmp.Or(r.URL.Query().Get("format"), multipleChoice)
	if format != multipleChoice {
		a.notFound(w, r)
		return course{}, exam{}, false
	}
	e, err := a.loadExam(r.Context(), c.ID, format)
	if err != nil {
		a.serverError(w, r, err)
		return course{}, exam{}, false
	}
	return c, e, true
}

// AdminPath is the address of the Exam's Admin page followed by rest, naming the format unless it is Multiple-choice.
func (e exam) AdminPath(rest string) string {
	return "/admin" + e.Path(rest)
}

// Path is the address of the Exam's Learner page followed by rest, naming the format unless it is Multiple-choice.
func (e exam) Path(rest string) string {
	p := "/courses/" + strconv.FormatInt(e.CourseID, 10) + "/exam" + rest
	if e.Format != multipleChoice {
		p += "?format=" + e.Format
	}
	return p
}

func (a *App) adminExam(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	var err error
	if e.Topics, err = a.bankTopics(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "exam", page{Course: &c, Exam: &e})
}

// saveExam sets the pass mark, time limit and attempt limit. The first save creates the Exam.
func (a *App) saveExam(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	e.PassMark = strings.TrimSpace(r.PostFormValue("pass_mark"))
	e.TimeLimit = strings.TrimSpace(r.PostFormValue("time_limit"))
	e.MaxAttempts = strings.TrimSpace(r.PostFormValue("max_attempts"))
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if msg := e.settingsProblem(lang); msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam", page{Course: &c, Exam: &e, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `
		INSERT INTO exams (course_id, format, pass_mark, time_limit_minutes, max_attempts)
		VALUES ($1, $2, $3, NULLIF($4, '')::integer, NULLIF($5, '')::integer)
		ON CONFLICT (course_id, format) DO UPDATE SET
			pass_mark = EXCLUDED.pass_mark,
			time_limit_minutes = EXCLUDED.time_limit_minutes,
			max_attempts = EXCLUDED.max_attempts`,
		c.ID, e.Format, e.PassMark, e.TimeLimit, e.MaxAttempts)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// settingsProblem returns the first problem with the settings form, in lang, empty when it is valid.
func (e exam) settingsProblem(lang string) string {
	if n, err := strconv.Atoi(e.PassMark); err != nil || n < 0 || n > 100 {
		return text.T(lang, "adm.exam.err.pass-mark")
	}
	if e.TimeLimit != "" {
		if n, err := strconv.Atoi(e.TimeLimit); err != nil || n < 1 {
			return text.T(lang, "adm.exam.err.time-limit")
		}
	}
	if e.MaxAttempts != "" {
		if n, err := strconv.Atoi(e.MaxAttempts); err != nil || n < 1 {
			return text.T(lang, "adm.exam.err.attempts")
		}
	}
	return ""
}

func (a *App) newExamQuestionForm(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_question_form", page{Course: &c, Exam: &e, Question: &examQuestion{}})
}

func (a *App) createExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	q, msg := questionFromForm(r, lang)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam_question_form", page{Course: &c, Exam: &e, Question: &q, Error: msg})
		return
	}
	if err := a.insertQuestion(r.Context(), e, q, c.Title); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// insertQuestion adds a question at the end of the Exam, first saving the Exam with default settings if it is new.
// A copy goes into the Question bank under topic, in Malay.
func (a *App) insertQuestion(ctx context.Context, e exam, q examQuestion, topic string) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	examID, err := saveExamRow(ctx, tx, e)
	if err != nil {
		return err
	}
	var bankID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO bank_questions (format, topic, language, prompt, is_draft)
		VALUES ($1, $2, 'ms', $3, $4) RETURNING id`, e.Format, topic, q.Prompt, q.Draft).Scan(&bankID); err != nil {
		return err
	}
	if err := insertOptions(ctx, tx, "bank_options", bankID, q.Options); err != nil {
		return err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exam_questions (exam_id, position, prompt, is_draft, bank_question_id)
		VALUES ($1, (SELECT COALESCE(MAX(position), 0) + 1 FROM exam_questions WHERE exam_id = $1), $2, $3, $4)
		RETURNING id`, examID, q.Prompt, q.Draft, bankID).Scan(&id)
	if err != nil {
		return err
	}
	if err := insertOptions(ctx, tx, "exam_options", id, q.Options); err != nil {
		return err
	}
	return tx.Commit()
}

// saveExamRow saves the Exam with default settings if it is new and returns its id.
func saveExamRow(ctx context.Context, tx *sql.Tx, e exam) (int64, error) {
	// The no-op update makes RETURNING give the id of an Exam that is already saved.
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO exams (course_id, format) VALUES ($1, $2)
		ON CONFLICT (course_id, format) DO UPDATE SET format = EXCLUDED.format RETURNING id`, e.CourseID, e.Format).Scan(&id)
	return id, err
}

// questionFromForm reads the question form. msg is the first problem found, in lang, empty when the form is valid.
func questionFromForm(r *http.Request, lang string) (q examQuestion, msg string) {
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
		msg = text.T(lang, "adm.exam.err.prompt")
	case filled < 2 || len(raw) > len(q.Options):
		msg = text.T(lang, "adm.exam.err.options")
	case err != nil || correct < 0 || correct >= len(q.Options) || q.Options[correct].Text == "":
		msg = text.T(lang, "adm.exam.err.correct")
	default:
		q.Options[correct].Correct = true
	}
	return q, msg
}

// loadExam reads a Course's Exam of the format with its questions. An Exam not saved yet gets the defaults.
func (a *App) loadExam(ctx context.Context, courseID int64, format string) (exam, error) {
	e := exam{CourseID: courseID, Format: format, PassMark: "50", MaxAttempts: "2"}
	err := a.db.QueryRowContext(ctx, `
		SELECT id, pass_mark, COALESCE(time_limit_minutes::text, ''), COALESCE(max_attempts::text, '')
		FROM exams WHERE course_id = $1 AND format = $2`, courseID, format).
		Scan(&e.ID, &e.PassMark, &e.TimeLimit, &e.MaxAttempts)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return exam{}, fmt.Errorf("load exam: %w", err)
	}
	e.Questions, err = a.queryQuestions(ctx, `q.exam_id = $1`, e.ID)
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

// queryQuestions reads the Exam questions matching the SQL condition where, in order.
func (a *App) queryQuestions(ctx context.Context, where string, args ...any) ([]examQuestion, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT q.id, q.prompt, q.is_draft, o.position, o.text, o.is_correct
		FROM exam_questions q
		LEFT JOIN exam_options o ON o.question_id = q.id
		WHERE `+where+`
		ORDER BY q.position, q.id, o.position`, args...)
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

func (a *App) editExamQuestionForm(w http.ResponseWriter, r *http.Request) {
	c, e, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "exam_question_form", page{Course: &c, Exam: &e, Question: &q})
}

// updateExamQuestion replaces the prompt and options. Draft state is changed only by publishing.
func (a *App) updateExamQuestion(w http.ResponseWriter, r *http.Request) {
	c, e, existing, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	q, msg := questionFromForm(r, lang)
	q.ID, q.Draft = existing.ID, existing.Draft
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "exam_question_form", page{Course: &c, Exam: &e, Question: &q, Error: msg})
		return
	}
	if err := a.replaceQuestion(r.Context(), e.ID, q); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// replaceQuestion rewrites a question's prompt and options in one transaction.
func (a *App) replaceQuestion(ctx context.Context, examID int64, q examQuestion) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE exam_questions SET prompt = $3 WHERE id = $1 AND exam_id = $2`, q.ID, examID, q.Prompt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM exam_options WHERE question_id = $1`, q.ID); err != nil {
		return err
	}
	if err := insertOptions(ctx, tx, "exam_options", q.ID, q.Options); err != nil {
		return err
	}
	return tx.Commit()
}

// insertOptions writes the filled option slots of a question into table, exam_options or bank_options.
func insertOptions(ctx context.Context, tx *sql.Tx, table string, questionID int64, options [5]examOption) error {
	for slot, o := range options {
		if o.Text == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO `+table+` (question_id, position, text, is_correct) VALUES ($1, $2, $3, $4)`,
			questionID, slot, o.Text, o.Correct); err != nil {
			return err
		}
	}
	return nil
}

// moveExamQuestion returns the handler that swaps a question with its neighbour above or below it.
func (a *App) moveExamQuestion(down bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, e, q, ok := a.examQuestionFor(w, r)
		if !ok {
			return
		}
		if err := a.swapQuestion(r.Context(), e.ID, q.ID, down); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
	}
}

// swapQuestion exchanges the positions of a question and its neighbour. A question already at the end does nothing.
func (a *App) swapQuestion(ctx context.Context, examID, questionID int64, down bool) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM exam_questions WHERE id = $1 AND exam_id = $2 FOR UPDATE`, questionID, examID).Scan(&pos); err != nil {
		return err
	}
	op, order := "<", "DESC"
	if down {
		op, order = ">", "ASC"
	}
	var otherID int64
	var otherPos int
	err = tx.QueryRowContext(ctx,
		`SELECT id, position FROM exam_questions WHERE exam_id = $1 AND position `+op+` $2 ORDER BY position `+order+` LIMIT 1`,
		examID, pos).Scan(&otherID, &otherPos)
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
	c, e, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM exam_questions WHERE id = $1 AND exam_id = $2`, q.ID, e.ID)
	if err == nil {
		err = a.issueEarnedCertificates(r.Context(), c.ID) // the Exam may have no published questions left
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// publishExamQuestion turns a Draft question into a published one. Learners see it from then on.
func (a *App) publishExamQuestion(w http.ResponseWriter, r *http.Request) {
	_, e, q, ok := a.examQuestionFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE exam_questions SET is_draft = false WHERE id = $1 AND exam_id = $2`, q.ID, e.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// examQuestionFor loads the question named by {qid} on the Exam the request names. When there is none it writes
// the Not found page and returns false.
func (a *App) examQuestionFor(w http.ResponseWriter, r *http.Request) (course, exam, examQuestion, bool) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return course{}, exam{}, examQuestion{}, false
	}
	notFound := func() (course, exam, examQuestion, bool) {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return course{}, exam{}, examQuestion{}, false
		}
		a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "adm.exam.q-not-found.title"), Message: text.T(lang, "adm.exam.q-not-found")})
		return course{}, exam{}, examQuestion{}, false
	}
	qid, err := strconv.ParseInt(r.PathValue("qid"), 10, 64)
	if err != nil {
		return notFound()
	}
	for _, q := range e.Questions {
		if q.ID == qid {
			return c, e, q, true
		}
	}
	return notFound()
}
