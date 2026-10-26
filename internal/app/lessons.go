package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"html/template"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// lesson is one step of a Course: text (Body set), a Gallery item (GalleryItemID set), or both; or text with a
// Lesson image (ImageKey set) instead of the Gallery item. Draft Lessons (Published false) are shown to the Admin only.
type lesson struct {
	ID               int64
	CourseID         int64
	Position         int
	Title            string
	Objective        string
	Body             string
	GalleryItemID    int64
	ItemTitle        string
	ImageKey         string // the Lesson image's storage key, "" when there is none
	imageType        string
	ImageDescription string
	Published        bool
	Completed        bool          // set on the Course page for the enrolled Learner
	HTML             template.HTML // the Body rendered as sanitised Markdown, for the Lesson page
}

// Objectives lists the Lesson's Objectives, stored one per line.
func (l lesson) Objectives() []string {
	if l.Objective == "" {
		return nil
	}
	return strings.Split(l.Objective, "\n")
}

const lessonColumns = `l.id, l.course_id, l.position, l.title, l.objective, l.body, COALESCE(l.gallery_item_id, 0), COALESCE(g.title, ''), l.image_key, l.image_type, l.image_description, l.published`

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
		if err := rows.Scan(&l.ID, &l.CourseID, &l.Position, &l.Title, &l.Objective, &l.Body, &l.GalleryItemID, &l.ItemTitle, &l.ImageKey, &l.imageType, &l.ImageDescription, &l.Published); err != nil {
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
	a.renderLessonForm(w, r, http.StatusOK, page{Course: &c, Lesson: &lesson{CourseID: c.ID}}) // Keep as Draft starts ticked
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

// lessonFromForm reads the Lesson form: text, a Gallery item, or both. msg is the first problem, in lang, empty when valid.
func lessonFromForm(r *http.Request, lang string) (l lesson, msg string) {
	l.Title = strings.TrimSpace(r.PostFormValue("title"))
	l.Body = strings.TrimSpace(r.PostFormValue("body"))
	l.ImageDescription = strings.TrimSpace(r.PostFormValue("image_description"))
	objectives, ok := objectivesFromForm(r.PostFormValue("objective"))
	l.Objective = strings.Join(objectives, "\n")
	if v := strings.TrimSpace(r.PostFormValue("gallery_item_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return l, text.T(lang, "adm.lessons.err.item")
		}
		l.GalleryItemID = id
	}
	switch {
	case l.Body == "" && l.GalleryItemID == 0:
		msg = text.T(lang, "adm.lessons.err.content")
	case l.Title == "" || utf8.RuneCountInString(l.Title) > 200:
		msg = text.T(lang, "adm.err.title-200")
	case !ok:
		msg = text.T(lang, "adm.lessons.err.objective")
	case utf8.RuneCountInString(l.Body) > aiwriter.MaxLessonText:
		msg = text.T(lang, "adm.lessons.err.body")
	}
	return l, msg
}

// objectivesFromForm splits the Objectives box into its non-blank lines. ok is false for more than five lines or a
// line over 200 characters; none is valid.
func objectivesFromForm(v string) (lines []string, ok bool) {
	for _, line := range strings.Split(strings.ReplaceAll(v, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
			if utf8.RuneCountInString(line) > 200 {
				return lines, false
			}
		}
	}
	return lines, len(lines) <= 5
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
	if status, key := readLessonForm(w, r); status != 0 {
		a.renderLessonForm(w, r, status, page{Course: &c, Lesson: &lesson{CourseID: c.ID}, Error: text.T(lang, key)})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	l, msg := lessonFromForm(r, lang)
	l.CourseID = c.ID
	l.Published = r.PostFormValue("draft") != "on"
	if m := lessonImageProblem(r, lang, &l); m != "" {
		msg = m
	}
	if msg == "" {
		if msg, err = a.takeLessonImage(r, lang, &l); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if msg != "" {
		l.ImageKey = ""
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `
		INSERT INTO lessons (course_id, position, title, objective, body, gallery_item_id, image_key, image_type, image_description, published)
		VALUES ($1, (SELECT COALESCE(max(position), 0) + 1 FROM lessons WHERE course_id = $1), $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.ID, l.Title, l.Objective, l.Body, nullID(l.GalleryItemID), l.ImageKey, l.imageType, l.ImageDescription, l.Published)
	if err != nil {
		a.dropPicture(r, l.ImageKey)
		l.ImageKey = ""
	}
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

// generateNewLesson has the AI write the text of the Lesson on the new Lesson form, saving nothing. The form comes
// back with Keep as Draft ticked, so Learners never see AI-written text the Admin has not published.
func (a *App) generateNewLesson(w http.ResponseWriter, r *http.Request) {
	c, ok := a.courseFor(w, r)
	if !ok {
		return
	}
	a.generateLesson(w, r, c, lesson{CourseID: c.ID})
}

// generateLessonText has the AI write the text of the Lesson on its edit form, saving nothing.
func (a *App) generateLessonText(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	c, err := a.courseByID(r.Context(), existing.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.generateLesson(w, r, c, existing)
}

// generateLesson re-renders the Lesson form as posted with the AI's text in the Text box, in the posted language
// (the admin screen's when none or an unknown one is posted), written from the Course title, the Lesson's title and
// Objectives and the Course's other Lessons' titles. A missing title or Objective asks no AI.
func (a *App) generateLesson(w http.ResponseWriter, r *http.Request, c course, existing lesson) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if status, key := readLessonForm(w, r); status != 0 {
		a.renderLessonForm(w, r, status, page{Course: &c, Lesson: &existing, Error: text.T(lang, key)})
		return
	}
	data := page{Course: &c, AILanguage: r.PostFormValue("language")}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
		// A chosen file cannot come back in the re-rendered form.
		if len(r.MultipartForm.File["picture"]) > 0 {
			data.Message = text.T(lang, "adm.lessons.image-again")
		}
	}
	l, _ := lessonFromForm(r, lang)
	l.ID, l.CourseID, l.Position, l.Published = existing.ID, existing.CourseID, existing.Position, existing.Published
	l.ImageKey, l.imageType = existing.ImageKey, existing.imageType
	data.Lesson = &l
	if !text.Supported(data.AILanguage) {
		data.AILanguage = lang
	}
	objectives, ok := objectivesFromForm(r.PostFormValue("objective"))
	switch {
	case l.Title == "" || utf8.RuneCountInString(l.Title) > 200:
		data.Error = text.T(lang, "adm.err.title-200")
	case !ok:
		data.Error = text.T(lang, "adm.lessons.err.objective")
	case len(objectives) == 0:
		data.Error = text.T(lang, "adm.lessons.gen.err.objective")
	}
	if data.Error != "" {
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, data)
		return
	}
	others, err := a.lessonsOf(r.Context(), c.ID, true)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var titles []string
	for _, o := range others {
		if o.ID != l.ID {
			titles = append(titles, o.Title)
		}
	}
	body, err := aiwriter.Lesson(r.Context(), a.writer, aiwriter.LessonRequest{CourseTitle: c.Title, Title: l.Title,
		Objectives: objectives, OtherTitles: titles, Language: data.AILanguage})
	if msg := aiProblem(err); msg != "" {
		a.logError(r, err)
		data.Error = text.T(lang, "adm.lessons."+msg)
		a.renderLessonForm(w, r, http.StatusServiceUnavailable, data)
		return
	}
	l.Body = strings.TrimSpace(body)
	a.renderLessonForm(w, r, http.StatusOK, data)
}

// maxLessonImage caps a Lesson image, well below the picture form's limit.
const maxLessonImage = 5 << 20

// readLessonForm is readPictureForm for the Lesson form, whose too-large message names the Lesson image's limit.
func readLessonForm(w http.ResponseWriter, r *http.Request) (status int, msgKey string) {
	status, msgKey = readPictureForm(w, r)
	if status == http.StatusRequestEntityTooLarge {
		msgKey = "adm.lessons.err.image-size"
	}
	return status, msgKey
}

// lessonImageProblem applies the Lesson form's remove_image to l and checks the Lesson image it would then have: the
// one sent, else the one l keeps. msg is the first problem, in lang, empty when there is none. Without an image the
// description is dropped.
func lessonImageProblem(r *http.Request, lang string, l *lesson) (msg string) {
	if r.PostFormValue("remove_image") != "" {
		l.ImageKey, l.imageType = "", ""
	}
	var sent []*multipart.FileHeader
	if r.MultipartForm != nil {
		sent = r.MultipartForm.File["picture"]
	}
	switch {
	case len(sent) == 0 && l.ImageKey == "":
		l.ImageDescription = ""
		return ""
	case len(sent) > 0 && sent[0].Size > maxLessonImage:
		return text.T(lang, "adm.lessons.err.image-size")
	case l.ImageDescription == "" || utf8.RuneCountInString(l.ImageDescription) > 200:
		return text.T(lang, "adm.lessons.err.image-description")
	case l.GalleryItemID != 0:
		return text.T(lang, "adm.lessons.err.image-and-item")
	case l.Body == "":
		return text.T(lang, "adm.lessons.err.image-text")
	}
	return ""
}

// takeLessonImage stores the Lesson form's picture, if one was sent, as l's Lesson image. msg is the problem, in lang,
// when the file is not a picture. The caller drops the image it replaces once the Lesson is saved.
func (a *App) takeLessonImage(r *http.Request, lang string, l *lesson) (msg string, err error) {
	key, mimeType, bad, err := a.storePicture(r, "lesson-")
	switch {
	case err != nil:
		return "", err
	case bad:
		return text.T(lang, "picture.err.type"), nil
	case key != "":
		l.ImageKey, l.imageType = key, mimeType
	}
	return "", nil
}

// lessonImage serves a Lesson image by its storage key, by the Lesson page's rule: to the Admin, or to a Learner
// enrolled in its Course when the Lesson is published. Gallery files are not served here.
func (a *App) lessonImage(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var courseID int64
	var published bool
	var contentType string
	err = a.db.QueryRowContext(r.Context(), `SELECT course_id, published, image_type FROM lessons WHERE image_key = $1 AND image_key <> ''`,
		r.PathValue("key")).Scan(&courseID, &published, &contentType)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !learner.IsAdmin {
		enrolled, err := a.enrolled(r.Context(), learner.ID, courseID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if !enrolled || !published {
			a.notFound(w, r)
			return
		}
	}
	a.serveStored(w, r, r.PathValue("key"), contentType)
}

// nullID maps the zero ID to SQL NULL, which is how a text Lesson has no Gallery item.
func nullID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}

// confirmed reports whether the post came from a confirmation page, which sends confirm=yes with its button. A
// destructive action posted without it does nothing.
func confirmed(r *http.Request) bool { return r.PostFormValue("confirm") == "yes" }

// isForeignKeyError reports whether err is a violated foreign key (23503).
func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// lessonMarkdown is CommonMark plus GitHub-style tables, aligned with the align attribute (the CSP keeps inline styles
// for view transitions only).
var lessonMarkdown = goldmark.New(goldmark.WithExtensions(
	extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute))))

// renderMarkdown turns Lesson text into HTML. Raw HTML and dangerous link targets are dropped, never passed through.
// A table is wrapped in .table-wrap so a wide one scrolls inside its own box; raw HTML is escaped, so every "<table>"
// in the output is the renderer's own.
func renderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := lessonMarkdown.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	html := strings.ReplaceAll(buf.String(), "<table>", `<div class="table-wrap"><table>`)
	return template.HTML(strings.ReplaceAll(html, "</table>", "</table></div>"))
}

// lessonByPath loads the Lesson named by the {lessonID} path value. When there is none it writes the Not found page and returns false.
func (a *App) lessonByPath(w http.ResponseWriter, r *http.Request) (lesson, bool) {
	id, err := strconv.ParseInt(r.PathValue("lessonID"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return lesson{}, false
	}
	var l lesson
	err = a.db.QueryRowContext(r.Context(), `SELECT `+lessonColumns+` FROM lessons l
		LEFT JOIN gallery_items g ON g.id = l.gallery_item_id WHERE l.id = $1`, id).
		Scan(&l.ID, &l.CourseID, &l.Position, &l.Title, &l.Objective, &l.Body, &l.GalleryItemID, &l.ItemTitle, &l.ImageKey, &l.imageType, &l.ImageDescription, &l.Published)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
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
		if err := a.swapWithNeighbour(r.Context(), "lessons", l.ID, l.CourseID, l.Position, dir); err != nil {
			a.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
	}
}

// swapWithNeighbour swaps the positions of row id and its neighbour in the same Course, in table (a constant, never
// user input): dir -1 is up, +1 is down. At the top or bottom it changes nothing.
func (a *App) swapWithNeighbour(ctx context.Context, table string, id, courseID int64, position, dir int) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	op, order := "<", "DESC"
	if dir > 0 {
		op, order = ">", "ASC"
	}
	var otherID int64
	var otherPos int
	err = tx.QueryRowContext(ctx, `SELECT id, position FROM `+table+`
		WHERE course_id = $1 AND position `+op+` $2 ORDER BY position `+order+`, id `+order+` LIMIT 1`,
		courseID, position).Scan(&otherID, &otherPos)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET position = CASE WHEN id = $1 THEN $2::integer ELSE $3::integer END WHERE id IN ($1, $4)`,
		id, otherPos, position, otherID); err != nil {
		return err
	}
	return tx.Commit()
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
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusForbidden, "members_only", page{Learner: learner, Course: &c,
			Message: text.T(lang, "lesson.join-to-open")})
		return
	}
	l, ok := a.lessonByPath(w, r)
	if !ok || l.CourseID != c.ID {
		if ok {
			a.notFound(w, r)
		}
		return
	}
	if !l.Published && !learner.IsAdmin {
		a.notFound(w, r)
		return
	}
	place, err := a.lessonPlaceOf(r.Context(), c.ID, l.ID, learner.IsAdmin)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := page{Learner: learner, Course: &c, Lesson: &l, Place: &place, Enrolled: enrolled}
	if enrolled {
		done, err := a.completedLessons(r.Context(), learner.ID, c.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		l.Completed = done[l.ID]
		// A done Lesson's one way on is the pager's next Lesson; only the last Lesson offers the Course's next step,
		// which can point elsewhere (an earlier Lesson not done, the Exam).
		if l.Completed && place.Next == nil {
			step, err := a.nextStep(r.Context(), learner, c, nil)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			data.Step = &step
		}
	}
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
	}
	if l.Body != "" {
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

// updateLesson saves the Admin's edits, which may add or remove the Lesson's text or Gallery item.
func (a *App) updateLesson(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	c, err := a.courseByID(r.Context(), existing.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if status, key := readLessonForm(w, r); status != 0 {
		a.renderLessonForm(w, r, status, page{Course: &c, Lesson: &existing, Error: text.T(lang, key)})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	l, msg := lessonFromForm(r, lang)
	l.ID, l.CourseID, l.Position, l.Published = existing.ID, existing.CourseID, existing.Position, existing.Published
	l.ImageKey, l.imageType = existing.ImageKey, existing.imageType
	if m := lessonImageProblem(r, lang, &l); m != "" {
		msg = m
	}
	if msg == "" {
		if msg, err = a.takeLessonImage(r, lang, &l); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if msg != "" {
		l.ImageKey, l.imageType = existing.ImageKey, existing.imageType
		a.renderLessonForm(w, r, http.StatusUnprocessableEntity, page{Course: &c, Lesson: &l, Error: msg})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `UPDATE lessons SET title = $2, objective = $3, body = $4, gallery_item_id = $5,
		image_key = $6, image_type = $7, image_description = $8 WHERE id = $1`,
		l.ID, l.Title, l.Objective, l.Body, nullID(l.GalleryItemID), l.ImageKey, l.imageType, l.ImageDescription)
	if err != nil && l.ImageKey != existing.ImageKey {
		a.dropPicture(r, l.ImageKey)
		l.ImageKey, l.imageType = existing.ImageKey, existing.imageType
	}
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
	if existing.ImageKey != l.ImageKey {
		a.dropPicture(r, existing.ImageKey)
	}
	http.Redirect(w, r, lessonsPath(l.CourseID), http.StatusSeeOther)
}

// confirmDeleteLesson asks the Admin to confirm removing a Lesson.
func (a *App) confirmDeleteLesson(w http.ResponseWriter, r *http.Request) {
	l, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	c, err := a.courseByID(r.Context(), l.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "remove_lesson", page{Lesson: &l, Course: &c})
}

// deleteLesson removes a Lesson once its confirmation page is posted; a post without it goes to that page instead.
func (a *App) deleteLesson(w http.ResponseWriter, r *http.Request) {
	l, ok := a.lessonByPath(w, r)
	if !ok {
		return
	}
	if !confirmed(r) {
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM lessons WHERE id = $1`, l.ID)
	if err == nil {
		a.dropPicture(r, l.ImageKey)
		err = a.issueEarnedCertificates(r.Context(), 0, l.CourseID)
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
			err = a.issueEarnedCertificates(r.Context(), 0, l.CourseID)
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
