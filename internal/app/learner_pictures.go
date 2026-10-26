package app

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
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
// transparent pictures, turned upright as a JPEG's EXIF orientation says. A centred square turns into the centred
// square of the turned picture, so turning the small result is the same as turning the photo first.
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
	return encodeJPEG(orient(scaleOnWhite(src, crop, learnerPictureSide, learnerPictureSide), exifOrientation(data)))
}

// exifOrientation is the Orientation tag, 1 to 8, of a JPEG's EXIF block: how the camera was held. It is 1, upright,
// for anything else, or when there is no tag.
func exifOrientation(jpg []byte) int {
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(jpg) && jpg[i] == 0xFF; {
		marker, size := jpg[i+1], int(binary.BigEndian.Uint16(jpg[i+2:]))
		if marker == 0xDA || size < 2 || i+2+size > len(jpg) { // the image data starts, or a broken segment
			return 1
		}
		if seg := jpg[i+4 : i+2+size]; marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation reads the Orientation tag from the first IFD of an EXIF block's TIFF data.
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	for e := ifd + 2; e < ifd+2+12*int(order.Uint16(tiff[ifd:])) && e+12 <= len(tiff); e += 12 {
		if order.Uint16(tiff[e:]) == 0x0112 {
			if o := int(order.Uint16(tiff[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

// orient turns or flips a square picture upright from EXIF orientation o.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 {
		return src
	}
	n := src.Bounds().Dx()
	dst := image.NewRGBA(src.Bounds())
	for y := range n {
		for x := range n {
			// Where the pixel at (x, y) goes, per the EXIF specification's eight orientations.
			dx, dy := x, y
			switch o {
			case 2:
				dx = n - 1 - x
			case 3:
				dx, dy = n-1-x, n-1-y
			case 4:
				dy = n - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = n-1-y, x
			case 7:
				dx, dy = n-1-y, n-1-x
			case 8:
				dx, dy = y, n-1-x
			}
			dst.SetRGBA(dx, dy, src.RGBAAt(x, y))
		}
	}
	return dst
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
