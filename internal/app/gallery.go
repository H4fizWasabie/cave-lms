package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	visibilityEveryone      = "everyone"
	visibilityLearners      = "learners"
	visibilityCourseMembers = "course_members"
)

// galleryItem is one row of gallery_items. storageKey is the file's name on disk.
type galleryItem struct {
	ID          int64
	Title       string
	Description string
	Category    string
	Format      string
	Promotional bool
	Visibility  string
	MimeType    string
	SizeBytes   int64
	storageKey  string
}

// allowedTypes maps the content type sniffed from the first 512 bytes to a Format.
var allowedTypes = map[string]string{
	"video/mp4":       "video",
	"application/pdf": "ebook",
	"image/jpeg":      "picture",
	"image/png":       "picture",
	"image/webp":      "picture",
}

// mp4Brands are ftyp major brands that http.DetectContentType misses in many real MP4 files.
var mp4Brands = map[string]bool{"isom": true, "iso2": true, "iso4": true, "iso5": true, "iso6": true, "mp41": true, "mp42": true, "avc1": true, "M4V ": true}

// sniffType returns the allowed content type of an upload from its first bytes.
func sniffType(head []byte) (string, bool) {
	ct := http.DetectContentType(head)
	if _, ok := allowedTypes[ct]; ok {
		return ct, true
	}
	if len(head) >= 12 && string(head[4:8]) == "ftyp" && mp4Brands[string(head[8:12])] {
		return "video/mp4", true
	}
	return "", false
}

// mayView is the single access decision: may viewer (nil for a Visitor) see this Gallery item?
// Every page and file download goes through it.
func mayView(viewer *learner, item galleryItem) bool {
	if viewer != nil && viewer.IsAdmin {
		return true
	}
	switch item.Visibility {
	case visibilityEveryone:
		return true
	case visibilityLearners:
		return viewer != nil
	}
	// ponytail: course_members needs Enrollments and Lessons (#9, #10); until then only Admins see these.
	return false
}

// galleryItemFor loads the item named by the {id} path value. When there is none it writes the Not found page and returns false.
func (a *App) galleryItemFor(w http.ResponseWriter, r *http.Request) (galleryItem, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return galleryItem{}, false
	}
	item, err := a.loadGalleryItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return galleryItem{}, false
	}
	if err != nil {
		serverError(w, err)
		return galleryItem{}, false
	}
	return item, true
}

// viewItem loads the item named in the path and applies mayView. When it returns false, the response is already written.
func (a *App) viewItem(w http.ResponseWriter, r *http.Request) (galleryItem, bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return galleryItem{}, false
	}
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return galleryItem{}, false
	}
	if !mayView(learner, item) {
		if learner == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return galleryItem{}, false
		}
		a.render(w, http.StatusForbidden, "message", page{
			Title:   "Members only",
			Message: "This item is only for members of its Courses.",
		})
		return galleryItem{}, false
	}
	return item, true
}

const galleryColumns = `id, title, description, category, format, promotional, visibility, mime_type, size_bytes, storage_key`

func scanGalleryItem(scan func(dest ...any) error) (galleryItem, error) {
	var item galleryItem
	err := scan(&item.ID, &item.Title, &item.Description, &item.Category, &item.Format, &item.Promotional, &item.Visibility, &item.MimeType, &item.SizeBytes, &item.storageKey)
	return item, err
}

func (a *App) loadGalleryItem(ctx context.Context, id int64) (galleryItem, error) {
	return scanGalleryItem(a.db.QueryRowContext(ctx, `SELECT `+galleryColumns+` FROM gallery_items WHERE id = $1`, id).Scan)
}

