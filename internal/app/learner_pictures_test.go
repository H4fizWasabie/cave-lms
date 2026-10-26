package app_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var learnerPictureSrc = regexp.MustCompile(`src="(/learner-pictures/([^"]+))"`)

// photo is a w x h PNG, wider or taller than square so the crop shows.
func photo(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// uploadPicture posts data as the logged-in Learner's profile picture, asking for lang.
func (s *site) uploadPicture(data []byte, lang string) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("picture", "me.png")
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		s.t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/my-profile/picture", &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept-Language", lang)
	return s.do(req)
}

// myPicture is the path and storage key of the logged-in Learner's picture as their profile page shows it.
func (s *site) myPicture() (path, key string) {
	s.t.Helper()
	_, body := s.get("/my-profile")
	m := learnerPictureSrc.FindStringSubmatch(content(body))
	if m == nil {
		s.t.Fatalf("the profile page shows no picture: %s", body)
	}
	return m[1], m[2]
}

func (s *site) stored(key string) bool {
	_, err := os.Stat(filepath.Join(s.storageDir, key))
	return err == nil
}

func TestALearnerUploadsAProfilePictureStoredAsA256SquareJPEG(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Siti Nur Aisyah", "siti@example.com", "correct horse battery")
	_, body := s.get("/my-profile")
	expectAll(t, content(body), `action="/my-profile/picture"`, `>SA<`)

	res, body := s.uploadPicture(photo(t, 600, 400), "en")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if got := res.Header.Get("Location"); got != "/my-profile" {
		t.Fatalf("an upload redirects to %q, want /my-profile", got)
	}
	path, _ := s.myPicture()
	_, body = s.get("/my-profile")
	expectAll(t, body, `action="/my-profile/picture/remove"`)

	res, picture := s.get(path)
	expectStatus(t, res, picture, http.StatusOK, "")
	if ct := res.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("a profile picture is served as %q, want image/jpeg", ct)
	}
	img, err := jpeg.Decode(strings.NewReader(picture))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("a profile picture is %dx%d, want 256x256", b.Dx(), b.Dy())
	}
	// The header shows the Learner's picture beside their name.
	_, body = s.get("/")
	expectAll(t, body, `src="`+path+`"`)
}

func TestReplacingOrRemovingAProfilePictureDeletesTheOldFile(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	expectStatus(t, must(s.uploadPicture(photo(t, 300, 500), "en")), "", http.StatusSeeOther, "")
	oldPath, oldKey := s.myPicture()

	expectStatus(t, must(s.uploadPicture(photo(t, 400, 400), "en")), "", http.StatusSeeOther, "")
	_, newKey := s.myPicture()
	if newKey == oldKey || s.stored(oldKey) || !s.stored(newKey) {
		t.Fatalf("after a replace: old %s stored %v, new %s stored %v", oldKey, s.stored(oldKey), newKey, s.stored(newKey))
	}
	res, body := s.get(oldPath)
	expectStatus(t, res, body, http.StatusNotFound, "")

	res, body = s.post("/my-profile/picture/remove", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if s.stored(newKey) {
		t.Fatal("removing the picture left its file behind")
	}
	_, body = s.get("/my-profile")
	expectNone(t, content(body), "/learner-pictures/", `action="/my-profile/picture/remove"`)
	expectAll(t, content(body), `>A<`)
}

func TestAProfilePictureOfTheWrongTypeTooLargeOrUnreadableIsRefusedInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	before := s.storedFiles()
	big := append(photo(t, 10, 10), make([]byte, 2<<20)...)
	for _, c := range []struct {
		name   string
		data   []byte
		status int
		en, ms string
	}{
		{"a PDF", pdfFile, http.StatusUnprocessableEntity, "Upload a JPEG, PNG or WebP picture.", "Muat naik gambar JPEG, PNG atau WebP."},
		{"over 2 MB", big, http.StatusRequestEntityTooLarge, "Profile pictures are limited to 2 MB", "Gambar profil terhad kepada 2 MB"},
		{"not decodable", pngFile, http.StatusUnprocessableEntity, "This picture could not be read", "Gambar ini tidak dapat dibaca"},
	} {
		res, body := s.uploadPicture(c.data, "en")
		expectStatus(t, res, body, c.status, c.en)
		res, body = s.uploadPicture(c.data, "ms")
		expectStatus(t, res, body, c.status, c.ms)
	}
	if got := s.storedFiles(); got != before {
		t.Fatalf("refused uploads left %d files, want %d", got, before)
	}
	_, body := s.get("/my-profile")
	expectNone(t, content(body), "/learner-pictures/")
}

// The picture route serves only to logged-in users, and only profile pictures: a Gallery file or a Course picture
// keeps its own route and access check.
func TestTheProfilePictureRouteServesOnlyProfilePicturesToLoggedInUsers(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	expectStatus(t, must(s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)), "", http.StatusSeeOther, "")
	s.uploadAs("Poster", "everyone", pngFile)
	expectStatus(t, must(s.uploadPicture(photo(t, 300, 300), "en")), "", http.StatusSeeOther, "")
	path, mine := s.myPicture()

	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == mine {
			continue
		}
		res, body := s.get("/learner-pictures/" + e.Name())
		expectStatus(t, res, body, http.StatusNotFound, "")
	}

	s.logOut()
	res, body := s.get(path)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("a Visitor asking for a profile picture is sent to %q, want /login", res.Header.Get("Location"))
	}
}

func TestChatMessagesShowTheWritersPictureOrTheirInitials(t *testing.T) {
	ada, _, ben, id := communitySite(t)
	expectStatus(t, must(ada.uploadPicture(photo(t, 300, 300), "en")), "", http.StatusSeeOther, "")
	path, _ := ada.myPicture()
	expectStatus(t, must(ben.post("/community/join", nil)), "", http.StatusSeeOther, "")

	for _, chat := range []string{community, id} {
		ada.say(chat, "Hello from Ada")
		ben.say(chat, "Hello from Ben")
		_, body := ada.get(chatURL(chat))
		expectInOrder(t, body, `<img class="msg-avatar" src="`+path+`"`, "Hello from Ada", `<span class="msg-avatar" aria-hidden="true">B</span>`, "Hello from Ben")
	}
}
