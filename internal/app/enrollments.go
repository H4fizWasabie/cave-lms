package app

import (
	"context"
	"net/http"
)

// enrolled reports whether the Learner is a member of the Course. It is the only thing that grants Course access.
func (a *App) enrolled(ctx context.Context, learnerID, courseID int64) (bool, error) {
	var n int
	err := a.db.QueryRowContext(ctx,
		`SELECT count(*) FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learnerID, courseID).Scan(&n)
	return n > 0, err
}

// requireLearner guards a Learner page: Visitors are sent to the login page.
func (a *App) requireLearner(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			serverError(w, err)
			return
		}
		if learner == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

func (a *App) joinCourse(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !enrolled && !c.Open {
		a.render(w, http.StatusConflict, "course", page{Course: &c, Learner: learner, Error: "This Course is closed to new joins."})
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO enrollments (learner_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		learner.ID, c.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

func (a *App) leaveCourse(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learner.ID, c.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

// myCourses lists the Courses the logged-in Learner has joined.
func (a *App) myCourses(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	list, err := a.queryCourses(r, `SELECT `+courseColumns+` FROM courses
		WHERE id IN (SELECT course_id FROM enrollments WHERE learner_id = $1)
		ORDER BY start_date NULLS LAST, id`, learner.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "my_courses", page{Learner: learner, Courses: list})
}
