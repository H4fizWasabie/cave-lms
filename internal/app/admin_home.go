package app

import (
	"context"
	"fmt"
	"net/http"
)

// adminHomePage is the Admin area's home: a few counts, what is waiting for the Admin to mark, and the latest Results.
type adminHomePage struct {
	pageBase
	Learners, Courses, Certificates int // verified Learners who aren't Admins, every Course, valid Certificates
	ToMark                          []submissionsToMark
	Unmarked                        []answersToMark
	Recent                          []adminResult // the latest released Results
}

// submissionsToMark is one Assignment with Submissions the Admin has not commented on since their last file.
type submissionsToMark struct {
	AssignmentID       int64
	Title, CourseTitle string
	Count              int
}

// answersToMark is one submitted Written attempt with answers still waiting for a mark, from the AI or the Admin.
type answersToMark struct {
	AttemptID                int64
	LearnerName, CourseTitle string
	Count                    int
}

// SubmissionsToMark is how many Submissions wait for the Admin, across every Assignment.
func (p *adminHomePage) SubmissionsToMark() int {
	n := 0
	for _, s := range p.ToMark {
		n += s.Count
	}
	return n
}

// AnswersToMark is how many Written answers wait for a mark, across every attempt.
func (p *adminHomePage) AnswersToMark() int {
	n := 0
	for _, u := range p.Unmarked {
		n += u.Count
	}
	return n
}

func (a *App) adminHome(w http.ResponseWriter, r *http.Request) {
	p := &adminHomePage{}
	err := a.loadAdminHome(r.Context(), p)
	if err == nil {
		p.Recent, err = a.queryResults(r, `t.id IN (SELECT id FROM exam_attempts WHERE released_at IS NOT NULL ORDER BY released_at DESC, id DESC LIMIT 5)`)
	}
	if err != nil {
		a.serverError(w, r, fmt.Errorf("load the Admin area's home: %w", err))
		return
	}
	a.render(w, r, http.StatusOK, "admin", p)
}

// loadAdminHome fills in the counts and what needs marking.
func (a *App) loadAdminHome(ctx context.Context, p *adminHomePage) error {
	err := a.db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM learners WHERE NOT is_admin AND verified_at IS NOT NULL),
		(SELECT count(*) FROM courses),
		(SELECT count(*) FROM certificates WHERE revoked_at IS NULL)`).Scan(&p.Learners, &p.Courses, &p.Certificates)
	if err != nil {
		return err
	}
	// A Submission needs marking until the Admin comments on it, and again when a file comes in after the comment.
	rows, err := a.db.QueryContext(ctx, `
		SELECT a.id, a.title, c.title, count(*)
		FROM (SELECT assignment_id, learner_id, max(uploaded_at) AS last FROM submission_files GROUP BY 1, 2) s
		JOIN assignments a ON a.id = s.assignment_id
		JOIN courses c ON c.id = a.course_id
		LEFT JOIN submission_comments m ON m.assignment_id = s.assignment_id AND m.learner_id = s.learner_id
		WHERE m.updated_at IS NULL OR m.updated_at < s.last
		GROUP BY a.id, a.title, c.title
		ORDER BY min(s.last)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s submissionsToMark
		if err := rows.Scan(&s.AssignmentID, &s.Title, &s.CourseTitle, &s.Count); err != nil {
			return err
		}
		p.ToMark = append(p.ToMark, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = a.db.QueryContext(ctx, `
		SELECT t.id, l.name, c.title,
			(SELECT count(*) FROM jsonb_array_elements(t.questions) q WHERE q->>'Answer' <> '' AND q->'Mark' IS NULL)
		FROM (SELECT * FROM exam_attempts WHERE `+unmarkedSQL+`) t
		JOIN exams x ON x.id = t.exam_id
		JOIN learners l ON l.id = t.learner_id
		JOIN courses c ON c.id = x.course_id
		ORDER BY t.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u answersToMark
		if err := rows.Scan(&u.AttemptID, &u.LearnerName, &u.CourseTitle, &u.Count); err != nil {
			return err
		}
		p.Unmarked = append(p.Unmarked, u)
	}
	return rows.Err()
}
