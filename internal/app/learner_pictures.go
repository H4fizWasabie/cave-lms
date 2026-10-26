package app

import (
	"bytes"
	"crypto/rand"
	"errors"
	"image"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

const (
	maxLearnerPicture  = 2 << 20 // the largest profile picture upload
	learnerPictureSide = 256     // a profile picture is stored as a square JPEG this many pixels wide
)

// initials stand in for a Learner's picture: the first letters of the first and last words of their name.
func initials(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}
	first := func(w string) string { return string(unicode.ToUpper([]rune(w)[0])) }
	if len(words) == 1 {
		return first(words[0])
	}
	return first(words[0]) + first(words[len(words)-1])
}

// Initials stand in for the Learner's picture when they have none.
func (l *learner) Initials() string { return initials(l.Name) }

// setMyPicture is the Learner uploading a profile picture, replacing any they had.
func (a *App) setMyPicture(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	status, refusal, key, err := a.storeLearnerPicture(w, r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if refusal != "" {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.renderMyProfile(w, r, status, learner, text.T(lang, refusal))
		return
	}
	if err := a.setLearnerPicture(r, learner.ID, key); err != nil {
		a.dropPicture(r, key)
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/my-profile", http.StatusSeeOther)
}

// removeMyPicture is the Learner removing their profile picture; their initials show instead.
func (a *App) removeMyPicture(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.setLearnerPicture(r, learner.ID, ""); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/my-profile", http.StatusSeeOther)
}

// setLearnerPicture makes key, "" for none, the Learner's picture and deletes the one it replaces.
func (a *App) setLearnerPicture(r *http.Request, learnerID int64, key string) error {
	var old string
	// ponytail: two uploads racing for one Learner can leave one file behind; lock the row if that ever matters.
	err := a.db.QueryRowContext(r.Context(), `UPDATE learners l SET picture_key = $2 FROM learners o
		WHERE l.id = $1 AND o.id = l.id RETURNING o.picture_key`, learnerID, key).Scan(&old)
	if err != nil {
		return err
	}
	a.dropPicture(r, old)
	return nil
}

// storeLearnerPicture reads the form's "picture" file, crops it to a centred square, scales it to
// learnerPictureSide and stores it as a JPEG under a new key. Only the re-encoded picture is stored. A file that is
// missing, too large, not a JPEG, PNG or WebP, or does not decode is refused with the status and the catalog key of
// the reason, and nothing is stored.
func (a *App) storeLearnerPicture(w http.ResponseWriter, r *http.Request) (status int, refusal, key string, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLearnerPicture+64<<10) // room for the form around the file
	file, header, err := r.FormFile("picture")
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, "learner-picture.err.too-large", "", nil
	case errors.Is(err, http.ErrMissingFile), errors.Is(err, http.ErrNotMultipart):
		return http.StatusUnprocessableEntity, "picture.err.type", "", nil
	case err != nil:
		return http.StatusBadRequest, "adm.gallery.unread", "", nil
	}
	defer file.Close()
	if header.Size > maxLearnerPicture {
		return http.StatusRequestEntityTooLarge, "learner-picture.err.too-large", "", nil
	}
	whole, mimeType, err := sniffPicture(file)
	if err != nil {
		return 0, "", "", err
	}
	if mimeType == "" {
		return http.StatusUnprocessableEntity, "picture.err.type", "", nil
	}
	picture, err := squareJPEG(whole)
	if err != nil {
		return http.StatusUnprocessableEntity, "learner-picture.err.unreadable", "", nil
	}
	key = "profile-" + rand.Text()
	return 0, "", key, a.store.Put(r.Context(), key, picture)
}

// squareJPEG decodes a picture, crops it to a centred square and scales it to learnerPictureSide, on white for
// transparent pictures.
// ponytail: EXIF orientation is ignored, so a phone photo may come out on its side; read the tag if Learners hit it.
func squareJPEG(file io.Reader) ([]byte, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	src, err := decodePicture(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(0, 0, side, side).Add(b.Min).Add(image.Pt((b.Dx()-side)/2, (b.Dy()-side)/2))
	return scaleToJPEG(src, crop, learnerPictureSide, learnerPictureSide)
}

// learnerPicture serves a Learner's profile picture, to logged-in users only, by its storage key; other stored files
// are not served here.
func (a *App) learnerPicture(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var found bool
	err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM learners WHERE picture_key = $1 AND picture_key <> '')`, key).Scan(&found)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !found {
		a.notFound(w, r)
		return
	}
	a.serveStored(w, r, key, "image/jpeg")
}
