package migrations_test

import (
	"context"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Migration 00137 removes the web search links the AI writer left in saved questions, but only those carrying
// utm_source=openai: a link an Admin wrote stays.
func TestOnlyAIWriterCitationsAreStrippedFromSavedQuestions(t *testing.T) {
	ctx := context.Background()
	db, _ := testutil.NewDatabase(t)
	if err := migrations.DownTo(ctx, db, 136); err != nil {
		t.Fatal(err)
	}
	const (
		wrapped = " ([learn.microsoft.com](https://learn.microsoft.com/a/b%3A_c%2C_d?trk=x&utm_source=openai))"
		bare    = " [conker.ai](https://www.conker.ai/quiz?utm_source=openai)"
		own     = "Lihat [panduan](https://example.com/panduan)."
	)
	var course, exam, bank, question int64
	if err := db.QueryRow(`INSERT INTO courses (title, description, start_date, end_date, days, organisation_id, published) VALUES ('C', 'd', '2026-10-01', '2026-10-02', 1, NULL, true) RETURNING id`).Scan(&course); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO exams (course_id, format) VALUES ($1, 'multiple-choice') RETURNING id`, course).Scan(&exam); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO bank_questions (format, topic, language, prompt, model_answer) VALUES ('written', 't', 'ms', $1, $2) RETURNING id`,
		"Apakah AI?"+wrapped, "Ia pintar."+bare).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO bank_options (question_id, position, text, is_correct) VALUES ($1, 0, $2, true), ($1, 1, $3, false)`, bank, "Betul"+bare, own+" Salah."); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO exam_questions (exam_id, position, prompt, model_answer) VALUES ($1, 0, $2, $3) RETURNING id`,
		exam, "Apakah AI?"+wrapped, "Ia pintar."+bare).Scan(&question); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO exam_options (question_id, position, text, is_correct) VALUES ($1, 0, $2, true), ($1, 1, $3, false)`, question, "Betul"+bare, own+" Salah."); err != nil {
		t.Fatal(err)
	}

	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, query string
		id          int64
		want        []string
	}{
		{"bank question", `SELECT prompt, model_answer FROM bank_questions WHERE id = $1`, bank, []string{"Apakah AI?", "Ia pintar."}},
		{"bank options", `SELECT string_agg(text, '|' ORDER BY position), '' FROM bank_options WHERE question_id = $1`, bank, []string{"Betul|" + own + " Salah.", ""}},
		{"Exam question", `SELECT prompt, model_answer FROM exam_questions WHERE id = $1`, question, []string{"Apakah AI?", "Ia pintar."}},
		{"Exam options", `SELECT string_agg(text, '|' ORDER BY position), '' FROM exam_options WHERE question_id = $1`, question, []string{"Betul|" + own + " Salah.", ""}},
	} {
		var a, b string
		if err := db.QueryRow(c.query, c.id).Scan(&a, &b); err != nil {
			t.Fatal(err)
		}
		if a != c.want[0] || b != c.want[1] {
			t.Errorf("%s = %q and %q, want %q and %q", c.name, a, b, c.want[0], c.want[1])
		}
	}
}
