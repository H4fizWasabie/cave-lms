package app_test

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var lessonImageSrc = regexp.MustCompile(`src="(/lesson-images/[^"]+)"`)

// imageLessonCourse is the Admin making "Go Basics" and adding the Lesson "Variables" with text and a PNG Lesson
// image, published or as a Draft. It returns the Admin's site and the Course and Lesson IDs.
func imageLessonCourse(t *testing.T, draft bool) (admin *site, id, lessonID string) {
	t.Helper()
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go.", "")
	id = admin.onlyCourseID()
	fields := map[string]string{"title": "Variables", "body": "Use `:=`.", "image_description": "A box labelled x"}
	if draft {
		fields["draft"] = "on"
	}
	res, body := admin.saveProgramme("/admin/courses/"+id+"/lessons", fields, pngFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return admin, id, admin.lessonIDByTitle("Variables")
}

func TestAnEnrolledLearnerSeesTheLessonImageAboveTheTextAndItIsNeverInTheGallery(t *testing.T) {
	admin, id, lessonID := imageLessonCourse(t, false)
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)

	_, body := ada.get("/courses/" + id + "/lessons/" + lessonID)
	expectInOrder(t, body, "<h1>Variables</h1>", `<img src="/lesson-images/`, `alt="A box labelled x"`, `class="prose"`)
	src := lessonImageSrc.FindStringSubmatch(body)[1]
	res, picture := ada.get(src)
	expectStatus(t, res, picture, http.StatusOK, "")
	if picture != string(pngFile) || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("served Lesson image differs from the upload (%s, %s)", res.Header.Get("Content-Type"), res.Header.Get("X-Content-Type-Options"))
	}
	res, picture = admin.get(src)
	expectStatus(t, res, picture, http.StatusOK, "")

	for _, path := range []string{"/gallery", "/admin/gallery"} {
		_, body = admin.get(path)
		expectNone(t, body, "A box labelled x", "/lesson-images/")
	}
}

