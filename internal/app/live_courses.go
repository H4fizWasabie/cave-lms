package app

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
)

// duplicateCourseForm is the form that duplicates a Course into a Live Course, filled with the original's title.
func (a *App) duplicateCourseForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "course_form", page{Course: &course{Title: c.Title, Description: c.Description}, DuplicateOf: c.ID})
}

// duplicateCourse copies a Course, its Lessons and its Exams with their questions into a new Draft Live Course.
// Learners, attempts and Certificates stay with the original.
func (a *App) duplicateCourse(w http.ResponseWriter, r *http.Request) {
	from, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, msg := courseFromForm(r, lang, true)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "course_form", page{Course: &c, DuplicateOf: from.ID, Error: msg})
		return
	}
	id, err := a.copyCourse(r.Context(), from.ID, c)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+strconv.FormatInt(id, 10)+"/edit", http.StatusSeeOther)
}

func (a *App) copyCourse(ctx context.Context, from int64, c course) (int64, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO courses (title, description, start_date, end_date, days, published)
		VALUES ($1, $2, $3::date, $4::date, $5, false) RETURNING id`, c.Title, c.Description, c.StartDate, c.EndDate, c.Days).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO lessons (course_id, position, title, objective, body, gallery_item_id, published)
		SELECT $2, position, title, objective, body, gallery_item_id, published FROM lessons WHERE course_id = $1`, from, id); err != nil {
		return 0, err
	}
	exams, err := idsOf(ctx, tx, `SELECT id FROM exams WHERE course_id = $1 ORDER BY id`, from)
	if err != nil {
		return 0, err
	}
	for _, examID := range exams {
		var newExam int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO exams (course_id, format, pass_mark, time_limit_minutes, max_attempts, draw_count)
			SELECT $2, format, pass_mark, time_limit_minutes, max_attempts, draw_count FROM exams WHERE id = $1 RETURNING id`,
			examID, id).Scan(&newExam); err != nil {
			return 0, err
		}
		questions, err := idsOf(ctx, tx, `SELECT id FROM exam_questions WHERE exam_id = $1 ORDER BY position, id`, examID)
		if err != nil {
			return 0, err
		}
		// ponytail: two statements per question; fine for an Exam's pool of tens to a few hundred questions.
		for _, q := range questions {
			var newQ int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO exam_questions (exam_id, position, prompt, is_draft, bank_question_id, model_answer)
				SELECT $2, position, prompt, is_draft, bank_question_id, model_answer FROM exam_questions WHERE id = $1 RETURNING id`,
				q, newExam).Scan(&newQ); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO exam_options (question_id, position, text, is_correct)
				SELECT $2, position, text, is_correct FROM exam_options WHERE question_id = $1`, q, newQ); err != nil {
				return 0, err
			}
		}
	}
	return id, tx.Commit()
}

// idsOf runs a query selecting one bigint column and returns every value, so the rows are closed before the next query.
func idsOf(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// publishCourse is the Admin publishing a Draft Course, so Learners see it.
func (a *App) publishCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE courses SET published = true WHERE id = $1`, c.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}
