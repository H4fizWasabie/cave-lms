package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yuin/goldmark"
)

// lesson is one step of a Course: one Gallery item (GalleryItemID set) or text (Body set).
// Draft Lessons (Published false) are shown to the Admin only.
type lesson struct {
	ID            int64
	CourseID      int64
	Position      int
	Title         string
	Objective     string
	Body          string
	GalleryItemID int64
	ItemTitle     string
	Published     bool
	Completed     bool          // set on the Course page for the enrolled Learner
	HTML          template.HTML // the Body rendered as sanitised Markdown, for the Lesson page
}

const lessonColumns = `l.id, l.course_id, l.position, l.title, l.objective, l.body, COALESCE(l.gallery_item_id, 0), COALESCE(g.title, ''), l.published`

// lessonsOf lists a Course's Lessons in order. Learners get only the published ones.
func (a *App) lessonsOf(ctx context.Context, courseID int64, includeDrafts bool) ([]lesson, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+lessonColumns+` FROM lessons l
		LEFT JOIN gallery_items g ON g.id = l.gallery_item_id
		WHERE l.course_id = $1 AND (l.published OR $2) ORDER BY l.position, l.id`, courseID, includeDrafts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []lesson
	for rows.Next() {
		var l lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.Position, &l.Title, &l.Objective, &l.Body, &l.GalleryItemID, &l.ItemTitle, &l.Published); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

// adminLessons lists every Lesson of a Course, drafts included, for the Admin to arrange.
func (a *App) adminLessons(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	list, err := a.lessonsOf(r.Context(), c.ID, true)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "admin_lessons", page{Course: &c, Lessons: list})
}

func (a *App) newLessonForm(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.renderLessonForm(w, r, http.StatusOK, page{Course: &c, Lesson: &lesson{CourseID: c.ID}})
}

// renderLessonForm fills in the Gallery items the Lesson form can pick from.
func (a *App) renderLessonForm(w http.ResponseWriter, r *http.Request, status int, data page) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	items, err := a.galleryItemsFor(r.Context(), learner)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data.Items = items
	a.render(w, r, status, "lesson_form", data)
}

// lessonFromForm reads the Lesson form for the given kind ("item" or "text"). msg is the first problem, in lang, empty when valid.
func lessonFromForm(r *http.Request, lang, kind string) (l lesson, msg string) {
	l.Title = strings.TrimSpace(r.PostFormValue("title"))
	l.Objective = strings.TrimSpace(r.PostFormValue("objective"))
	l.Body = strings.TrimSpace(r.PostFormValue("body"))
	switch kind {
	case "item":
		id, err := strconv.ParseInt(r.PostFormValue("gallery_item_id"), 10, 64)
		if err != nil || id <= 0 {
			msg = text.T(lang, "adm.lessons.err.item")
		}
		l.GalleryItemID = id
		l.Body = ""
	case "text":
		if l.Body == "" {
			msg = text.T(lang, "adm.lessons.err.text")
		}
	default:
		msg = text.T(lang, "adm.lessons.err.kind")
	}
	switch {
	case msg != "":
	case l.Title == "" || utf8.RuneCountInString(l.Title) > 200:
		msg = text.T(lang, "adm.err.title-200")
	case utf8.RuneCountInString(l.Objective) > 200 || strings.ContainsAny(l.Objective, "\r\n"):
		msg = text.T(lang, "adm.lessons.err.objective")
	case utf8.RuneCountInString(l.Body) > 20000:
		msg = text.T(lang, "adm.lessons.err.body")
	}
	return l, msg
}

func (a *App) createLesson(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	l, msg := lessonFromForm(r, lang, r.PostFormValue("kind"))
	l.CourseID = c.ID
	l.Published = r.PostFormValue("draft") != "on"
	if msg != "" {
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `
		INSERT INTO lessons (course_id, position, title, objective, body, gallery_item_id, published)
		VALUES ($1, (SELECT COALESCE(max(position), 0) + 1 FROM lessons WHERE course_id = $1), $2, $3, $4, $5, $6)`,
		c.ID, l.Title, l.Objective, l.Body, nullID(l.GalleryItemID), l.Published)
	if isForeignKeyError(err) {
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: text.T(lang, "adm.lessons.err.item")})
		return
	}
	if err == nil && l.Published {
		err = a.defaultMembersOnly(r.Context(), l.GalleryItemID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, lessonsPath(c.ID), http.StatusSeeOther)
}

// nullID maps the zero ID to SQL NULL, which is how a text Lesson has no Gallery item.
func nullID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}

// isForeignKeyError reports whether err is a violated foreign key (23503).
func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// renderMarkdown turns Lesson text into HTML. Raw HTML and dangerous link targets are dropped, never passed through.
func renderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

// lessonByPath loads the Lesson named by the {lessonID} path value. When there is none it writes the Not found page and returns false.
func (a *App) lessonByPath(w http.ResponseWriter, r *http.Request) (lesson, bool) {
	id, err := strconv.ParseInt(r.PathValue("lessonID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return lesson{}, false
	}
	var l lesson
	err = a.db.QueryRowContext(r.Context(), `SELECT `+lessonColumns+` FROM lessons l
		LEFT JOIN gallery_items g ON g.id = l.gallery_item_id WHERE l.id = $1`, id).
		Scan(&l.ID, &l.CourseID, &l.Position, &l.Title, &l.Objective, &l.Body, &l.GalleryItemID, &l.ItemTitle, &l.Published)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return lesson{}, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return lesson{}, false
	}
	return l, true
}

