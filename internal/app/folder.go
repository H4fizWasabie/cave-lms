package app

import (
	"context"
	"net/http"
)

// folderAssignment is one Assignment in a Learner's Folder: their files in it and the Admin's comment.
type folderAssignment struct {
	ID        int64
	Title     string
	Published bool
	Comment   string
	Files     []submissionFile
}

// folderCourse is one Course in a Learner's Folder. A Course they left is read only.
type folderCourse struct {
	ID          int64
	Title       string
	Enrolled    bool
	Assignments []folderAssignment
}

type folderPage struct {
	pageBase
	Workbooks []workbook // a Learner with none sees no Workbooks part
	Courses   []folderCourse
}

// myFolder is the Learner's Folder: their Workbooks, if they have any, and their Submissions by Course, with the
// Admin's comments.
func (a *App) myFolder(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	list, err := a.folder(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	books, err := a.workbooksOf(r.Context(), learner.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "my_folder", &folderPage{pageBase: pageBase{Learner: learner}, Workbooks: books, Courses: list})
}

// folder lists every Assignment the Learner has files in or a comment on, by Course.
func (a *App) folder(ctx context.Context, learnerID int64) ([]folderCourse, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT c.id, c.title, EXISTS (SELECT 1 FROM enrollments e WHERE e.course_id = c.id AND e.learner_id = $1),
			a.id, a.title, a.published, COALESCE(sc.body, '')
		FROM assignments a JOIN courses c ON c.id = a.course_id
		LEFT JOIN submission_comments sc ON sc.assignment_id = a.id AND sc.learner_id = $1
		WHERE sc.body IS NOT NULL OR EXISTS (SELECT 1 FROM submission_files f WHERE f.assignment_id = a.id AND f.learner_id = $1)
		ORDER BY lower(c.title), c.id, a.position, a.id`, learnerID)
	if err != nil {
		return nil, err
	}
	var list []folderCourse
	at := map[int64][2]int{} // Assignment ID to its place in list
	for rows.Next() {
		var c folderCourse
		var as folderAssignment
		if err := rows.Scan(&c.ID, &c.Title, &c.Enrolled, &as.ID, &as.Title, &as.Published, &as.Comment); err != nil {
			rows.Close()
			return nil, err
		}
		if len(list) == 0 || list[len(list)-1].ID != c.ID {
			list = append(list, c)
		}
		last := &list[len(list)-1]
		at[as.ID] = [2]int{len(list) - 1, len(last.Assignments)}
		last.Assignments = append(last.Assignments, as)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = a.db.QueryContext(ctx, `SELECT assignment_id, id, name, size, uploaded_at FROM submission_files
		WHERE learner_id = $1 ORDER BY uploaded_at, id`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var assignmentID int64
		var f submissionFile
		if err := rows.Scan(&assignmentID, &f.ID, &f.Name, &f.Size, &f.UploadedAt); err != nil {
			return nil, err
		}
		if i, ok := at[assignmentID]; ok { // a file uploaded between the two queries waits for the next visit
			as := &list[i[0]].Assignments[i[1]]
			as.Files = append(as.Files, f)
		}
	}
	return list, rows.Err()
}
