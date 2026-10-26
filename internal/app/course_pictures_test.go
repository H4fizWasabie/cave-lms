package app_test

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// saveCourse posts the Course form to path as multipart, with picture as its file when not nil.
func (s *site) saveCourse(path string, fields map[string]string, picture []byte) (*http.Response, string) {
	s.t.Helper()
	return s.saveProgramme(path, fields, picture)
}

var coursePictureSrc = regexp.MustCompile(`src="(/course-pictures/[^"]+)"`)

func TestAdminAddsACoursePictureThatTheCourseListShows(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	_, body = s.get("/courses")
	m := coursePictureSrc.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the Course list shows no Course picture: %s", body)
	}
	res, picture := s.get(m[1])
	expectStatus(t, res, picture, http.StatusOK, "")
	if picture != string(pngFile) || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("served picture differs from the upload (%s)", res.Header.Get("Content-Type"))
	}
}

// The Course picture route serves Course pictures only: a Gallery item's file, members-only or not, keeps its own
// access check, and a picture is never sniffed as something else.
func TestTheCoursePictureRouteServesOnlyCoursePicturesUnsniffed(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.uploadAs("Poster", "everyone", pngFile)
	members := s.uploadAs("Members' poster", "everyone", pngFile)
	if _, err := s.db.Exec(`UPDATE gallery_items SET visibility = 'course_members' WHERE id = $1`,
		strings.TrimPrefix(members, "/gallery/")); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.Query(`SELECT storage_key FROM gallery_items`)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	if err := rows.Close(); err != nil || len(keys) != 2 {
		t.Fatalf("Gallery keys %v, %v", keys, err)
	}

	s.logOut()
	_, body = s.get("/courses")
	m := coursePictureSrc.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the Course list shows no Course picture: %s", body)
	}
	res, body = s.get(m[1])
	expectStatus(t, res, body, http.StatusOK, "")
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("a Course picture is served with X-Content-Type-Options %q, want nosniff", got)
	}
	for _, k := range keys {
		res, body = s.get("/course-pictures/" + k)
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
}

var drawnCover = regexp.MustCompile(`(?s)<div class="course-cover course-cover-drawn.*?</div>`)

// drawnCoverIn returns the first drawn cover in html, failing when there is none.
func drawnCoverIn(t *testing.T, html string) string {
	t.Helper()
	c := drawnCover.FindString(html)
	if c == "" {
		t.Fatalf("no drawn cover: %s", html)
	}
	return c
}

// The title sits right under the cover, so a drawn cover carries only the Level or format and the mark.
func TestACourseWithoutAPictureShowsADrawnCoverWithItsLevelButNotItsTitle(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "3")
	s.post("/admin/courses/"+copyID+"/publish", nil)

	s.logOut()
	_, body := s.get("/courses")
	expectNone(t, body, "/course-pictures/")
	for _, c := range []struct{ title, level string }{{"Go Basics", "Self-paced online"}, {"Go for Acme", "PCC Practitioner"}} {
		card := courseCard(t, body, c.title)
		cover := drawnCoverIn(t, card)
		expectInOrder(t, cover, "course-cover-mark", c.level)
		expectNone(t, cover, c.title)
		expectInOrder(t, card, cover, ">"+c.title+"</a>")
	}

	_, body = s.getAsking("/courses", "ms")
	expectInOrder(t, drawnCoverIn(t, courseCard(t, body, "Go Basics")), "Dalam talian, kendiri")
	_, body = s.get("/courses/" + id)
	expectNone(t, drawnCoverIn(t, body), "Go Basics")
}