// galleryItemsFor lists the Gallery items the viewer may see, newest first. onlyPromotional narrows the list to promotional items.
func (a *App) galleryItemsFor(ctx context.Context, viewer *learner, onlyPromotional bool) ([]galleryItem, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+galleryColumns+` FROM gallery_items WHERE promotional OR NOT $1 ORDER BY created_at DESC, id DESC`, onlyPromotional)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []galleryItem
	for rows.Next() {
		item, err := scanGalleryItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		if mayView(viewer, item) {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

// gallery lists what the viewer may see, narrowed by the ?format and ?category filters.
func (a *App) gallery(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	items, err := a.galleryItemsFor(r.Context(), learner, false)
	if err != nil {
		serverError(w, err)
		return
	}
	format, category := r.URL.Query().Get("format"), r.URL.Query().Get("category")
	var shown []galleryItem
	seen := map[string]bool{}
	var categories []string // only categories of items the viewer may see
	for _, item := range items {
		if item.Category != "" && !seen[item.Category] {
			seen[item.Category] = true
			categories = append(categories, item.Category)
		}
		if (format == "" || item.Format == format) && (category == "" || item.Category == category) {
			shown = append(shown, item)
		}
	}
	sort.Strings(categories)
	a.render(w, http.StatusOK, "gallery", page{PublicBaseURL: a.publicBaseURL, Learner: learner, Items: shown,
		Format: format, Category: category, Categories: categories})
}

func (a *App) galleryItemPage(w http.ResponseWriter, r *http.Request) {
	item, ok := a.viewItem(w, r)
	if !ok {
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "gallery_item", page{PublicBaseURL: a.publicBaseURL, Learner: learner, Item: &item})
}

func (a *App) galleryFile(w http.ResponseWriter, r *http.Request) {
	item, ok := a.viewItem(w, r)
	if !ok {
		return
	}
	f, err := a.store.Open(item.storageKey)
	if err != nil {
		serverError(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if item.Format == "ebook" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="cave-gallery-%d.pdf"`, item.ID))
	}
	// ServeContent handles Range requests, so videos can be seeked.
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (a *App) galleryUploadForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "gallery_upload", page{Item: &galleryItem{Visibility: visibilityEveryone}})
}

// galleryUpload streams the file to disk as it arrives. Nothing is stored unless every field and the file are valid.
func (a *App) galleryUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		a.render(w, http.StatusBadRequest, "message", page{Title: "Upload not read", Message: "The upload form could not be read. Try again."})
		return
	}

	var (
		item      = galleryItem{Visibility: visibilityEveryone}
		stored    string // key of the file saved so far
		committed bool
	)
	defer func() {
		if stored != "" && !committed {
			_ = a.store.Remove(stored)
		}
	}()
	fail := func(status int, msg string) {
		a.render(w, status, "gallery_upload", page{Item: &item, Error: msg})
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			a.uploadReadFailed(w, err)
			return
		}
		if part.FormName() != "file" {
			value, err := io.ReadAll(io.LimitReader(part, 4096))
			if err != nil {
				a.uploadReadFailed(w, err)
				return
			}
			setGalleryField(&item, part.FormName(), strings.TrimSpace(string(value)))
			continue
		}
		if stored != "" {
			fail(http.StatusBadRequest, "Upload one file at a time.")
			return
		}
		head := make([]byte, 512)
		n, err := io.ReadFull(part, head)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			a.uploadReadFailed(w, err)
			return
		}
		head = head[:n]
		mimeType, ok := sniffType(head)
		if !ok {
			fail(http.StatusUnprocessableEntity, "Upload an MP4 video, a PDF ebook, or a JPEG, PNG or WebP picture.")
			return
		}
		key, size, err := a.store.Save(io.MultiReader(bytes.NewReader(head), part))
		if err != nil {
			a.uploadReadFailed(w, err)
			return
		}
		stored = key
		item.storageKey, item.MimeType, item.SizeBytes, item.Format = key, mimeType, size, allowedTypes[mimeType]
	}

	if msg := galleryFieldError(item, stored != ""); msg != "" {
		fail(http.StatusUnprocessableEntity, msg)
		return
	}
	err = a.db.QueryRowContext(r.Context(), `
		INSERT INTO gallery_items (title, description, category, format, promotional, visibility, storage_key, mime_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		item.Title, item.Description, item.Category, item.Format, item.Promotional, item.Visibility, item.storageKey, item.MimeType, item.SizeBytes).Scan(&item.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	committed = true
	http.Redirect(w, r, fmt.Sprintf("/gallery/%d", item.ID), http.StatusSeeOther)
}

// uploadReadFailed answers a failed read: an over-limit upload gets 413, anything else is a server error.
func (a *App) uploadReadFailed(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		a.render(w, http.StatusRequestEntityTooLarge, "gallery_upload", page{
			Item:  &galleryItem{Visibility: visibilityEveryone},
			Error: "This file is too large. Uploads are limited to about 2 GB, and nothing was stored.",
		})
		return
	}
	serverError(w, err)
}

func setGalleryField(item *galleryItem, name, value string) {
	switch name {
	case "title":
		item.Title = value
	case "description":
		item.Description = value
	case "category":
		item.Category = value
	case "visibility":
		item.Visibility = value
	case "promotional":
		item.Promotional = value == "on"
	}
}

// galleryFieldError returns a message for the first invalid field, or "". The Admin form offers only everyone and learners.
func galleryFieldError(item galleryItem, hasFile bool) string {
	switch {
	case !hasFile:
		return "Choose a file to upload."
	case item.Title == "" || utf8.RuneCountInString(item.Title) > 200:
		return "Enter a title (up to 200 characters)."
	case utf8.RuneCountInString(item.Description) > 2000:
		return "Use a description of at most 2000 characters."
	case utf8.RuneCountInString(item.Category) > 100:
		return "Use a category of at most 100 characters."
	case item.Visibility != visibilityEveryone && item.Visibility != visibilityLearners:
		return "Choose who may see this item."
	}
	return ""
}

// adminGallery lists every Gallery item for the Admin to edit or delete. mayView lets the Admin see all of them.
func (a *App) adminGallery(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	items, err := a.galleryItemsFor(r.Context(), learner, false)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "admin_gallery", page{Learner: learner, Items: items})
}

func (a *App) editGalleryForm(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	a.render(w, http.StatusOK, "gallery_edit", page{Item: &item})
}

// updateGalleryItem saves the Admin's edits. The file itself never changes.
func (a *App) updateGalleryItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	item.Title = strings.TrimSpace(r.PostFormValue("title"))
	item.Description = strings.TrimSpace(r.PostFormValue("description"))
	item.Category = strings.TrimSpace(r.PostFormValue("category"))
	item.Visibility = r.PostFormValue("visibility")
	item.Promotional = r.PostFormValue("promotional") == "on"
	if msg := galleryFieldError(item, true); msg != "" {
		a.render(w, http.StatusUnprocessableEntity, "gallery_edit", page{Item: &item, Error: msg})
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`UPDATE gallery_items SET title = $2, description = $3, category = $4, promotional = $5, visibility = $6 WHERE id = $1`,
		item.ID, item.Title, item.Description, item.Category, item.Promotional, item.Visibility)
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/gallery", http.StatusSeeOther)
}

func (a *App) confirmDeleteGalleryItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	a.render(w, http.StatusOK, "delete_gallery_item", page{Item: &item})
}

// deleteGalleryItem removes the row, then its stored file.
func (a *App) deleteGalleryItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM gallery_items WHERE id = $1`, item.ID); err != nil {
		serverError(w, err)
		return
	}
	if err := a.store.Remove(item.storageKey); err != nil && !errors.Is(err, os.ErrNotExist) {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/gallery", http.StatusSeeOther)
}
