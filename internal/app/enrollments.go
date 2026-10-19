package app

import (
	"context"
	"net/http"
	"strconv"
)

// enrolled reports whether the Learner is a member of the Course. It is the only thing that grants Course access.
func (a *App) enrolled(ctx context.Context, learnerID, courseID int64) (bool, error) {
	var n int
	err := a.db.QueryRowContext(ctx,
		`SELECT count(*) FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learnerID, courseID).Scan(&n)
	return n > 0, err
}

// enrollee is a Learner enrolled in a Course, as the Admin's Course page shows them.
type enrollee struct {
	ID          int64
	Name        string
	Email       string
	Done, Total int    // Lessons completed out of the Course's published Lessons
	Best        int    // best Exam score, shown when HasBest
	HasBest     bool   // false when the Learner has no finished attempt
	Certificate string // "None", "Issued <date>" or "Revoked"
}

// adminCourse is the Admin's page for one Course: the Learners enrolled in it.
func (a *App) adminCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT l.id, l.name, l.email FROM enrollments e JOIN learners l ON l.id = e.learner_id
		WHERE e.course_id = $1 ORDER BY l.name, l.id`, c.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	var list []enrollee
	for rows.Next() {
		var e enrollee
		if err := rows.Scan(&e.ID, &e.Name, &e.Email); err != nil {
			serverError(w, err)
			return
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	// ponytail: three small queries per Learner (N+1). Fine for one Course's roster; batch them if a Course grows past a few hundred.
	for i := range list {
		e := &list[i]
		if e.Done, e.Total, err = a.courseProgress(r.Context(), e.ID, c.ID); err != nil {
			serverError(w, err)
			return
		}
		if e.Best, e.HasBest, err = a.bestExamScore(r.Context(), e.ID, c.ID); err != nil {
			serverError(w, err)
			return
		}
		if e.Certificate, err = a.certificateStatus(r.Context(), e.ID, c.ID); err != nil {
			serverError(w, err)
			return
		}
	}
	a.render(w, http.StatusOK, "admin_course", page{Course: &c, Enrollees: list})
}

// removeLearner is the Admin removing a Learner from a Course. Access ends on the next request.
func (a *App) removeLearner(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	learnerID, err := strconv.ParseInt(r.PathValue("learnerID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learnerID, c.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+r.PathValue("id"), http.StatusSeeOther)
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