func TestTheCoursePageHeroShowsTheCoursePictureOrItsDrawnCover(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.saveCourse("/admin/courses", map[string]string{"title": "Rust Basics", "description": "Learn Rust."}, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	pictured, drawn := s.courseIDByTitle("Go Basics"), s.courseIDByTitle("Rust Basics")

	s.logOut()
	_, body = s.get("/courses/" + pictured)
	expectInOrder(t, body, `class="cp-hero-text"`, "<h1>Go Basics</h1>", `class="cp-hero-media"`, `<img class="course-cover" src="/course-pictures/`, "</section>")
	_, body = s.get("/courses/" + drawn)
	expectInOrder(t, body, `class="cp-hero-text"`, "<h1>Rust Basics</h1>", `class="cp-hero-media"`, "course-cover-drawn", "</section>")
}

func TestAdminReplacesThenRemovesACoursePictureFromTheCourseForm(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	fields := map[string]string{"title": "Go Basics", "description": "Learn Go."}
	s.saveCourse("/admin/courses", fields, pngFile)
	id := s.onlyCourseID()
	old := s.firstMatch("/courses", coursePictureSrc)

	res, body := s.get("/admin/courses/" + id + "/edit")
	expectInOrder(t, body, `enctype="multipart/form-data"`, `src="`+old+`"`, `name="picture"`, `name="remove_picture"`)

	res, body = s.saveCourse("/admin/courses/"+id, fields, jpegFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	replaced := s.firstMatch("/courses", coursePictureSrc)
	res, picture := s.get(replaced)
	if picture != string(jpegFile) {
		t.Fatalf("the Course shows the old picture after replacing it")
	}
	res, _ = s.get(old)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET old picture = %d, want 404", res.StatusCode)
	}
	if n := s.storedFiles(); n != 1 {
		t.Fatalf("%d files stored after replacing, want 1", n)
	}

	// Saving without a file keeps the picture.
	s.saveCourse("/admin/courses/"+id, fields, nil)
	if got := s.firstMatch("/courses", coursePictureSrc); got != replaced {
		t.Fatalf("saving without a file changed the picture to %s", got)
	}

	res, body = s.saveCourse("/admin/courses/"+id, map[string]string{"title": "Go Basics", "description": "Learn Go.", "remove_picture": "on"}, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/courses")
	expectNone(t, body, "/course-pictures/")
	expectInOrder(t, courseCard(t, body, "Go Basics"), "course-cover-drawn")
	_, body = s.get("/admin/courses/" + id + "/edit")
	expectNone(t, body, `name="remove_picture"`)
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored after removing, want 0", n)
	}
}

func TestACoursePictureMustBeAPictureOfAtMost20MB(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	fields := map[string]string{"title": "Go Basics", "description": "Learn Go."}
	res, body := s.saveCourse("/admin/courses", fields, pdfFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "JPEG, PNG or WebP")
	big := append(append([]byte{}, pngFile...), bytes.Repeat([]byte{0}, 20<<20)...)
	res, body = s.saveCourse("/admin/courses", fields, big)
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "Pictures are limited to 20 MB")

	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	res, body = s.saveCourse("/admin/courses/"+id, fields, pdfFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "JPEG, PNG or WebP")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
}

func TestDeletingACourseRemovesItsPicture(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)
	res, body := s.post("/admin/courses/"+s.onlyCourseID()+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := s.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
}

func TestTheCourseListHasNoHeadingSkip(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.courseWithContent()
	s.logOut()
	_, body := s.get("/courses")
	expectHeadingsInOrder(t, "/courses", body)
}

// courseCard is the card of the Course titled title on the page body, or fails the test.
func courseCard(t *testing.T, body, title string) string {
	t.Helper()
	for _, c := range strings.Split(body, `<li class="catalogue-card`)[1:] {
		if strings.Contains(c, ">"+title+"</a>") {
			return c
		}
	}
	t.Fatalf("no Course card titled %q: %s", title, body)
	return ""
}

func TestACourseCardShowsItsLessonCountExamAndCertificate(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent() // "Go Basics": one Lesson and a Multiple-choice Exam
	s.addTextLesson(id, "Functions", "Write a function.", "Use func.", false)
	s.addTextLesson(id, "Generics", "Later.", "Not yet.", true)
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "2")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	s.createCourse("Reading List", "Books to read.", "")
	workshopID := s.copiedCourseID(s.courseIDByTitle("Reading List"), "Acme Workshop", "2027-03-01", "2027-03-03", "3")
	s.post("/admin/courses/"+workshopID+"/publish", nil)

	s.logOut()
	_, body := s.get("/courses")
	selfPaced := courseCard(t, body, "Go Basics")
	expectInOrder(t, selfPaced, "2 Lessons", "Exam", "Certificate")
	expectNone(t, selfPaced, "PCC")
	live := courseCard(t, body, "Go for Acme")
	expectInOrder(t, live, "2 Lessons", "Exam", "/static/pcc-foundation", "PCC Level: Foundation")
	empty := courseCard(t, body, "Reading List")
	expectNone(t, empty, "Lesson", "Exam", "Certificate", "PCC")
	// A Live Course run on dates with no Lessons shows its dates and Level, not "0 Lessons".
	workshop := courseCard(t, body, "Acme Workshop")
	expectInOrder(t, workshop, "PCC Level:")
	expectNone(t, workshop, "Lesson")
}

// On a phone the side card leads with the call to action: Join or Register comes before the Course picture, which
// the card repeats for phones, and before a Learner's Certificate art. Desktop keeps the art at the top by CSS alone.
func TestTheCoursePagesSideCardPutsJoinOrRegisterBeforeThePictureAndTheCertificateArt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.saveCourse("/admin/courses", map[string]string{"title": "Go Basics", "description": "Learn Go."}, pngFile)
	id := s.onlyCourseID()
	s.logOut()

	card := func(body string) string {
		_, card, _ := strings.Cut(body, `<div class="cp-card">`)
		card, _, _ = strings.Cut(card, "</aside>")
		return card
	}
	_, body := s.get("/courses/" + id)
	expectInOrder(t, card(body), "Register to join", `class="cp-card-cover"`, `<img class="course-cover" src="/course-pictures/`)
	expectNone(t, card(body), "cp-cert") // no Certificate, even a sample, for Visitors
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	_, body = s.getAsking("/courses/"+id, "ms")
	expectInOrder(t, card(body), "Sertai Kursus ini", `class="cp-card-cover"`, `class="cp-cert"`)
}
