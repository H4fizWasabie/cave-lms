package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
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
	IC          string // formatted, or "" when unset
	Done, Total int    // Lessons completed out of the Course's published Lessons
	Best        int    // best Exam score, shown when HasBest
	HasBest     bool   // false when the Learner has no finished attempt
	Certificate string // "None", "Issued <date>" or "Revoked", in the page's language
}

// adminCourse is the Admin's page for one Course: the Learners enrolled in it.
func (a *App) adminCourse(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT l.id, l.name, l.email, coalesce(l.ic, '') FROM enrollments e JOIN learners l ON l.id = e.learner_id
		WHERE e.course_id = $1 ORDER BY l.name, l.id`, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer rows.Close()
	var list []enrollee
	for rows.Next() {
		var e enrollee
		if err := rows.Scan(&e.ID, &e.Name, &e.Email, &e.IC); err != nil {
			a.serverError(w, r, err)
			return
		}
		e.IC = formatIC(e.IC)
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	// ponytail: three small queries per Learner (N+1). Fine for one Course's roster; batch them if a Course grows past a few hundred.
	for i := range list {
		e := &list[i]
		if e.Done, e.Total, err = a.courseProgress(r.Context(), e.ID, c.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		if e.Best, e.HasBest, err = a.bestExamScore(r.Context(), e.ID, c.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		if e.Certificate, err = a.certificateStatus(r.Context(), lang, e.ID, c.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.render(w, r, http.StatusOK, "admin_course", page{Course: &c, Enrollees: list})
}

// removeLearner is the Admin removing a Learner from a Course. Access ends on the next request.
func (a *App) removeLearner(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	learnerID, err := strconv.ParseInt(r.PathValue("learnerID"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learnerID, c.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

// giveCertificate is the Admin giving a Learner in a Live Course a Certificate by hand, whatever their attempts. It
// issues through the same path as an earned one, skipping the Exam check but not the IC check.
func (a *App) giveCertificate(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	learnerID, err := strconv.ParseInt(r.PathValue("learnerID"), 10, 64)
	if err != nil || !c.Live() {
		a.notFound(w, r)
		return
	}
	var name, ic string
	var valid sql.NullBool // whether the Learner's Certificate for the Course is valid; null when they have none
	err = a.db.QueryRowContext(r.Context(), `SELECT l.name, coalesce(l.ic, ''), (SELECT bool_or(revoked_at IS NULL)
		FROM certificates c WHERE c.learner_id = e.learner_id AND c.course_id = e.course_id) FROM enrollments e
		JOIN learners l ON l.id = e.learner_id WHERE e.learner_id = $1 AND e.course_id = $2`, learnerID, c.ID).Scan(&name, &ic, &valid)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	notGiven := func(status int, key string) {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, status, "message", page{Title: text.T(lang, "adm.course.not-given"), Message: text.T(lang, key, name)})
	}
	switch {
	case valid.Valid && valid.Bool:
		notGiven(http.StatusConflict, "adm.course.give-has-valid")
		return
	case valid.Valid: // only revoked ones: a corrected Certificate is the Admin's reissue
		notGiven(http.StatusConflict, "adm.course.give-revoked")
		return
	case ic == "":
		notGiven(http.StatusUnprocessableEntity, "adm.course.give-no-ic")
		return
	}
	if err := a.issueCertificate(r.Context(), learnerID, c.ID, true); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

// requireLearner guards a Learner page: Visitors are sent to the login page,
// which returns them to the page they asked for when it was a GET.
func (a *App) requireLearner(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if learner == nil {
			login := "/login"
			if r.Method == http.MethodGet {
				login += "?next=" + url.QueryEscape(r.URL.RequestURI())
			}
			http.Redirect(w, r, login, http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

func (a *App) joinCourse(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	if !c.Published {
		a.courseNotFound(w, r)
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !enrolled && !c.Joinable() {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusConflict, "course", page{Course: &c, Learner: learner, Error: text.T(lang, "course.closed")})
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO enrollments (learner_id, course_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		learner.ID, c.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

// confirmLeaveCourse asks the Learner to confirm leaving a Course they have joined.
func (a *App) confirmLeaveCourse(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !enrolled {
		http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "leave_course", page{Course: &c, Learner: learner})
}

// leaveCourse leaves a Course once the Learner has confirmed it on the confirmation page; a post without that
// confirmation, such as from an old Course page, goes to the confirmation page instead.
func (a *App) leaveCourse(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	if !confirmed(r) {
		http.Redirect(w, r, "/courses/"+r.PathValue("id")+"/leave", http.StatusSeeOther)
		return
	}
	if _, err := a.db.ExecContext(r.Context(),
		`DELETE FROM enrollments WHERE learner_id = $1 AND course_id = $2`, learner.ID, c.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
}

// myCourse is one joined Course on My courses: the Learner's progress, next step and standing on each Exam.
type myCourse struct {
	Course      course
	Done, Total int
	Step        nextStep
	Exams       []courseExam
}

// myCoursesPage is the Learner's home: their Courses and the Certificates they have earned.
type myCoursesPage struct {
	pageBase
	Courses      []myCourse
	Certificates []certificate
}

// myCourses lists the Courses the logged-in Learner has joined, each with its progress, next step and Exam status,
// then their valid Certificates.
func (a *App) myCourses(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	list, err := a.queryCourses(r, `SELECT `+courseColumns+` FROM courses
		WHERE id IN (SELECT course_id FROM enrollments WHERE learner_id = $1)
		ORDER BY start_date NULLS LAST, id`, learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := myCoursesPage{pageBase: pageBase{Learner: learner}}
	ctx := r.Context()
	for _, c := range list {
		mc := myCourse{Course: c}
		if mc.Done, mc.Total, err = a.courseProgress(ctx, learner.ID, c.ID); err == nil {
			if mc.Step, err = a.nextStep(ctx, learner, c); err == nil {
				mc.Exams, err = a.courseExams(ctx, learner, c, mc.Done, mc.Total)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		p.Courses = append(p.Courses, mc)
	}
	if p.Certificates, err = a.queryCertificates(ctx, `SELECT `+certificateColumns+` FROM certificates
		WHERE learner_id = $1 AND revoked_at IS NULL ORDER BY issued_at, year, number`, learner.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_courses", &p)
}
