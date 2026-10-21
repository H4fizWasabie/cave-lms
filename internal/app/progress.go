package app

import (
	"context"
	"net/http"
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
			a.render(w, r, http.StatusForbidden, "members_only", page{Learner: learner, Course: &c,
				Message: "Join this Course to mark its Lessons complete."})
			return
		}
		l, ok := a.lessonByPath(w, r)
		if !ok {
			return
		}
		if l.CourseID != c.ID || !l.Published {
			http.NotFound(w, r)
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
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/courses/"+r.PathValue("id"), http.StatusSeeOther)
	}
}
