package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// bankFormats are the formats the Question bank can be browsed by.
var bankFormats = []string{multipleChoice, "written"}

// bankQuestion is one Question bank question. Exams hold copies of bank questions, never the bank question itself.
type bankQuestion struct {
	examQuestion
	Format, Topic, Language string
}

type bankPage struct {
	pageBase
	Questions     []bankQuestion
	Topics        []string // every topic in the bank, one spelling per topic ignoring case
	Formats       []string
	Topic, Format string        // the filters, empty for all
	Question      *bankQuestion // the edit form's
	Languages     []string
}

// adminQuestionBank lists the bank's questions, filtered by ?topic= (ignoring case) and ?format= when given.
func (a *App) adminQuestionBank(w http.ResponseWriter, r *http.Request) {
	p := &bankPage{Topic: r.URL.Query().Get("topic"), Format: r.URL.Query().Get("format"), Formats: bankFormats}
	var err error
	if p.Topics, err = a.bankTopics(r.Context()); err == nil {
		p.Questions, err = a.queryBankQuestions(r.Context(), `($1 = '' OR lower(q.topic) = lower($1)) AND ($2 = '' OR q.format = $2)`, p.Topic, p.Format)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_question_bank", p)
}

// bankTopics lists the bank's topics alphabetically ignoring case, each in its earliest spelling.
func (a *App) bankTopics(ctx context.Context) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT topic FROM (SELECT DISTINCT ON (lower(topic)) topic FROM bank_questions ORDER BY lower(topic), id) t ORDER BY lower(topic)`)
	if err != nil {
		return nil, fmt.Errorf("load bank topics: %w", err)
	}
	defer rows.Close()
	var topics []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("load bank topics: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// queryBankQuestions reads the bank questions matching the SQL condition where, by topic then age.
func (a *App) queryBankQuestions(ctx context.Context, where string, args ...any) ([]bankQuestion, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT q.id, q.format, q.topic, q.language, q.prompt, q.is_draft, o.position, o.text, o.is_correct
		FROM bank_questions q
		LEFT JOIN bank_options o ON o.question_id = q.id
		WHERE `+where+`
		ORDER BY lower(q.topic), q.id, o.position`, args...)
	if err != nil {
		return nil, fmt.Errorf("load bank questions: %w", err)
	}
	defer rows.Close()
	var list []bankQuestion
	for rows.Next() {
		var (
			q       bankQuestion
			slot    sql.NullInt64
			text    sql.NullString
			correct sql.NullBool
		)
		if err := rows.Scan(&q.ID, &q.Format, &q.Topic, &q.Language, &q.Prompt, &q.Draft, &slot, &text, &correct); err != nil {
			return nil, fmt.Errorf("load bank questions: %w", err)
		}
		if len(list) == 0 || list[len(list)-1].ID != q.ID {
			list = append(list, q)
		}
		if slot.Valid {
			list[len(list)-1].Options[slot.Int64] = examOption{Text: text.String, Correct: correct.Bool}
		}
	}
	return list, rows.Err()
}

// bankQuestionFor loads the bank question named by {qid}. When there is none it writes the Not found page and
// returns false.
func (a *App) bankQuestionFor(w http.ResponseWriter, r *http.Request) (bankQuestion, bool) {
	id, err := strconv.ParseInt(r.PathValue("qid"), 10, 64)
	var list []bankQuestion
	if err == nil {
		if list, err = a.queryBankQuestions(r.Context(), `q.id = $1`, id); err != nil {
			a.serverError(w, r, err)
			return bankQuestion{}, false
		}
	}
	if len(list) == 0 {
		a.notFound(w, r)
		return bankQuestion{}, false
	}
	return list[0], true
}

func (a *App) editBankQuestionForm(w http.ResponseWriter, r *http.Request) {
	q, ok := a.bankQuestionFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "bank_question_form", &bankPage{Question: &q, Languages: text.Languages})
}

