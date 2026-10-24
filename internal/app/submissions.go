package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// A Submission's limits (spec #196). The total counts all of a Learner's Submissions and is separate from the Lab's
// Storage allowance.
const (
	maxSubmissionFile  = 20 << 20
	maxSubmissionFiles = 5
	maxSubmissionTotal = 200 << 20
)

// submissionLimits is the line under the upload field naming the limits, in lang.
func submissionLimits(lang string) string {
	return text.T(lang, "submission.limits", sizeText(maxSubmissionFile), maxSubmissionFiles, sizeText(maxSubmissionTotal))
}

const (
	zipMagic = "PK\x03\x04"
	oleMagic = "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1" // the old Word, Excel and PowerPoint formats
)

// submissionKinds maps each accepted extension to the bytes its content starts with and the type it is served as,
// so a file is accepted by what it holds, not only by its name.
var submissionKinds = map[string]struct{ magic, contentType string }{
	".pdf":  {"%PDF-", "application/pdf"},
	".doc":  {oleMagic, "application/msword"},
	".docx": {zipMagic, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	".xls":  {oleMagic, "application/vnd.ms-excel"},
	".xlsx": {zipMagic, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	".ppt":  {oleMagic, "application/vnd.ms-powerpoint"},
	".pptx": {zipMagic, "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	".png":  {"\x89PNG\r\n\x1a\n", "image/png"},
	".jpg":  {"\xff\xd8\xff", "image/jpeg"},
	".jpeg": {"\xff\xd8\xff", "image/jpeg"},
	".zip":  {zipMagic, "application/zip"},
}

// submissionFile is one file of a Learner's Submission.
type submissionFile struct {
	ID         int64
	Name       string
	Size       int64
	UploadedAt time.Time
}

// submissionFiles lists a Learner's files in an Assignment, oldest first.
func (a *App) submissionFiles(ctx context.Context, assignmentID, learnerID int64) ([]submissionFile, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, size, uploaded_at FROM submission_files
		WHERE assignment_id = $1 AND learner_id = $2 ORDER BY uploaded_at, id`, assignmentID, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []submissionFile
	for rows.Next() {
		var f submissionFile
		if err := rows.Scan(&f.ID, &f.Name, &f.Size, &f.UploadedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// refuseNotEnrolled is the answer to changing a Submission without an Enrollment, such as after leaving the Course.
func (a *App) refuseNotEnrolled(w http.ResponseWriter, r *http.Request, c course) {
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusForbidden, "members_only", page{Course: &c, Message: text.T(lang, "submission.join-to-change")})
}

// uploadSubmission adds the enrolled Learner's uploaded files to their Submission. Every file is checked first;
// when one is refused, none is kept.
func (a *App) uploadSubmission(w http.ResponseWriter, r *http.Request) {
	p, ok := a.learnerAssignmentFor(w, r)
	if !ok {
		return
	}
	if !p.Enrolled {
		a.refuseNotEnrolled(w, r, p.Course)
		return
	}
	// Before the body is read: a second upload from the same Learner is refused while the first is in flight.
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, busy := a.uploading.LoadOrStore(learner.ID, true); busy {
		a.tooManyAttempts(w, r)
		return
	}
	defer a.uploading.Delete(learner.ID)
	lang, err := a.language(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	refuse := func(msg string) {
		p.Error = msg
		a.renderAssignment(w, r, http.StatusUnprocessableEntity, &p)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSubmissionFiles*maxSubmissionFile+1<<20)
	err = r.ParseMultipartForm(1 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	var over *http.MaxBytesError
	if errors.As(err, &over) {
		refuse(text.T(lang, "submission.err.request-too-big", maxSubmissionFiles, sizeText(maxSubmissionFile)))
		return
	}
	var headers []*multipart.FileHeader
	if err == nil {
		headers = r.MultipartForm.File["file"]
	}
	if len(headers) == 0 {
		refuse(text.T(lang, "submission.err.choose"))
		return
	}
	var files []checkedFile
	var size int64
	for _, h := range headers {
		f, msg, err := checkSubmissionFile(h, lang)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if msg != "" {
			refuse(msg)
			return
		}
		files = append(files, f)
		size += int64(len(f.data))
	}
	learnerID := p.Learner.ID
	// A cheap check first, so an upload that cannot fit is refused without touching the store; the locked check
	// below stays the authority.
	if msg, err := a.submissionRoom(r.Context(), a.db, lang, p.Assignment.ID, learnerID, len(files), size); err != nil || msg != "" {
		if err != nil {
			a.serverError(w, r, err)
		} else {
			refuse(msg)
		}
		return
	}
	var keys []string
	dropStored := func() {
		for _, key := range keys {
			if err := a.datasets.Delete(context.WithoutCancel(r.Context()), key); err != nil {
				a.logError(r, fmt.Errorf("delete refused Submission file %s: %w", key, err))
			}
		}
	}
	for _, f := range files {
		key, err := submissionKey(learnerID)
		if err == nil {
			err = a.datasets.Put(r.Context(), key, f.data)
		}
		if err != nil {
			dropStored()
			a.serverError(w, r, err)
			return
		}
		keys = append(keys, key)
	}
	msg, err := a.saveSubmissionFiles(r.Context(), lang, p.Assignment.ID, learnerID, files, keys, size)
	if err != nil || msg != "" {
		dropStored()
		if err != nil {
			a.serverError(w, r, err)
		} else {
			refuse(msg)
		}
		return
	}
	http.Redirect(w, r, p.path(), http.StatusSeeOther)
}

// checkedFile is an uploaded file that passed checkSubmissionFile, read whole: at most maxSubmissionFile bytes.
type checkedFile struct {
	name, contentType string
	data              []byte
}

// checkSubmissionFile reads one uploaded file and checks its name, size and content. msg is the reason it is
// refused, in lang, empty when it is accepted.
func checkSubmissionFile(h *multipart.FileHeader, lang string) (f checkedFile, msg string, err error) {
	f.name = cleanFileName(h.Filename)
	kind, ok := submissionKinds[strings.ToLower(filepath.Ext(f.name))]
	if !ok {
		return f, text.T(lang, "submission.err.type", f.name), nil
	}
	if h.Size > maxSubmissionFile {
		return f, text.T(lang, "submission.err.too-big", f.name, sizeText(maxSubmissionFile)), nil
	}
	file, err := h.Open()
	if err != nil {
		return f, "", err
	}
	defer file.Close()
	if f.data, err = io.ReadAll(io.LimitReader(file, maxSubmissionFile+1)); err != nil {
		return f, "", err
	}
	switch {
	case len(f.data) > maxSubmissionFile:
		return f, text.T(lang, "submission.err.too-big", f.name, sizeText(maxSubmissionFile)), nil
	case len(f.data) == 0 || !strings.HasPrefix(string(f.data[:min(len(f.data), 8)]), kind.magic):
		return f, text.T(lang, "submission.err.type", f.name), nil
	}
	f.contentType = kind.contentType
	return f, "", nil
}

// cleanFileName keeps an uploaded file's own name, without any folder or control characters, up to 200 characters.
func cleanFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if runes := []rune(name); len(runes) > 200 {
		ext := filepath.Ext(name)
		name = string(runes[:200-len([]rune(ext))]) + ext
	}
	if name == "" || name == "." || name == "/" {
		return "file"
	}
	return name
}

// submissionKey is a new storage key for one of the Learner's files; the prefix finds a Learner's files in the store.
func submissionKey(learnerID int64) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "submission-" + strconv.FormatInt(learnerID, 10) + "-" + hex.EncodeToString(b[:]), nil
}

// submissionRoom checks that n more files of size bytes in all fit the Learner's Submission to the Assignment and
// their total. msg is the reason they do not, in lang, empty when they fit.
func (a *App) submissionRoom(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, lang string, assignmentID, learnerID int64, n int, size int64) (string, error) {
	var count int
	var used int64
	if err := q.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE assignment_id = $1), COALESCE(sum(size), 0)
		FROM submission_files WHERE learner_id = $2`, assignmentID, learnerID).Scan(&count, &used); err != nil {
		return "", err
	}
	switch {
	case count+n > maxSubmissionFiles:
		return text.T(lang, "submission.err.count", maxSubmissionFiles), nil
	case used+size > maxSubmissionTotal:
		return text.T(lang, "submission.err.total", sizeText(maxSubmissionTotal), sizeText(used)), nil
	}
	return "", nil
}

// saveSubmissionFiles records the stored files, under keys, in a transaction holding the Learner's row, after
// checking again that they fit, so two uploads at once cannot both slip past the limits. msg is why they do not fit.
func (a *App) saveSubmissionFiles(ctx context.Context, lang string, assignmentID, learnerID int64, files []checkedFile, keys []string, size int64) (string, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners WHERE id = $1 FOR UPDATE`, learnerID); err != nil {
		return "", err
	}
	if msg, err := a.submissionRoom(ctx, tx, lang, assignmentID, learnerID, len(files), size); err != nil || msg != "" {
		return msg, err
	}
	for i, f := range files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO submission_files (assignment_id, learner_id, name, size, content_type, storage_key)
			VALUES ($1, $2, $3, $4, $5, $6)`, assignmentID, learnerID, f.name, len(f.data), f.contentType, keys[i]); err != nil {
			return "", err
		}
	}
	return "", tx.Commit()
}

