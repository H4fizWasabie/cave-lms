package app

import (
	"bytes"
	"cmp"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/go-pdf/fpdf"
)

// answerSheetPDF lays out a passed attempt's Answer sheet on portrait A4: the Learner, Course, Exam and mark, then
// every question with the Learner's answer and the correct option, or the model answer, the mark and the AI's reason. Its headings
// are in lang.
func answerSheetPDF(lang, name, courseTitle, format string, score int, questions []attemptQuestion) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	// Uncompressed, so the text stays greppable in tests, as on Certificates.
	pdf.SetCompression(false)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	line := func(style string, size float64, s string) {
		pdf.SetFont("Helvetica", style, size)
		pdf.MultiCell(0, size/2, tr(s), "", "L", false)
	}
	line("B", 20, text.T(lang, "answer-sheet.title"))
	pdf.Ln(4)
	line("", 12, text.T(lang, "answer-sheet.learner", name))
	line("", 12, text.T(lang, "answer-sheet.course", courseTitle))
	line("", 12, text.T(lang, "answer-sheet.exam", text.T(lang, "bank.format."+format)))
	line("", 12, text.T(lang, "exam.score", score))
	for i, q := range questions {
		pdf.Ln(6)
		line("B", 12, strconv.Itoa(i+1)+". "+q.Prompt)
		answer := text.T(lang, "answer-sheet.no-answer")
		if format == written {
			line("", 11, text.T(lang, "answer-sheet.your-answer", cmp.Or(q.Answer, answer)))
			line("", 11, text.T(lang, "answer-sheet.model-answer", q.ModelAnswer))
			line("", 11, text.T(lang, "answer-sheet.mark", q.MarkValue(), writtenMarks))
			if q.Reason != "" {
				line("", 11, text.T(lang, "answer-sheet.reason", q.Reason))
			}
			continue
		}
		if q.Chosen >= 0 {
			answer = q.Options[q.Chosen].Text
		}
		line("", 11, text.T(lang, "answer-sheet.your-answer", answer))
		for _, o := range q.Options {
			if o.Correct {
				line("", 11, text.T(lang, "answer-sheet.correct-answer", o.Text))
			}
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// downloadAnswerSheet serves the logged-in Learner the Answer sheet of their passed attempt at the Exam. Before a
// released pass there is none: Not found.
func (a *App) downloadAnswerSheet(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, e, ok := a.examFor(w, r)
	if !ok {
		return
	}
	att, err := scanAttempt(a.db.QueryRowContext(r.Context(), `SELECT `+attemptColumns+` FROM exam_attempts
		WHERE learner_id = $1 AND exam_id = $2 AND released_at IS NOT NULL AND passed ORDER BY id DESC LIMIT 1`,
		learner.ID, e.ID))
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	pdf, err := answerSheetPDF(lang, learner.Name, c.Title, e.Format, att.Score, att.Questions)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="answer-sheet.pdf"`)
	_, _ = w.Write(pdf)
}
