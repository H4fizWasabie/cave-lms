package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// bankFormats are the formats the Question bank can be browsed by.
var bankFormats = []string{multipleChoice, written}

// bankQuestion is one Question bank question. Exams hold copies of bank questions, never the bank question itself.
type bankQuestion struct {
	examQuestion
	Format, Topic, Language string
}

// Written reports whether the bank question is a Written one.
func (q bankQuestion) Written() bool { return q.Format == written }

type bankPage struct {
	pageBase
	Questions     []bankQuestion
	Topics        []string // every topic in the bank, one spelling per topic ignoring case
	Formats       []string
	Topic, Format string        // the filters, empty for all
	Question      *bankQuestion // the edit form's
	Languages     []string
	Query         template.URL // the filters as a query string with its "?", empty for none, kept across actions
}

// bankFilters are the query parameters the Question bank page filters by.
var bankFilters = []string{"topic", "format"}

// bankQuery is the request's Question bank filters as a query string with its "?", empty for none. Anything else in
// the request's query is dropped.
func bankQuery(r *http.Request) template.URL {
	v := url.Values{}
	for _, k := range bankFilters {
		if f := r.URL.Query().Get(k); f != "" {
			v.Set(k, f)
		}
	}
	if len(v) == 0 {
		return ""
	}
	return template.URL("?" + v.Encode())
}

// bankCardPath is the address of bank question id's card on the Question bank page under the request's filters, or
// of the question list when id is 0.
func bankCardPath(r *http.Request, id int64) string {
	if id == 0 {
		return "/admin/question-bank" + string(bankQuery(r)) + "#questions"
	}
	return "/admin/question-bank" + string(bankQuery(r)) + "#q-" + strconv.FormatInt(id, 10)
}

// filteredBankQuestions reads the bank's questions under the request's ?topic= (ignoring case) and ?format=.
func (a *App) filteredBankQuestions(r *http.Request) ([]bankQuestion, error) {
	q := r.URL.Query()
	return a.queryBankQuestions(r.Context(), `($1 = '' OR lower(q.topic) = lower($1)) AND ($2 = '' OR q.format = $2)`, q.Get("topic"), q.Get("format"))
}