// storedSubmissionFile is a file's record as its download and delete need it.
type storedSubmissionFile struct {
	LearnerID, AssignmentID, CourseID int64
	Name, ContentType, Key            string
	Published                         bool // its Assignment's
}

// submissionFileByPath loads the file named by {fid} when it is the logged-in Learner's, or any file for the Admin.
// Otherwise it writes the Not found page, so another Learner's files are not revealed.
func (a *App) submissionFileByPath(w http.ResponseWriter, r *http.Request, anyForAdmin bool) (storedSubmissionFile, *learner, bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return storedSubmissionFile{}, nil, false
	}
	id, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return storedSubmissionFile{}, nil, false
	}
	var f storedSubmissionFile
	err = a.db.QueryRowContext(r.Context(), `SELECT f.learner_id, f.assignment_id, a.course_id, f.name, f.content_type, f.storage_key, a.published
		FROM submission_files f JOIN assignments a ON a.id = f.assignment_id WHERE f.id = $1`, id).
		Scan(&f.LearnerID, &f.AssignmentID, &f.CourseID, &f.Name, &f.ContentType, &f.Key, &f.Published)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && f.LearnerID != learner.ID && !(anyForAdmin && learner.IsAdmin)) {
		a.notFound(w, r)
		return storedSubmissionFile{}, nil, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return storedSubmissionFile{}, nil, false
	}
	return f, learner, true
}