// moveLesson returns the handler that swaps a Lesson with its neighbour: dir -1 is up, +1 is down.
// At the top or bottom it changes nothing.
func (a *App) moveLesson(dir int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, ok := a.lessonByPath(w, r)
		if !ok {
			return
		}
		ctx := r.Context()
		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		defer tx.Rollback()
		op, order := "<", "DESC"
		if dir > 0 {
			op, order = ">", "ASC"
		}
		var otherID int64
		var otherPos int
		err = tx.QueryRowContext(ctx, `SELECT id, position FROM lessons
			WHERE course_id = $1 AND position `+op+` $2 ORDER BY position `+order+`, id `+order+` LIMIT 1`,
			l.CourseID, l.Position).Scan(&otherID, &otherPos)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.serverError(w, r, err)
			return
		}
		if err == nil {
			if _, err := tx.ExecContext(ctx, `UPDATE lessons SET position = CASE WHEN id = $1 THEN $2::integer ELSE $3::integer END WHERE id IN ($1, $4)`,
				l.ID, otherPos, l.Position, otherID); err != nil {
				a.serverError(w, r, err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
	}
}

func lessonsPath(courseID int64) string {
	return "/admin/courses/" + strconv.FormatInt(courseID, 10) + "/lessons"
}

// showLesson opens one Lesson for an enrolled Learner or the Admin. Other Learners are asked to join.
func (a *App) showLesson(w http.ResponseWriter, r *http.Request) {
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
	if !enrolled && !learner.IsAdmin {
		a.render(w, r, http.StatusForbidden, "members_only", page{Learner: learner, Course: &c,
			Message: "Join this Course to open its Lessons."})
		return
	}
	l, ok := a.lessonByPath(w, r)
	if !ok || l.CourseID != c.ID {
		if ok {
			http.NotFound(w, r)
		}
		return
	}
	if !l.Published && !learner.IsAdmin {
		http.NotFound(w, r)
		return
	}
	data := page{Learner: learner, Course: &c, Lesson: &l}
	if l.GalleryItemID != 0 {
		item, err := a.loadGalleryItem(r.Context(), l.GalleryItemID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		allowed, err := a.mayView(r.Context(), learner, item)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if !allowed {
			a.refuseMembersItem(w, r, item)
			return
		}
		data.Item = &item
	} else {
		l.HTML = renderMarkdown(l.Body)
	}
	a.render(w, r, http.StatusOK, "lesson", data)
}

func (a *App) editLessonForm(w http.ResponseWriter, r *http.Request) {
	l, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	c, err := a.courseByID(r.Context(), l.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderLessonForm(w, r, http.StatusOK, page{Course: &c, Lesson: &l})
}

// updateLesson saves the Admin's edits. A Lesson keeps its kind: an item stays an item, text stays text.
func (a *App) updateLesson(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	kind := "text"
	if existing.GalleryItemID != 0 {
		kind = "item"
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	l, msg := lessonFromForm(r, lang, kind)
	l.ID, l.CourseID, l.Position, l.Published = existing.ID, existing.CourseID, existing.Position, existing.Published
	c, err := a.courseByID(r.Context(), l.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if msg != "" {
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE lessons SET title = $2, objective = $3, body = $4, gallery_item_id = $5 WHERE id = $1`,
		l.ID, l.Title, l.Objective, l.Body, nullID(l.GalleryItemID))
	if isForeignKeyError(err) {
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: text.T(lang, "adm.lessons.err.item")})
		return
	}
	if err == nil && l.Published {
		err = a.defaultMembersOnly(r.Context(), l.GalleryItemID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
}

func (a *App) deleteLesson(w http.ResponseWriter, r *http.Request) {
	l, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM lessons WHERE id = $1`, l.ID)
	if err == nil {
		err = a.issueEarnedCertificates(r.Context(), l.CourseID)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
}

// setLessonPublished returns the handler that publishes a Lesson to Learners or turns it back into a Draft.
func (a *App) setLessonPublished(published bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, ok := a.lessonByPath(w, r)
		if !ok {
			return
		}
		_, err := a.db.ExecContext(r.Context(), `UPDATE lessons SET published = $2 WHERE id = $1`, l.ID, published)
		if err == nil && published {
			err = a.defaultMembersOnly(r.Context(), l.GalleryItemID)
		}
		if err == nil && !published {
			err = a.issueEarnedCertificates(r.Context(), l.CourseID)
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
	}
}

// publishAllLessons publishes every Draft Lesson of the Course at once.
func (a *App) publishAllLessons(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`UPDATE lessons SET published = true WHERE course_id = $1 AND NOT published RETURNING COALESCE(gallery_item_id, 0)`, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var items []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			a.serverError(w, r, err)
			return
		}
		items = append(items, id)
	}
	if err := rows.Close(); err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := rows.Err(); err != nil {
		a.serverError(w, r, err)
		return
	}
	for _, id := range items {
		if err := a.defaultMembersOnly(r.Context(), id); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, lessonsPath(c.ID), http.StatusSeeOther)
}

// defaultMembersOnly makes a video or ebook placed in a published Lesson Course members only, unless the Admin has
// already chosen another Visibility or the item is promotional. Pictures keep their Visibility. Callers run it when
// the Lesson is saved published or a Draft is published.
func (a *App) defaultMembersOnly(ctx context.Context, itemID int64) error {
	if itemID == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx, `UPDATE gallery_items SET visibility = 'course_members'
		WHERE id = $1 AND visibility = 'everyone' AND NOT promotional AND format IN ('video', 'ebook')`, itemID)
	return err
}
