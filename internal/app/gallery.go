package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers PNG for image.Decode
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP for image.Decode
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

// mayView is the single access decision: may viewer (nil for a Visitor) see this Gallery item? Every page and file download goes through it.
// A course_members item is visible to a Learner enrolled in any Course that uses it in a published Lesson.
func (a *App) mayView(ctx context.Context, viewer *learner, item galleryItem) (bool, error) {
	if viewer != nil && viewer.IsAdmin {
		return true, nil
	}
	switch item.Visibility {
	case visibilityEveryone:
		return true, nil
	case visibilityLearners:
		return viewer != nil, nil
	}
	if viewer == nil {
		return false, nil
	}
	var ok bool
	err := a.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM lessons l
		JOIN enrollments e ON e.course_id = l.course_id
		WHERE l.gallery_item_id = $1 AND l.published AND e.learner_id = $2)`, item.ID, viewer.ID).Scan(&ok)
	return ok, err
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
		a.serverError(w, r, err)
		return galleryItem{}, false
	}
	return item, true
}

// viewItem loads the item named in the path and applies mayView. When it returns false, the response is already written.
func (a *App) viewItem(w http.ResponseWriter, r *http.Request) (galleryItem, bool) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return galleryItem{}, false
	}
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return galleryItem{}, false
	}
	allowed, err := a.mayView(r.Context(), learner, item)
	if err != nil {
		a.serverError(w, r, err)
		return galleryItem{}, false
	}
	if !allowed {
		if learner == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return galleryItem{}, false
		}
		a.refuseMembersItem(w, r, item)
		return galleryItem{}, false
	}
	return item, true
}

// refuseMembersItem answers a Learner who may not see a members-only item: it names the Courses that use the item so the Learner can join one.
func (a *App) refuseMembersItem(w http.ResponseWriter, r *http.Request, item galleryItem) {
	courses, err := a.queryCourses(r, `SELECT `+courseColumns+` FROM courses
		WHERE id IN (SELECT l.course_id FROM lessons l WHERE l.gallery_item_id = $1 AND l.published)
		ORDER BY start_date NULLS LAST, id`, item.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusForbidden, "members_only", page{Courses: courses,
		Message: "This item is only for members of its Courses."})
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

// galleryItemsFor lists the Gallery items the viewer may see, newest first.
func (a *App) galleryItemsFor(ctx context.Context, viewer *learner) ([]galleryItem, error) {
	return a.visibleGalleryItems(ctx, viewer, `TRUE`)
}

// promotionalItemsFor lists the promotional Gallery items the viewer may see, newest first.
func (a *App) promotionalItemsFor(ctx context.Context, viewer *learner) ([]galleryItem, error) {
	return a.visibleGalleryItems(ctx, viewer, `promotional`)
}

// visibleGalleryItems lists the Gallery items matching the SQL condition where that the viewer may see, newest first.
func (a *App) visibleGalleryItems(ctx context.Context, viewer *learner, where string) ([]galleryItem, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+galleryColumns+` FROM gallery_items WHERE `+where+` ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	var all []galleryItem
	for rows.Next() {
		item, err := scanGalleryItem(rows.Scan)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var items []galleryItem
	// ponytail: one mayView query per course_members item (N+1); fine at this Gallery's size, batch the Enrollment
	// lookups into one query if the Gallery grows to thousands of items.
	for _, item := range all {
		ok, err := a.mayView(ctx, viewer, item)
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

// gallery lists what the viewer may see, narrowed by the ?format and ?category filters.
func (a *App) gallery(w http.ResponseWriter, r *http.Request) {
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
	a.render(w, r, http.StatusOK, "gallery", page{PublicBaseURL: a.publicBaseURL, Learner: learner, Items: shown,
		Format: format, Category: category, Categories: categories})
}

func (a *App) galleryItemPage(w http.ResponseWriter, r *http.Request) {
	item, ok := a.viewItem(w, r)
	if !ok {
		return
	}
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "gallery_item", page{PublicBaseURL: a.publicBaseURL, Learner: learner, Item: &item})
}

func (a *App) galleryFile(w http.ResponseWriter, r *http.Request) {
	item, ok := a.viewItem(w, r)
	if !ok {
		return
	}
	if item.Format == "ebook" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="cave-gallery-%d.pdf"`, item.ID))
	}
	a.serveStored(w, r, item.storageKey, item.MimeType)
}

// serveStored writes the stored file for key as contentType.
func (a *App) serveStored(w http.ResponseWriter, r *http.Request, key, contentType string) {
	f, err := a.store.Open(key)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// An item's file never changes, as no code replaces it after upload, so browsers keep it and skip a round trip to the server in France.
	// Private keeps logged-in-only items out of shared caches; a day bounds how long a deleted item lingers.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	// ServeContent handles Range requests, so videos can be seeked.
	http.ServeContent(w, r, "", info.ModTime(), f)
}

const (
	thumbWidth     = 800
	thumbMaxPixels = 50_000_000 // larger pictures are not decoded, so a huge upload cannot exhaust memory
)

// thumbKey names a picture's thumbnail in storage, next to the original.
func thumbKey(item galleryItem) string { return item.storageKey + ".thumb" }

// galleryThumb serves a picture's thumbnail, making and keeping it on first request.
// A picture that cannot be thumbnailed is served whole.
func (a *App) galleryThumb(w http.ResponseWriter, r *http.Request) {
	item, ok := a.viewItem(w, r)
	if !ok {
		return
	}
	if item.Format != "picture" {
		http.NotFound(w, r)
		return
	}
	f, err := a.store.Open(thumbKey(item))
	if err == nil {
		f.Close()
	} else if errors.Is(err, os.ErrNotExist) {
		thumb, err := a.makeThumb(item)
		if err != nil {
			a.logError(r, fmt.Errorf("thumbnail for gallery item %d: %w", item.ID, err))
			a.serveStored(w, r, item.storageKey, item.MimeType)
			return
		}
		if err := a.store.Put(thumbKey(item), thumb); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.serveStored(w, r, thumbKey(item), "image/jpeg")
}

// makeThumb scales a picture down to thumbWidth wide (never up) as a JPEG, on white for transparent pictures.
func (a *App) makeThumb(item galleryItem) ([]byte, error) {
	f, err := a.store.Open(item.storageKey)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width*cfg.Height > thumbMaxPixels {
		return nil, fmt.Errorf("%dx%d is too large to thumbnail", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w > thumbWidth {
		w, h = thumbWidth, max(1, h*thumbWidth/w)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) galleryUploadForm(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "gallery_upload", page{Item: &galleryItem{Visibility: visibilityEveryone}})
}

// galleryUpload streams the file to disk as it arrives. Nothing is stored unless every field and the file are valid.
func (a *App) galleryUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		a.render(w, r, http.StatusBadRequest, "message", page{Title: "Upload not read", Message: "The upload form could not be read. Try again."})
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
		a.render(w, r, status, "gallery_upload", page{Item: &item, Error: msg})
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			a.uploadReadFailed(w, r, err)
			return
		}
		if part.FormName() != "file" {
			value, err := io.ReadAll(io.LimitReader(part, 4096))
			if err != nil {
				a.uploadReadFailed(w, r, err)
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
			a.uploadReadFailed(w, r, err)
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
			a.uploadReadFailed(w, r, err)
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
		a.serverError(w, r, err)
		return
	}
	committed = true
	http.Redirect(w, r, fmt.Sprintf("/gallery/%d", item.ID), http.StatusSeeOther)
}

// uploadReadFailed answers a failed read: an over-limit upload gets 413, anything else is a server error.
func (a *App) uploadReadFailed(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		a.render(w, r, http.StatusRequestEntityTooLarge, "gallery_upload", page{
			Item:  &galleryItem{Visibility: visibilityEveryone},
			Error: "This file is too large. Uploads are limited to about 2 GB, and nothing was stored.",
		})
		return
	}
	a.serverError(w, r, err)
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

// galleryFieldError returns a message for the first invalid field, or "".
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
	case item.Visibility != visibilityEveryone && item.Visibility != visibilityLearners && item.Visibility != visibilityCourseMembers:
		return "Choose who may see this item."
	}
	return ""
}

// adminGallery lists every Gallery item for the Admin to edit or delete. mayView lets the Admin see all of them.
func (a *App) adminGallery(w http.ResponseWriter, r *http.Request) {
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
	a.render(w, r, http.StatusOK, "admin_gallery", page{Learner: learner, Items: items})
}

func (a *App) editGalleryForm(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "gallery_edit", page{Item: &item})
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
		a.render(w, r, http.StatusUnprocessableEntity, "gallery_edit", page{Item: &item, Error: msg})
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`UPDATE gallery_items SET title = $2, description = $3, category = $4, promotional = $5, visibility = $6 WHERE id = $1`,
		item.ID, item.Title, item.Description, item.Category, item.Promotional, item.Visibility)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/gallery", http.StatusSeeOther)
}

func (a *App) confirmDeleteGalleryItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "delete_gallery_item", page{Item: &item})
}

// deleteGalleryItem removes the row, then its stored file.
func (a *App) deleteGalleryItem(w http.ResponseWriter, r *http.Request) {
	item, ok := a.galleryItemFor(w, r)
	if !ok {
		return
	}
	// A Lesson holds ON DELETE RESTRICT on its item, so the database refuses the delete while one uses it.
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM gallery_items WHERE id = $1`, item.ID)
	if isForeignKeyError(err) {
		a.render(w, r, http.StatusConflict, "delete_gallery_item", page{Item: &item,
			Error: "This item cannot be deleted because a Lesson uses it. Remove the Lesson first."})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	for _, key := range []string{item.storageKey, thumbKey(item)} {
		if err := a.store.Remove(key); err != nil && !errors.Is(err, os.ErrNotExist) {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/admin/gallery", http.StatusSeeOther)
}
