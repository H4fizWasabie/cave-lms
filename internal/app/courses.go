package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// course is a Course as the pages show it. StartDate is YYYY-MM-DD, or empty when none.
type course struct {
	ID          int64
	Title       string
	Description string
	StartDate   string
	Open        bool
}

const courseColumns = `id, title, description, COALESCE(start_date::text, ''), is_open`

// courses lists every Course for Visitors and Learners.
func (a *App) courses(w http.ResponseWriter, r *http.Request) {
	list, err := a.listCourses(r)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "courses", page{Courses: list})
}

func (a *App) showCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, http.StatusOK, "course", page{Course: &c})
}

func (a *App) adminCourses(w http.ResponseWriter, r *http.Request) {
	list, err := a.listCourses(r)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "admin_courses", page{Courses: list})
}

func (a *App) newCourseForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "course_form", page{Course: &course{}})
}

func (a *App) editCourseForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.render(w, http.StatusOK, "course_form", page{Course: &c})
}

func (a *App) createCourse(w http.ResponseWriter, r *http.Request) {
	c, msg := courseFromForm(r)
	if msg != "" {
		a.render(w, http.StatusUnprocessableEntity, "course_form", page{Course: &c, Error: msg})
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO courses (title, description, start_date) VALUES ($1, $2, NULLIF($3, '')::date)`,
		c.Title, c.Description, c.StartDate)
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (a *App) updateCourse(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	c, msg := courseFromForm(r)
	c.ID = existing.ID
	if msg != "" {
		a.render(w, http.StatusUnprocessableEntity, "course_form", page{Course: &c, Error: msg})
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`UPDATE courses SET title = $2, description = $3, start_date = NULLIF($4, '')::date WHERE id = $1`,
		c.ID, c.Title, c.Description, c.StartDate)
	if err != nil {
		serverError(w, err)
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
			serverError(w, err)
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
	a.render(w, http.StatusOK, "delete_course", page{Course: &c})
}

func (a *App) deleteCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	// ponytail: no Enrollments or Lessons yet, so the confirm page is the only guard. Those tickets add
	// ON DELETE RESTRICT here and show a "cannot delete" message on the foreign key error.
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM courses WHERE id = $1`, c.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

// courseFor loads the Course named by the {id} path value. When there is none it writes
// the Not found page and returns false.
func (a *App) courseFor(w http.ResponseWriter, r *http.Request) (course, bool) {
	var c course
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.courseNotFound(w)
		return course{}, false
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT `+courseColumns+` FROM courses WHERE id = $1`, id).
		Scan(&c.ID, &c.Title, &c.Description, &c.StartDate, &c.Open)
	if errors.Is(err, sql.ErrNoRows) {
		a.courseNotFound(w)
		return course{}, false
	}
	if err != nil {
		serverError(w, err)
		return course{}, false
	}
	return c, true
}

func (a *App) courseNotFound(w http.ResponseWriter) {
	a.render(w, http.StatusNotFound, "message", page{Title: "Course not found", Message: "There is no Course at this address."})
}

func (a *App) listCourses(r *http.Request) ([]course, error) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT `+courseColumns+` FROM courses ORDER BY start_date NULLS LAST, id`)
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

// courseFromForm reads the Course form. msg is the first problem found, empty when the form is valid.
func courseFromForm(r *http.Request) (c course, msg string) {
	c.Title = strings.TrimSpace(r.PostFormValue("title"))
	c.Description = strings.TrimSpace(r.PostFormValue("description"))
	c.StartDate = strings.TrimSpace(r.PostFormValue("start_date"))
	switch {
	case c.Title == "":
		msg = "Enter a title."
	case c.Description == "":
		msg = "Enter a description."
	case c.StartDate != "":
		if _, err := time.Parse(time.DateOnly, c.StartDate); err != nil {
			msg = "Enter the start date as YYYY-MM-DD, or leave it empty."
		}
	}
	return c, msg
}
