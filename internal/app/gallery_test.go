package app_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// Tiny fabricated files with the right magic numbers.
var (
	pdfFile  = []byte("%PDF-1.4\n%fake ebook\n")
	jpegFile = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00}
	pngFile  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	webpFile = []byte("RIFF\x00\x00\x00\x00WEBPVP8 fake")
	mp4File  = []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2mp41fake")
)

// upload posts the Gallery upload form as multipart with the given text fields and file content.
func (s *site) upload(fields map[string]string, content []byte) (*http.Response, string) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			s.t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", "upload.bin")
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		s.t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		s.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/admin/gallery", &buf)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req)
}

func (s *site) getWithHeader(path, key, value string) (*http.Response, []byte) {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodGet, s.server.URL+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set(key, value)
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return res, body
}

func titleFields(title, visibility string) map[string]string {
	return map[string]string{"title": title, "description": "A description", "category": "Maths", "visibility": visibility}
}

// uploadAs uploads one file as the Admin and returns the new item's path, e.g. /gallery/3.
func (s *site) uploadAs(title, visibility string, content []byte) string {
	s.t.Helper()
	res, body := s.upload(titleFields(title, visibility), content)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	return res.Header.Get("Location")
}

var galleryItemPath = regexp.MustCompile(`^/gallery/\d+$`)

