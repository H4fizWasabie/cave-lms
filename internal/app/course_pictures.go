package app

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
)

// maxPictureForm caps a form that carries a Programme or Course picture, well below the Gallery's limit.
const maxPictureForm = 20 << 20

// readPictureForm parses a form that may carry a picture, multipart or not, capped at maxPictureForm. When it
// cannot, it returns the status and the catalog key of the message to show.
func readPictureForm(w http.ResponseWriter, r *http.Request) (status int, msgKey string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPictureForm)
	err := r.ParseMultipartForm(1 << 20)
	var tooLarge *http.MaxBytesError
	switch {
	case err == nil, errors.Is(err, http.ErrNotMultipart):
		return 0, ""
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, "programmes.err.too-large"
	default:
		return http.StatusBadRequest, "adm.gallery.unread"
	}
}

// storePicture stores the parsed form's "picture" file. key is "" when none was sent; bad reports a file that is
// not a JPEG, PNG or WebP picture, which is not stored.
func (a *App) storePicture(r *http.Request) (key, mimeType string, bad bool, err error) {
	file, _, err := r.FormFile("picture")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	defer file.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", false, err
	}
	mimeType, ok := sniffType(head[:n])
	if !ok || allowedTypes[mimeType] != "picture" {
		return "", "", true, nil
	}
	key, _, err = a.store.Save(io.MultiReader(bytes.NewReader(head[:n]), file))
	return key, mimeType, false, err
}

// coursePicture serves a Course's picture by its storage key; Gallery files are not served here.
func (a *App) coursePicture(w http.ResponseWriter, r *http.Request) {
	var contentType string
	err := a.db.QueryRowContext(r.Context(), `SELECT picture_type FROM courses WHERE picture_key = $1 AND picture_key <> ''`, r.PathValue("key")).Scan(&contentType)
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.serveStored(w, r, r.PathValue("key"), contentType)
}
