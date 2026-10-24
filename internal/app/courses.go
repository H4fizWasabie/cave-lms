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

// course is a Course as the pages show it. StartDate and EndDate are YYYY-MM-DD, or empty when none. A Live Course
// has an EndDate and Days (1 to 5); a self-paced one has neither (ADR 0012).
type course struct {
	ID             int64
	Title          string
	Description    string
	StartDate      string
	EndDate        string
	Days           int
	OrganisationID int64 // the Organisation a Live Course is run for, 0 when none
	Open           bool  // the Admin's open or closed to new joins
	Published      bool  // false for a Draft, which only the Admin sees
	Ended          bool  // a Live Course past the end of its end date, Malaysia time
	Upcoming       bool  // before the start of its start date, Malaysia time
}

// Live reports whether the Course is a Live Course.
func (c course) Live() bool { return c.Days > 0 }

// Joinable reports whether a Learner may join the Course now.
func (c course) Joinable() bool { return c.Open && !c.Ended }

// Level is the Live Course's PCC Level, following from its days, or "" for a self-paced Course.
func (c course) Level() string {
	switch c.Days {
	case 0:
		return ""
	case 1:
		return "Basic"
	case 2:
		return "Foundation"
	case 3:
		return "Practitioner"
	default:
		return "Professional"
	}
}

// Badge is the static file of the PCC Level's badge, or "" for a self-paced Course.
func (c course) Badge() string {
	if c.Days == 0 {
		return ""
	}
	return "pcc-" + strings.ToLower(c.Level()) + ".jpg"
}

// EndOn is the end date written in lang, or "" when the Course has none.
func (c course) EndOn(lang string) string {
	day, err := time.Parse(time.DateOnly, c.EndDate)
	if err != nil {
		return ""
	}
	return text.Date(lang, day)
}

// StartOn is the start date written in lang, or "" when the Course has none.
func (c course) StartOn(lang string) string {
	day, err := time.Parse(time.DateOnly, c.StartDate)
	if err != nil {
		return ""
	}
	return text.Date(lang, day)
}

const courseColumns = `id, title, description, COALESCE(start_date::text, ''), COALESCE(end_date::text, ''),
	COALESCE(days, 0), is_open, published, COALESCE(organisation_id, 0)`

// courses lists every Course for Visitors and Learners.
func (a *App) courses(w http.ResponseWriter, r *http.Request) {
	list, err := a.queryCourses(r, `SELECT `+courseColumns+` FROM courses WHERE published ORDER BY start_date NULLS LAST, id`)
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
	if !c.Published && (learner == nil || !learner.IsAdmin) {
		a.courseNotFound(w, r)
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
	a.renderCourseForm(w, r, http.StatusOK, page{Course: &c})
}

// renderCourseForm renders the Course form with the Organisations a Live Course may be run for.
func (a *App) renderCourseForm(w http.ResponseWriter, r *http.Request, status int, p page) {
	var err error
	if p.Organisations, err = a.organisations(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, status, "course_form", p)
}

func (a *App) createCourse(w http.ResponseWriter, r *http.Request) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, msg := courseFromForm(r, lang, false)
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
	c, msg := courseFromForm(r, lang, existing.Live())
	c.ID = existing.ID
	if msg != "" {
		a.renderCourseForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE courses SET title = $2, description = $3, start_date = NULLIF($4, '')::date,
		end_date = NULLIF($5, '')::date, days = NULLIF($6, 0), organisation_id = NULLIF($7, 0) WHERE id = $1`,
		c.ID, c.Title, c.Description, c.StartDate, c.EndDate, c.Days, c.OrganisationID)
	if isForeignKeyError(err) { // the Organisation was deleted while the form was open
		a.renderCourseForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Error: text.T(lang, "adm.courses.err.organisation")})
		return
	}
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
	return a.scanCourse(a.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE id = $1`, id))
}

// scanCourse reads one row of courseColumns.
func (a *App) scanCourse(row interface{ Scan(...any) error }) (course, error) {
	var c course
	err := row.Scan(&c.ID, &c.Title, &c.Description, &c.StartDate, &c.EndDate, &c.Days, &c.Open, &c.Published, &c.OrganisationID)
	if end, perr := time.ParseInLocation(time.DateOnly, c.EndDate, malaysia); perr == nil {
		c.Ended = !a.now().Before(end.AddDate(0, 0, 1))
	}
	if start, perr := time.ParseInLocation(time.DateOnly, c.StartDate, malaysia); perr == nil {
		c.Upcoming = a.now().Before(start)
	}
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
		c, err := a.scanCourse(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// courseFromForm reads the Course form, with the end date and days when the Course is live. msg is the first problem
// found, in lang, empty when the form is valid.
func courseFromForm(r *http.Request, lang string, live bool) (c course, msg string) {
	c.Title = strings.TrimSpace(r.PostFormValue("title"))
	c.Description = strings.TrimSpace(r.PostFormValue("description"))
	c.StartDate = strings.TrimSpace(r.PostFormValue("start_date"))
	start, startErr := time.Parse(time.DateOnly, c.StartDate)
	var end time.Time
	endErr := errors.New("self-paced")
	daysText := ""
	var orgErr error
	if live {
		c.EndDate = strings.TrimSpace(r.PostFormValue("end_date"))
		end, endErr = time.Parse(time.DateOnly, c.EndDate)
		daysText = strings.TrimSpace(r.PostFormValue("days"))
		c.Days, _ = strconv.Atoi(daysText)
		if v := r.PostFormValue("organisation_id"); v != "" { // empty is none
			c.OrganisationID, orgErr = strconv.ParseInt(v, 10, 64)
		}
	}
	switch {
	case c.Title == "":
		msg = text.T(lang, "adm.courses.err.title")
	case c.Description == "":
		msg = text.T(lang, "adm.courses.err.description")
	case live && startErr != nil:
		msg = text.T(lang, "adm.courses.err.live-start-date")
	case live && (endErr != nil || end.Before(start)):
		msg = text.T(lang, "adm.courses.err.end-date")
	case live && (c.Days < 1 || c.Days > 5 || strconv.Itoa(c.Days) != daysText):
		msg = text.T(lang, "adm.courses.err.days")
	case orgErr != nil:
		msg = text.T(lang, "adm.courses.err.organisation")
	case c.StartDate != "" && startErr != nil:
		msg = text.T(lang, "adm.courses.err.start-date")
	}
	return c, msg
}