func (s *site) countGalleryItems() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM gallery_items`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func (s *site) storedFiles() int {
	s.t.Helper()
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		s.t.Fatal(err)
	}
	return len(entries)
}

func TestAdminUploadsPictureAndItDisplays(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")

	res, body := s.upload(titleFields("Poster", "everyone"), pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	location := res.Header.Get("Location")
	if !galleryItemPath.MatchString(location) {
		t.Fatalf("Location = %q, want /gallery/<id>", location)
	}

	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusOK, "Poster")
	if !strings.Contains(body, "<img") {
		t.Fatalf("picture page has no <img>: %s", body)
	}

	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusOK, "")
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if body != string(pngFile) {
		t.Fatalf("served file differs from the upload")
	}
}

func TestUploadAcceptsEachAllowedFormatByContent(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	for _, tc := range []struct {
		name     string
		content  []byte
		mimeType string
	}{
		{"mp4", mp4File, "video/mp4"},
		{"pdf", pdfFile, "application/pdf"},
		{"jpeg", jpegFile, "image/jpeg"},
		{"png", pngFile, "image/png"},
		{"webp", webpFile, "image/webp"},
	} {
		location := s.uploadAs(tc.name, "everyone", tc.content)
		res, body := s.get(location + "/file")
		expectStatus(t, res, body, http.StatusOK, "")
		if got := res.Header.Get("Content-Type"); got != tc.mimeType {
			t.Fatalf("%s: Content-Type = %q, want %q", tc.name, got, tc.mimeType)
		}
	}
}

func TestUploadRefusesWrongTypeAndStoresNothing(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")

	// Named like a picture but the content is an executable header.
	res, body := s.upload(titleFields("Sneaky", "everyone"), []byte("MZ\x90\x00 not a picture"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Upload an MP4 video, a PDF ebook, or a JPEG, PNG or WebP picture.")
	if n := s.countGalleryItems(); n != 0 {
		t.Fatalf("%d gallery items stored after a wrong-type upload, want 0", n)
	}
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored after a wrong-type upload, want 0", n)
	}
}

func TestUploadRefusesMissingTitleAndStoresNothing(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")

	res, body := s.upload(map[string]string{"visibility": "everyone"}, pngFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a title")
	if n := s.countGalleryItems() + s.storedFiles(); n != 0 {
		t.Fatalf("%d rows and files stored after an upload without a title, want 0", n)
	}
}

func TestUploadRefusesTooLargeAndStoresNothing(t *testing.T) {
	s := newSite(t, func(c *config.Config) { c.MaxUploadBytes = 1024 })
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")

	big := append(append([]byte{}, pngFile...), bytes.Repeat([]byte{0}, 2048)...)
	res, body := s.upload(titleFields("Huge", "everyone"), big)
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "too large")
	if n := s.countGalleryItems() + s.storedFiles(); n != 0 {
		t.Fatalf("%d rows and files stored after a too-large upload, want 0", n)
	}
}

func TestOnlyTheAdminCanUpload(t *testing.T) {
	s := newSite(t)
	res, body := s.upload(titleFields("Poster", "everyone"), pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor upload redirected to %q, want /login", loc)
	}

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.upload(titleFields("Poster", "everyone"), pngFile)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	if n := s.countGalleryItems(); n != 0 {
		t.Fatalf("%d gallery items stored by non-Admins, want 0", n)
	}
}

func TestVideoRangeRequestSeeks(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Lesson video", "everyone", mp4File)

	res, body := s.getWithHeader(location+"/file", "Range", "bytes=4-7")
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("range request status = %d, want 206", res.StatusCode)
	}
	if want := "bytes 4-7/" + strconv.Itoa(len(mp4File)); res.Header.Get("Content-Range") != want {
		t.Fatalf("Content-Range = %q, want %q", res.Header.Get("Content-Range"), want)
	}
	if string(body) != string(mp4File[4:8]) {
		t.Fatalf("range body = %q, want %q", body, mp4File[4:8])
	}
}

func TestEbookDownloads(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Workbook", "everyone", pdfFile)

	res, body := s.get(location + "/file")
	expectStatus(t, res, body, http.StatusOK, "")
	if !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", res.Header.Get("Content-Disposition"))
	}
}

func TestVisitorIsRefusedLoggedInOnlyItem(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Members picture", "learners", pngFile)
	s.logOut()

	res, body := s.get(location)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor page redirected to %q, want /login", loc)
	}
	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if strings.Contains(body, "\x89PNG") {
		t.Fatal("Visitor got the file bytes")
	}

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusOK, "")
}

func TestGalleryListShowsOnlyWhatTheViewerMaySee(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.uploadAs("Open poster", "everyone", pngFile)
	s.uploadAs("Members poster", "learners", pngFile)
	s.logOut()

	res, body := s.get("/gallery")
	expectStatus(t, res, body, http.StatusOK, "Open poster")
	if strings.Contains(body, "Members poster") {
		t.Fatal("Visitor sees a logged-in-only item in the Gallery list")
	}
}

func TestCourseMembersItemIsHiddenFromLearnersOutsideItsCourses(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Course picture", "everyone", pngFile)
	if _, err := s.db.Exec(`UPDATE gallery_items SET visibility = 'course_members'`); err != nil {
		t.Fatal(err)
	}

	res, body := s.get(location + "/file")
	expectStatus(t, res, body, http.StatusOK, "")

	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
}

func TestStoredFilesAreNotPublic(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.uploadAs("Poster", "everyone", pngFile)

	var key string
	if err := s.db.QueryRow(`SELECT storage_key FROM gallery_items`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	res, body := s.get("/" + key)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get("/gallery/" + key)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestGalleryFiltersByFormat(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.uploadAs("Snapshot", "everyone", pngFile)
	s.uploadAs("Workbook", "everyone", pdfFile)
	s.logOut()

	res, body := s.get("/gallery?format=ebook")
	expectStatus(t, res, body, http.StatusOK, "Workbook")
	if strings.Contains(body, "Snapshot") {
		t.Fatal("format=ebook lists a picture")
	}
}

func TestGalleryFiltersByCategory(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.uploadAs("Algebra", "everyone", pngFile) // category Maths
	res, body := s.upload(map[string]string{"title": "Circuits", "category": "Physics", "visibility": "everyone"}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()

	res, body = s.get("/gallery?category=Physics")
	expectStatus(t, res, body, http.StatusOK, "Circuits")
	if strings.Contains(body, "Algebra") {
		t.Fatal("category=Physics lists a Maths item")
	}
}

func TestPromotionalItemsShowOnTheHomePage(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.upload(map[string]string{"title": "Taster lesson", "category": "Maths", "promotional": "on", "visibility": "everyone"}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.uploadAs("Plain picture", "everyone", pngFile)
	s.logOut()

	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "Taster lesson")
	if strings.Contains(body, "Plain picture") {
		t.Fatal("home page lists a non-promotional item")
	}
}

func TestPromotionalItemDefaultsToEveryone(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	// No visibility field sent: a promotional item must still be public.
	res, body := s.upload(map[string]string{"title": "Open promo", "promotional": "on"}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()

	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "Open promo")
}

func TestHiddenItemsNeverLeakIntoListings(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.upload(map[string]string{"title": "Members promo", "category": "Secret", "promotional": "on", "visibility": "learners"}, pngFile)
	s.logOut()

	res, body := s.get("/")
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(body, "Members promo") {
		t.Fatal("Visitor sees a logged-in-only promotional item on the home page")
	}
	res, body = s.get("/gallery")
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(body, "Members promo") || strings.Contains(body, "Secret") {
		t.Fatal("Visitor sees a hidden item or its category in the Gallery")
	}

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "Members promo")
}

func TestAdminListsEveryItemWithEditAndDeleteLinks(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.uploadAs("Open poster", "everyone", pngFile)
	s.uploadAs("Members poster", "learners", pngFile)

	res, body := s.get("/admin/gallery")
	expectStatus(t, res, body, http.StatusOK, "Members poster")
	if !strings.Contains(body, "Open poster") || strings.Count(body, "/edit") != 2 || strings.Count(body, "/delete") != 2 {
		t.Fatalf("Admin list does not show both items with Edit and Delete: %s", body)
	}
}

func TestOnlyTheAdminManagesTheGallery(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Poster", "everyone", pngFile)
	id := strings.TrimPrefix(location, "/gallery/")
	s.logOut()

	res, body := s.get("/admin/gallery")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor redirected to %q, want /login", loc)
	}
	res, body = s.get("/admin/gallery/" + id + "/edit")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	for _, path := range []string{"/admin/gallery", "/admin/gallery/" + id + "/edit", "/admin/gallery/" + id + "/delete"} {
		res, body = s.get(path)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	res, body = s.post("/admin/gallery/"+id+"/delete", url.Values{})
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	if n := s.countGalleryItems(); n != 1 {
		t.Fatalf("%d gallery items after a Learner's delete, want 1", n)
	}
}

func TestAdminEditsAGalleryItem(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Old title", "everyone", pngFile)
	id := strings.TrimPrefix(location, "/gallery/")

	res, body := s.get("/admin/gallery/" + id + "/edit")
	expectStatus(t, res, body, http.StatusOK, `value="Old title"`)

	res, body = s.post("/admin/gallery/"+id, url.Values{"title": {"New title"}, "description": {"Fixed"}, "category": {"Physics"}, "promotional": {"on"}, "visibility": {"learners"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/gallery" {
		t.Fatalf("edit redirected to %q, want /admin/gallery", loc)
	}
	res, body = s.get("/gallery/" + id)
	expectStatus(t, res, body, http.StatusOK, "New title")
	if strings.Contains(body, "Old title") {
		t.Fatal("edited item still shows its old title")
	}

	s.logOut()
	res, body = s.get("/")
	if strings.Contains(body, "New title") {
		t.Fatal("Visitor sees the item after it became logged-in only")
	}
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "New title")
}

func TestEditRefusesInvalidInputAndKeepsTheItem(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Keep me", "everyone", pngFile)
	id := strings.TrimPrefix(location, "/gallery/")

	res, body := s.post("/admin/gallery/"+id, url.Values{"title": {""}, "visibility": {"everyone"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a title")
	res, body = s.post("/admin/gallery/"+id, url.Values{"title": {"Renamed"}, "visibility": {"nobody"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose who may see this item.")

	res, body = s.get("/gallery/" + id)
	expectStatus(t, res, body, http.StatusOK, "Keep me")
}

func TestAdminDeletesAGalleryItemAndItsFile(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Doomed", "everyone", realPNG(t, 1600, 900))
	id := strings.TrimPrefix(location, "/gallery/")
	res, body := s.get(location + "/thumb") // leaves a thumbnail on disk too
	expectStatus(t, res, body, http.StatusOK, "")

	res, body = s.get("/admin/gallery/" + id + "/delete")
	expectStatus(t, res, body, http.StatusOK, "Doomed")

	res, body = s.post("/admin/gallery/"+id+"/delete", url.Values{})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/gallery" {
		t.Fatalf("delete redirected to %q, want /admin/gallery", loc)
	}
	if n := s.countGalleryItems(); n != 0 {
		t.Fatalf("%d gallery items after delete, want 0", n)
	}
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after delete, want 0", n)
	}
	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestGalleryFormsOfferCourseMembersOnly(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Diagram", "everyone", pngFile)

	res, body := s.get("/admin/gallery/new")
	expectStatus(t, res, body, http.StatusOK, `value="course_members"`)
	res, body = s.get("/admin/gallery/" + strings.TrimPrefix(location, "/gallery/") + "/edit")
	expectStatus(t, res, body, http.StatusOK, `value="course_members"`)
}

func TestLearnerWhoLeftOrWasRemovedCannotDownloadACourseMembersFile(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	file := s.uploadAs("Diagram", "course_members", pngFile) + "/file"
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addItemLesson(id, strings.TrimPrefix(strings.TrimSuffix(file, "/file"), "/gallery/"), "Diagram lesson", false)

	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	res, body := ada.get(file)
	expectStatus(t, res, body, http.StatusOK, "")
	ada.post("/courses/"+id+"/leave", nil)
	res, body = ada.get(file)
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")

	grace := s.second()
	grace.loginAsNewLearner("Grace", "grace@example.com", "correct horse battery")
	grace.post("/courses/"+id+"/join", nil)
	res, body = grace.get(file)
	expectStatus(t, res, body, http.StatusOK, "")
	_, body = s.get("/admin/courses/" + id)
	m := removeLearnerLink.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("Admin Course page has no remove button for Grace: %s", body)
	}
	s.post("/admin/courses/"+id+"/learners/"+m[1]+"/remove", nil)
	res, body = grace.get(file)
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
}

// realPNG encodes a plain w×h picture, since thumbnails need a file that actually decodes.
func realPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPictureThumbnailIsASmallJPEG(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Big photo", "everyone", realPNG(t, 1600, 900))

	res, body := s.get(location + "/thumb")
	expectStatus(t, res, body, http.StatusOK, "")
	if got := res.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	cfg, err := jpeg.DecodeConfig(strings.NewReader(body))
	if err != nil {
		t.Fatalf("thumbnail is not a JPEG: %v", err)
	}
	if cfg.Width != 800 || cfg.Height != 450 {
		t.Fatalf("thumbnail is %dx%d, want 800x450", cfg.Width, cfg.Height)
	}

	ebook := s.uploadAs("Book", "everyone", pdfFile)
	res, body = s.get(ebook + "/thumb")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestVisitorIsRefusedALoggedInOnlyThumbnail(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Members picture", "learners", realPNG(t, 1600, 900))
	s.logOut()

	res, body := s.get(location + "/thumb")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Fatalf("Visitor thumbnail redirected to %q, want /login", loc)
	}
}

func TestPictureThatCannotBeThumbnailedIsServedWhole(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Broken picture", "everyone", pngFile)

	res, body := s.get(location + "/thumb")
	expectStatus(t, res, body, http.StatusOK, "")
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if body != string(pngFile) {
		t.Fatal("thumbnail fallback differs from the upload")
	}
}

func TestGalleryTilesShowThumbnails(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.upload(map[string]string{"title": "Taster", "promotional": "on", "visibility": "everyone"}, realPNG(t, 1600, 900))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	thumb := `src="` + res.Header.Get("Location") + `/thumb"`

	for _, path := range []string{"/gallery", "/"} {
		res, body = s.get(path)
		expectStatus(t, res, body, http.StatusOK, thumb)
	}
}