// adminQuestionBank lists the bank's questions, filtered by ?topic= (ignoring case) and ?format= when given.
func (a *App) adminQuestionBank(w http.ResponseWriter, r *http.Request) {
	p := &bankPage{Topic: r.URL.Query().Get("topic"), Format: r.URL.Query().Get("format"), Formats: bankFormats, Query: bankQuery(r)}
	var err error
	if p.Topics, err = a.bankTopics(r.Context()); err == nil {
		p.Questions, err = a.filteredBankQuestions(r)
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
		SELECT q.id, q.format, q.topic, q.language, q.prompt, q.is_draft, q.model_answer, COALESCE(q.difficulty, ''), o.position, o.text, o.is_correct
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
		if err := rows.Scan(&q.ID, &q.Format, &q.Topic, &q.Language, &q.Prompt, &q.Draft, &q.ModelAnswer, &q.Difficulty, &slot, &text, &correct); err != nil {
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
	a.render(w, r, http.StatusOK, "bank_question_form", &bankPage{Question: &q, Languages: text.Languages, Query: bankQuery(r)})
}

// updateBankQuestion replaces a bank question's topic, language, prompt and options or model answer. Exam copies are
// left alone.
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
	eq, msg := questionFromForm(r, lang, existing.Format)
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
		a.render(w, r, http.StatusUnprocessableEntity, "bank_question_form", &bankPage{pageBase: pageBase{Error: msg}, Question: &q, Languages: text.Languages, Query: bankQuery(r)})
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err == nil {
		defer tx.Rollback()
		_, err = tx.ExecContext(r.Context(), `UPDATE bank_questions SET topic = $2, language = $3, prompt = $4, model_answer = $5 WHERE id = $1`,
			q.ID, q.Topic, q.Language, q.Prompt, q.ModelAnswer)
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
	http.Redirect(w, r, bankCardPath(r, q.ID), http.StatusSeeOther)
}

// deleteBankQuestion removes a bank question and returns to the card after it under the same filters. Exam copies
// are left alone.
func (a *App) deleteBankQuestion(w http.ResponseWriter, r *http.Request) {
	q, ok := a.bankQuestionFor(w, r)
	if !ok {
		return
	}
	list, err := a.filteredBankQuestions(r)
	if err == nil {
		_, err = a.db.ExecContext(r.Context(), `DELETE FROM bank_questions WHERE id = $1`, q.ID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var next int64 // the question after the deleted one, 0 for none
	for i, bq := range list {
		if bq.ID == q.ID && i+1 < len(list) {
			next = list[i+1].ID
		}
	}
	http.Redirect(w, r, bankCardPath(r, next), http.StatusSeeOther)
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
	http.Redirect(w, r, bankCardPath(r, q.ID), http.StatusSeeOther)
}

// addFromBank copies up to count published bank questions of the Exam's format, the topic (ignoring case) and the
// language into the Exam, at random, skipping those it already has. The copies are published.
func (a *App) addFromBank(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	topic, qlang, count, msg := bankForm(r)
	var added int64
	if msg == "" {
		var err error
		if added, err = a.copyFromBank(r.Context(), e, topic, qlang, "", count, nil); err != nil {
			a.serverError(w, r, err)
			return
		}
		if added == 0 {
			msg = "bank.err.none"
		}
	}
	if msg != "" {
		a.examProblem(w, r, c, e, http.StatusUnprocessableEntity, msg)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// maxGenerated is the most questions one generation asks for, so the AI's reply fits its length cap.
const maxGenerated = 50

// generateQuestions fills the Exam with count questions on the topic in the language at the Difficulty, Medium when
// none or an unknown one is posted: matching published bank questions of that Difficulty first, then the AI writes
// the rest, which are saved with the Difficulty as Drafts in the bank and the Exam. When the AI fails nothing is saved.
func (a *App) generateQuestions(w http.ResponseWriter, r *http.Request) {
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	topic, qlang, count, msg := bankForm(r)
	if msg == "" && count > maxGenerated {
		msg = "gen.err.count"
	}
	if msg != "" {
		a.examProblem(w, r, c, e, http.StatusUnprocessableEntity, msg)
		return
	}
	difficulty := r.PostFormValue("difficulty")
	if !slices.Contains(difficulties, difficulty) {
		difficulty = "medium"
	}
	var banked int
	err := a.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bank_questions b WHERE `+bankMatch, e.ID, e.Format, topic, qlang, difficulty).Scan(&banked)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var aiWritten []examQuestion
	if rest := count - banked; rest > 0 {
		qs, err := aiwriter.Questions(r.Context(), a.writer, aiwriter.Request{Topic: topic, Format: e.Format, Language: qlang, Count: rest, Difficulty: difficulty})
		if msg = aiProblem(err); msg != "" {
			a.logError(r, err)
			a.examProblem(w, r, c, e, http.StatusServiceUnavailable, msg)
			return
		}
		for _, q := range qs {
			eq := examQuestion{Prompt: strings.TrimSpace(q.Prompt), ModelAnswer: strings.TrimSpace(q.ModelAnswer), Draft: true, Difficulty: difficulty}
			for i, o := range q.Options {
				eq.Options[i] = examOption{Text: strings.TrimSpace(o.Text), Correct: o.Correct}
			}
			aiWritten = append(aiWritten, eq)
		}
	}
	if _, err := a.copyFromBank(r.Context(), e, topic, qlang, difficulty, count, aiWritten); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, e.AdminPath(""), http.StatusSeeOther)
}

// aiProblem is the message key for the AI writer's err, empty for none.
func aiProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, aiwriter.ErrNotConfigured):
		return "gen.err.not-configured"
	case errors.Is(err, aiwriter.ErrOverLimit):
		return "gen.err.over-limit"
	case errors.Is(err, aiwriter.ErrBadShape):
		return "gen.err.bad-shape"
	}
	return "gen.err.unavailable"
}

// bankForm reads the topic, language and count the Exam page's bank and generate forms post. msg is the key of the
// first problem, empty when the form is valid.
func bankForm(r *http.Request) (topic, lang string, count int, msg string) {
	topic, lang = strings.TrimSpace(r.PostFormValue("topic")), r.PostFormValue("language")
	count, err := strconv.Atoi(r.PostFormValue("count"))
	switch {
	case topic == "":
		msg = "bank.err.topic"
	case !text.Supported(lang):
		msg = "bank.err.language"
	case err != nil || count < 1:
		msg = "bank.err.count"
	}
	return topic, lang, count, msg
}

// examProblem shows the Exam's Admin page with the message msg names.
func (a *App) examProblem(w http.ResponseWriter, r *http.Request, c course, e exam, status int, msg string) {
	lang, err := a.language(r)
	if err == nil {
		e.Topics, err = a.bankTopics(r.Context())
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "exam", page{Course: &c, Exam: &e, Error: text.T(lang, msg)})
}

// bankMatch picks, as b, the published bank questions of format $2, topic $3 (ignoring case), language $4 and
// Difficulty $5 (any when empty; one with none matches only that) that Exam $1 has not copied yet.
const bankMatch = `b.format = $2 AND lower(b.topic) = lower($3) AND b.language = $4 AND NOT b.is_draft
	AND ($5 = '' OR b.difficulty = $5)
	AND b.id NOT IN (SELECT bank_question_id FROM exam_questions WHERE exam_id = $1 AND bank_question_id IS NOT NULL)`

// copyFromBank copies up to count bank questions matching bankMatch, then adds aiWritten as new bank questions under
// the topic and language with copies in the Exam, all in one transaction. It returns how many it copied from the bank.
func (a *App) copyFromBank(ctx context.Context, e exam, topic, lang, difficulty string, count int, aiWritten []examQuestion) (int64, error) {
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
			SELECT id, prompt, model_answer, difficulty, row_number() OVER () AS n FROM (
				SELECT b.id, b.prompt, b.model_answer, b.difficulty FROM bank_questions b WHERE `+bankMatch+`
				ORDER BY random() LIMIT $6) r),
		copied AS (
			INSERT INTO exam_questions (exam_id, position, prompt, is_draft, bank_question_id, model_answer, difficulty)
			SELECT $1, (SELECT COALESCE(MAX(position), 0) FROM exam_questions WHERE exam_id = $1) + n, prompt, false, id, model_answer, difficulty
			FROM picked
			RETURNING id, bank_question_id),
		options AS (
			INSERT INTO exam_options (question_id, position, text, is_correct)
			SELECT c.id, o.position, o.text, o.is_correct FROM copied c JOIN bank_options o ON o.question_id = c.bank_question_id)
		SELECT count(*) FROM copied`, examID, e.Format, topic, lang, difficulty, count).Scan(&added)
	if err != nil {
		return 0, err
	}
	for _, q := range aiWritten {
		if err := insertBankedQuestion(ctx, tx, examID, e.Format, topic, lang, q); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}
