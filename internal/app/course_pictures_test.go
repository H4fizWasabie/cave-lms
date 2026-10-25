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

func TestACourseWithoutAPictureShowsADrawnCoverWithItsTitleAndLevel(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "3")
	s.post("/admin/courses/"+copyID+"/publish", nil)

	s.logOut()
	_, body := s.get("/courses")
	expectNone(t, body, "/course-pictures/")
	expectInOrder(t, courseCard(t, body, "Go Basics"), `class="course-cover course-cover-drawn`, "Go Basics", "Self-paced online", "</div>", ">Go Basics</a>")
	expectInOrder(t, courseCard(t, body, "Go for Acme"), `class="course-cover course-cover-drawn`, "Go for Acme", "PCC Practitioner", "</div>", ">Go for Acme</a>")

	_, body = s.getAsking("/courses", "ms")
	expectInOrder(t, courseCard(t, body, "Go Basics"), "course-cover-drawn", "Dalam talian, kendiri")
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

var headingTag = regexp.MustCompile(`<h([1-6])[ >]`)

// expectNoHeadingSkip fails when a heading in body is more than one level below the heading before it (h1 then h3).
func expectNoHeadingSkip(t *testing.T, body string) {
	t.Helper()
	prev := 0
	for _, m := range headingTag.FindAllStringSubmatch(body, -1) {
		level := int(m[1][0] - '0')
		if level > prev+1 {
			t.Fatalf("an h%d follows an h%d; body:\n%s", level, prev, body)
		}
		prev = level
	}
}

func TestTheCourseListHasNoHeadingSkip(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.courseWithContent()
	s.logOut()
	_, body := s.get("/courses")
	expectNoHeadingSkip(t, content(body))
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

	s.logOut()
	_, body := s.get("/courses")
	selfPaced := courseCard(t, body, "Go Basics")
	expectInOrder(t, selfPaced, "2 Lessons", "Exam", "Certificate")
	expectNone(t, selfPaced, "PCC")
	live := courseCard(t, body, "Go for Acme")
	expectInOrder(t, live, "2 Lessons", "Exam", "/static/pcc-foundation", "PCC Level: Foundation")
	empty := courseCard(t, body, "Reading List")
	expectInOrder(t, empty, "0 Lessons")
	expectNone(t, empty, "Exam", "Certificate", "PCC")
}