// updateBankQuestion replaces a bank question's topic, language, prompt and options. Exam copies are left alone.
func (a *App) updateBankQuestion(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.bankQuestionFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	eq, msg := questionFromForm(r, lang)
	eq.ID, eq.Draft = existing.ID, existing.Draft
	q := bankQuestion{examQuestion: eq, Format: existing.Format,
		Topic: strings.TrimSpace(r.PostFormValue("topic")), Language: r.PostFormValue("language")}
	switch {
	case msg != "":
	case q.Topic == "":
		msg = text.T(lang, "bank.err.topic")
	case !text.Supported(q.Language):
		msg = text.T(lang, "bank.err.language")
	}
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "bank_question_form", &bankPage{pageBase: pageBase{Error: msg}, Question: &q, Languages: text.Languages})
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		_, err = tx.ExecContext(r.Context(), `UPDATE bank_questions SET topic = $2, language = $3, prompt = $4 WHERE id = $1`, q.ID, q.Topic, q.Language, q.Prompt)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM bank_options WHERE question_id = $1`, q.ID)
	}
	if err == nil {
		err = insertOptions(r.Context(), tx, "bank_options", q.ID, q.Options)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/question-bank", http.StatusSeeOther)
}

// deleteBankQuestion removes a bank question. Exam copies are left alone.
func (a *App) deleteBankQuestion(w http.ResponseWriter, r *http.Request) {
	q, ok := a.bankQuestionFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM bank_questions WHERE id = $1`, q.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/question-bank", http.StatusSeeOther)
}

// publishBankQuestion turns a Draft bank question into a published one, which Exams can then take from the bank.
func (a *App) publishBankQuestion(w http.ResponseWriter, r *http.Request) {
	q, ok := a.bankQuestionFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE bank_questions SET is_draft = false WHERE id = $1`, q.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/question-bank", http.StatusSeeOther)
}

// addFromBank copies up to count published bank questions of the Exam's format, the topic (ignoring case) and the
// language into the Exam, at random, skipping those it already has. The copies are published.
func (a *App) addFromBank(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	topic, qlang := strings.TrimSpace(r.PostFormValue("topic")), r.PostFormValue("language")
	count, err := strconv.Atoi(r.PostFormValue("count"))
	msg := ""
	switch {
	case topic == "":
		msg = "bank.err.topic"
	case !text.Supported(qlang):
		msg = "bank.err.language"
	case err != nil || count < 1:
		msg = "bank.err.count"
	}
	var added int64
	if msg == "" {
		if added, err = a.copyFromBank(r.Context(), e, topic, qlang, count); err != nil {
			a.serverError(w, r, err)
			return
		}
		if added == 0 {
			msg = "bank.err.none"
		}
	}
	if msg != "" {
		if e.Topics, err = a.bankTopics(r.Context()); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusUnprocessableEntity, "exam", page{Course: &c, Exam: &e, Error: text.T(lang, msg)})
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// copyFromBank does addFromBank's copying in one transaction and returns how many questions it added.
func (a *App) copyFromBank(ctx context.Context, e exam, topic, lang string, count int) (int64, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	examID, err := saveExamRow(ctx, tx, e)
	if err != nil {
		return 0, err
	}
	var added int64
	err = tx.QueryRowContext(ctx, `
		WITH picked AS (
			SELECT id, prompt, row_number() OVER () AS n FROM (
				SELECT b.id, b.prompt FROM bank_questions b
				WHERE b.format = $2 AND lower(b.topic) = lower($3) AND b.language = $4 AND NOT b.is_draft
					AND b.id NOT IN (SELECT bank_question_id FROM exam_questions WHERE exam_id = $1 AND bank_question_id IS NOT NULL)
				ORDER BY random() LIMIT $5) r),
		copied AS (
			INSERT INTO exam_questions (exam_id, position, prompt, is_draft, bank_question_id)
			SELECT $1, (SELECT COALESCE(MAX(position), 0) FROM exam_questions WHERE exam_id = $1) + n, prompt, false, id FROM picked
			RETURNING id, bank_question_id),
		options AS (
			INSERT INTO exam_options (question_id, position, text, is_correct)
			SELECT c.id, o.position, o.text, o.is_correct FROM copied c JOIN bank_options o ON o.question_id = c.bank_question_id)
		SELECT count(*) FROM copied`, examID, e.Format, topic, lang, count).Scan(&added)
	if err != nil {
		return 0, err
	}
	return added, tx.Commit()
}