// downloadSubmissionFile sends a Submission file to its Learner, even after they left the Course, or to the Admin.
func (a *App) downloadSubmissionFile(w http.ResponseWriter, r *http.Request) {
	f, _, ok := a.submissionFileByPath(w, r, true)
	if !ok {
		return
	}
	data, err := a.datasets.Get(r.Context(), f.Key)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(data)
}

// deleteSubmissionFile deletes one of the enrolled Learner's files from their Submission and from the store.
func (a *App) deleteSubmissionFile(w http.ResponseWriter, r *http.Request) {
	f, learner, ok := a.submissionFileByPath(w, r, false)
	if !ok {
		return
	}
	c, err := a.courseByID(r.Context(), f.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	enrolled, err := a.enrolled(r.Context(), learner.ID, f.CourseID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !f.Published {
		a.notFound(w, r)
		return
	}
	if !enrolled {
		a.refuseNotEnrolled(w, r, c)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM submission_files WHERE storage_key = $1`, f.Key); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropDeletedFiles(r.Context(), r)
	http.Redirect(w, r, assignmentPath(f.CourseID, f.AssignmentID), http.StatusSeeOther)
}

// dropDeletedFiles deletes from the store every Submission file and Work Dataset whose record is gone, and returns
// how many. A file the store fails to delete stays queued for the next run.
func (a *App) dropDeletedFiles(ctx context.Context, r *http.Request) int {
	ctx = context.WithoutCancel(ctx)
	rows, err := a.db.QueryContext(ctx, `SELECT storage_key FROM stored_file_deletions`)
	if err != nil {
		a.logError(r, fmt.Errorf("list deleted stored files: %w", err))
		return 0
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			a.logError(r, err)
			return 0
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		a.logError(r, err)
		return 0
	}
	n := 0
	for _, key := range keys {
		if err := a.datasets.Delete(ctx, key); err != nil {
			a.logError(r, fmt.Errorf("delete stored file %s: %w", key, err))
			continue
		}
		if _, err := a.db.ExecContext(ctx, `DELETE FROM stored_file_deletions WHERE storage_key = $1`, key); err != nil {
			a.logError(r, err)
			continue
		}
		n++
	}
	return n
}