func TestTheLessonImageRouteRefusesNonMembersVisitorsAndADraftForALearner(t *testing.T) {
	admin, id, lessonID := imageLessonCourse(t, false)
	_, body := admin.get("/admin/lessons/" + lessonID + "/edit")
	src := lessonImageSrc.FindStringSubmatch(body)[1]

	visitor := admin.second()
	res, _ := visitor.get(src)
	if res.StatusCode == http.StatusOK {
		t.Fatalf("a logged-out visitor got the Lesson image")
	}
	bea := admin.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	res, body = bea.get(src)
	expectStatus(t, res, body, http.StatusNotFound, "")

	bea.post("/courses/"+id+"/join", nil)
	res, body = bea.get(src)
	expectStatus(t, res, body, http.StatusOK, "")
	admin.post("/admin/lessons/"+lessonID+"/unpublish", nil)
	res, body = bea.get(src)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = admin.get(src)
	expectStatus(t, res, body, http.StatusOK, "")

	// The route serves Lesson images only, never a Gallery file by its key.
	admin.uploadAs("Poster", "everyone", pngFile)
	var key string
	if err := admin.db.QueryRow(`SELECT storage_key FROM gallery_items`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	res, body = admin.get("/lesson-images/" + key)
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestAdminReplacesThenRemovesALessonImageOnTheEditForm(t *testing.T) {
	admin, _, lessonID := imageLessonCourse(t, false)
	edit := "/admin/lessons/" + lessonID + "/edit"
	_, body := admin.get(edit)
	old := lessonImageSrc.FindStringSubmatch(body)[1]
	expectInOrder(t, body, `enctype="multipart/form-data"`, `name="body"`, `src="`+old+`"`, `name="picture"`, `name="remove_image"`, `value="A box labelled x"`)

	fields := map[string]string{"title": "Variables", "body": "Use `:=`.", "image_description": "A box labelled y"}
	res, body := admin.saveProgramme("/admin/lessons/"+lessonID, fields, jpegFile)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = admin.get(edit)
	replaced := lessonImageSrc.FindStringSubmatch(body)[1]
	expectAll(t, body, `value="A box labelled y"`)
	if _, picture := admin.get(replaced); picture != string(jpegFile) {
		t.Fatalf("the Lesson shows the old image after replacing it")
	}
	if res, _ := admin.get(old); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET old Lesson image = %d, want 404", res.StatusCode)
	}
	if n := admin.storedFiles(); n != 1 {
		t.Fatalf("%d files stored after replacing, want 1", n)
	}

	// Saving without a file keeps the image.
	admin.saveProgramme("/admin/lessons/"+lessonID, fields, nil)
	if _, body = admin.get(edit); lessonImageSrc.FindStringSubmatch(body)[1] != replaced {
		t.Fatalf("saving without a file changed the Lesson image")
	}

	fields["remove_image"] = "on"
	res, body = admin.saveProgramme("/admin/lessons/"+lessonID, fields, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = admin.get(edit)
	expectNone(t, body, "/lesson-images/", `name="remove_image"`)
	if n := admin.storedFiles(); n != 0 {
		t.Fatalf("%d files stored after removing, want 0", n)
	}
}

func TestDeletingALessonDeletesItsLessonImage(t *testing.T) {
	admin, _, lessonID := imageLessonCourse(t, false)
	res, body := admin.post("/admin/lessons/"+lessonID+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := admin.storedFiles(); n != 0 {
		t.Fatalf("%d files stored, want 0", n)
	}
}

func TestALessonImageIsRefusedWhenWrongTooBigUndescribedWithAGalleryItemOrWithoutText(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go.", "")
	id := admin.onlyCourseID()
	item := strings.TrimPrefix(admin.uploadAs("Talk", "everyone", mp4File), "/gallery/")
	files := admin.storedFiles()
	ok := map[string]string{"title": "Variables", "body": "Use `:=`.", "image_description": "A box"}
	with := func(k, v string) map[string]string {
		f := map[string]string{}
		for key, val := range ok {
			f[key] = val
		}
		f[k] = v
		return f
	}
	big := append(append([]byte{}, pngFile...), bytes.Repeat([]byte{0}, 5<<20)...)
	for _, c := range []struct {
		name   string
		fields map[string]string
		file   []byte
		want   string
	}{
		{"wrong type", ok, pdfFile, "JPEG, PNG or WebP"},
		{"too big", ok, big, "Lesson images are limited to 5 MB"},
		{"no description", with("image_description", " "), pngFile, "Describe the Lesson image"},
		{"with a Gallery item", with("gallery_item_id", item), pngFile, "either a Lesson image or a Gallery item"},
		{"without text", with("body", ""), pngFile, "A Lesson image needs the Lesson&#39;s text"},
	} {
		res, body := admin.saveProgramme("/admin/courses/"+id+"/lessons", c.fields, c.file)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	huge := append(append([]byte{}, pngFile...), bytes.Repeat([]byte{0}, 20<<20)...)
	res, body := admin.saveProgramme("/admin/courses/"+id+"/lessons", ok, huge)
	expectStatus(t, res, body, http.StatusRequestEntityTooLarge, "Lesson images are limited to 5 MB")
	if n := admin.storedFiles(); n != files {
		t.Fatalf("%d files stored, want %d", n, files)
	}
	_, body = admin.get("/admin/courses/" + id + "/lessons")
	expectNone(t, body, "Variables")

	// Editing refuses them too, and keeps the Lesson as it was.
	admin.addTextLesson(id, "Loops", "", "Use for.", false)
	lessonID := admin.lessonIDByTitle("Loops")
	res, body = admin.saveProgramme("/admin/lessons/"+lessonID, map[string]string{"title": "Loops", "body": "", "gallery_item_id": item, "image_description": "A box"}, pngFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "either a Lesson image or a Gallery item")
	if n := admin.storedFiles(); n != files {
		t.Fatalf("%d files stored after a refused edit, want %d", n, files)
	}
}

func TestGenerateAfterPickingAPictureSaysToPickItAgain(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go.", "")
	id := admin.onlyCourseID()
	_, body := admin.get("/admin/courses/" + id + "/lessons/new")
	expectInOrder(t, body, `enctype="multipart/form-data"`, `data-busy`, `name="body"`, `name="picture"`, "pick the picture after generating")

	fields := map[string]string{"title": "Variables", "objective": "Declare a variable.", "image_description": "A box"}
	res, body := admin.saveProgramme("/admin/courses/"+id+"/lessons/generate", fields, pngFile)
	expectStatus(t, res, body, http.StatusOK, "Pick the picture again")
	expectAll(t, body, "AI lesson text", `value="A box"`)
	if n := admin.storedFiles(); n != 0 {
		t.Fatalf("Generate stored %d files, want 0", n)
	}
	res, body = admin.saveProgramme("/admin/courses/"+id+"/lessons/generate", fields, nil)
	expectStatus(t, res, body, http.StatusOK, "AI lesson text")
	expectNone(t, body, "Pick the picture again")
}
