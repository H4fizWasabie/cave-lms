package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// course is a Course as the pages show it. StartDate is YYYY-MM-DD, or empty when none.
type course struct {
	ID          int64
	Title       string
	Description string
	StartDate   string
	Open        bool
}

// StartOn is the start date written in lang, or "" when the Course has none.
func (c course) StartOn(lang string) string {
	day, err := time.Parse(time.DateOnly, c.StartDate)
	if err != nil {
		return ""
	}
	return text.Date(lang, day)
}

const courseColumns = `id, title, description, COALESCE(start_date::text, ''), is_open`

// courses lists every Course for Visitors and Learners.
func (a *App) courses(w http.ResponseWriter, r *http.Request) {
	list, err := a.listCourses(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "courses", page{Courses: list})
}

func (a *App) showCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	enrolled := false
	if learner != nil {
		if enrolled, err = a.enrolled(r.Context(), learner.ID, c.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	// Everyone sees the published Lesson titles, so they know what the Course contains; only members open them.
	lessons, err := a.lessonsOf(r.Context(), c.ID, learner != nil && learner.IsAdmin)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var done, total int
	if enrolled {
		if done, total, err = a.courseProgress(r.Context(), learner.ID, c.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		completed, err := a.completedLessons(r.Context(), learner.ID, c.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		for i := range lessons {
			lessons[i].Completed = completed[lessons[i].ID]
		}
	}
	p := page{Course: &c, Learner: learner, Enrolled: enrolled, Lessons: lessons, Done: done, Total: total}
	if enrolled {
		e, err := a.loadExam(r.Context(), c.ID, written)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if e.Usable {
			p.WrittenExam = &e
		}
	}
	a.render(w, r, http.StatusOK, "course", p)
}

func (a *App) adminCourses(w http.ResponseWriter, r *http.Request) {
	list, err := a.listCourses(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_courses", page{Courses: list})
}

func (a *App) newCourseForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "course_form", page{Course: &course{}})
}

func (a *App) editCourseForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "course_form", page{Course: &c})
}

func (a *App) createCourse(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, msg := courseFromForm(r, lang)
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "course_form", page{Course: &c, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(),
		`INSERT INTO courses (title, description, start_date) VALUES ($1, $2, NULLIF($3, '')::date)`,
		c.Title, c.Description, c.StartDate)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (a *App) updateCourse(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, msg := courseFromForm(r, lang)
	c.ID = existing.ID
	if msg != "" {
		a.render(w, r, http.StatusUnprocessableEntity, "course_form", page{Course: &c, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(),
		`UPDATE courses SET title = $2, description = $3, start_date = NULLIF($4, '')::date WHERE id = $1`,
		c.ID, c.Title, c.Description, c.StartDate)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

// setCourseOpen returns the handler that opens or closes a Course to new joins.
func (a *App) setCourseOpen(open bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := a.courseFor(w, r)
		if !ok {
			return
		}
		if _, err := a.db.ExecContext(r.Context(), `UPDATE courses SET is_open = $2 WHERE id = $1`, c.ID, open); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
	}
}

func (a *App) confirmDeleteCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "delete_course", page{Course: &c})
}

func (a *App) deleteCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	// Enrollments, Lessons, Exam attempts and Certificates hold ON DELETE RESTRICT, so the database refuses the delete.
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM courses WHERE id = $1`, c.ID)
	if isForeignKeyError(err) {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusConflict, "delete_course", page{Course: &c, Error: text.T(lang, "adm.courses.err.in-use")})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

// courseFor loads the Course named by the {id} path value. When there is none it writes
// the Not found page and returns false.
func (a *App) courseFor(w http.ResponseWriter, r *http.Request) (course, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.courseNotFound(w, r)
		return course{}, false
	}
	c, err := a.courseByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		a.courseNotFound(w, r)
		return course{}, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return course{}, false
	}
	return c, true
}

// courseByID loads one Course; sql.ErrNoRows when there is none.
func (a *App) courseByID(ctx context.Context, id int64) (course, error) {
	var c course
	err := a.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE id = $1`, id).
		Scan(&c.ID, &c.Title, &c.Description, &c.StartDate, &c.Open)
	return c, err
}

func (a *App) courseNotFound(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusNotFound, "message", page{Title: text.T(lang, "course.not-found.title"), Message: text.T(lang, "course.not-found")})
}

func (a *App) listCourses(r *http.Request) ([]course, error) {
	return a.queryCourses(r, `SELECT `+courseColumns+` FROM courses ORDER BY start_date NULLS LAST, id`)
}

// queryCourses runs a query that selects courseColumns and returns the Courses it finds.
func (a *App) queryCourses(r *http.Request, query string, args ...any) ([]course, error) {
	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []course
	for rows.Next() {
		var c course
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.StartDate, &c.Open); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// courseFromForm reads the Course form. msg is the first problem found, in lang, empty when the form is valid.
func courseFromForm(r *http.Request, lang string) (c course, msg string) {
	c.Title = strings.TrimSpace(r.PostFormValue("title"))
	c.Description = strings.TrimSpace(r.PostFormValue("description"))
	c.StartDate = strings.TrimSpace(r.PostFormValue("start_date"))
	switch {
	case c.Title == "":
		msg = text.T(lang, "adm.courses.err.title")
	case c.Description == "":
		msg = text.T(lang, "adm.courses.err.description")
	case c.StartDate != "":
		if _, err := time.Parse(time.DateOnly, c.StartDate); err != nil {
			msg = text.T(lang, "adm.courses.err.start-date")
		}
	}
	return c, msg
}
