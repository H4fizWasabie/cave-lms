package app

import (
	"context"
	"net/http"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// courseProgress counts the Learner's completed Lessons against the Course's published Lessons.
func (a *App) courseProgress(ctx context.Context, learnerID, courseID int64) (done, total int, err error) {
	err = a.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE EXISTS (SELECT 1 FROM lesson_completions c
			WHERE c.lesson_id = l.id AND c.learner_id = $1)), count(*)
		FROM lessons l WHERE l.course_id = $2 AND l.published`, learnerID, courseID).Scan(&done, &total)
	return done, total, err
}

// completedLessons returns the IDs of the Lessons the Learner has completed in a Course.
func (a *App) completedLessons(ctx context.Context, learnerID, courseID int64) (map[int64]bool, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT c.lesson_id FROM lesson_completions c
		JOIN lessons l ON l.id = c.lesson_id WHERE c.learner_id = $1 AND l.course_id = $2`, learnerID, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	done := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		done[id] = true
	}
	return done, rows.Err()
}

// setLessonComplete returns the handler that marks a published Lesson complete or undoes that, for an enrolled Learner.
// Then it goes back to the Course page, or, as the post's "then" asks: "next", on to the next published Lesson (the
// Course page after the last), or "lesson", back to the Lesson.
func (a *App) setLessonComplete(complete bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			lang, err := a.language(r)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			a.render(w, r, http.StatusForbidden, "members_only", page{Learner: learner, Course: &c,
				Message: text.T(lang, "lesson.join-to-mark")})
			return
		}
		l, ok := a.lessonByPath(w, r)
		if !ok {
			return
		}
		if l.CourseID != c.ID || !l.Published {
			a.notFound(w, r)
			return
		}
		if complete {
			_, err = a.db.ExecContext(r.Context(), `INSERT INTO lesson_completions (learner_id, lesson_id)
				VALUES ($1, $2) ON CONFLICT DO NOTHING`, learner.ID, l.ID)
		} else {
			_, err = a.db.ExecContext(r.Context(), `DELETE FROM lesson_completions WHERE learner_id = $1 AND lesson_id = $2`,
				learner.ID, l.ID)
		}
		if err == nil && complete {
			err = a.issueCertificateIfEarned(r.Context(), learner.ID, c.ID)
		}
		dest := "/courses/" + r.PathValue("id")
		if r.PostFormValue("then") == "lesson" {
			dest = lessonPath(c.ID, l.ID)
		}
		if err == nil && r.PostFormValue("then") == "next" {
			var place lessonPlace
			place, err = a.lessonPlaceOf(r.Context(), c.ID, l.ID, false)
			if place.Next != nil {
				dest = lessonPath(c.ID, place.Next.ID)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
	}
}

// lessonPlace is where a Lesson stands in its Course: Lesson N of Total, with the Lessons before and after it (nil
// at either end).
type lessonPlace struct {
	N, Total   int
	Prev, Next *lesson
}

// lessonPlaceOf works out lessonID's place in its Course's order. Learners see only published Lessons; with drafts,
// the Admin's preview counts them too.
func (a *App) lessonPlaceOf(ctx context.Context, courseID, lessonID int64, includeDrafts bool) (lessonPlace, error) {
	list, err := a.lessonsOf(ctx, courseID, includeDrafts)
	if err != nil {
		return lessonPlace{}, err
	}
	p := lessonPlace{Total: len(list)}
	for i := range list {
		if list[i].ID != lessonID {
			continue
		}
		p.N = i + 1
		if i > 0 {
			p.Prev = &list[i-1]
		}
		if i+1 < len(list) {
			p.Next = &list[i+1]
		}
	}
	return p, nil
}

func lessonPath(courseID, lessonID int64) string {
	return "/courses/" + strconv.FormatInt(courseID, 10) + "/lessons/" + strconv.FormatInt(lessonID, 10)
}
