package app_test

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// addTextLesson posts the Lesson form as the logged-in Admin with a text Lesson.
func (s *site) addTextLesson(courseID, title, objective, body string, draft bool) (*http.Response, string) {
	form := url.Values{"kind": {"text"}, "title": {title}, "objective": {objective}, "body": {body}}
	if draft {
		form.Set("draft", "on")
	}
	return s.post("/admin/courses/"+courseID+"/lessons", form)
}

func TestAdminAddsATextLessonToACourse(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()

	res, body := s.addTextLesson(id, "Variables", "", "Use `:=` inside functions.", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = s.get("/admin/courses/" + id + "/lessons")
	expectStatus(t, res, body, http.StatusOK, "Variables")
}

func TestEnrolledLearnerOpensLessonsInAnyOrder(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "First lesson", "", "Hello.", false)
	s.addTextLesson(id, "Second lesson", "", "World.", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)

	_, body := s.get("/courses/" + id)
	first, second := strings.Index(body, "First lesson"), strings.Index(body, "Second lesson")
	if first < 0 || second < first {
		t.Fatalf("Course page does not list the Lessons in order: %s", body)
	}
	lessonID := s.lessonIDByTitle("Second lesson")
	res, body := s.get("/courses/" + id + "/lessons/" + lessonID)
	expectStatus(t, res, body, http.StatusOK, "<h1>Second lesson</h1>")
	expectStatus(t, res, body, http.StatusOK, "<p>World.</p>")
}

func TestLessonTextIsSanitisedMarkdown(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Markup", "", "**Bold** <script>alert(1)</script> [bad](javascript:alert(1))", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)

	res, body := s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Markup"))
	expectStatus(t, res, body, http.StatusOK, "<strong>Bold</strong>")
	for _, banned := range []string{"<script", "javascript:"} {
		if strings.Contains(body, banned) {
			t.Fatalf("Lesson page contains %q: %s", banned, body)
		}
	}
}

// lessonIDByTitle reads the ID of the Lesson titled title from the database.
func (s *site) lessonIDByTitle(title string) string {
	s.t.Helper()
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM lessons WHERE title = $1`, title).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return strconv.FormatInt(id, 10)
}

func TestNonMemberIsAskedToJoinBeforeOpeningALesson(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	lessonPath := "/courses/" + id + "/lessons/" + s.lessonIDByTitle("Variables")
	s.logOut()

	res, body := s.get(lessonPath)
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/login" {
		t.Fatalf("Visitor got %d to %q, want 303 to /login", res.StatusCode, loc)
	}
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get(lessonPath)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to open its Lessons.")
	expectStatus(t, res, body, http.StatusForbidden, "/courses/"+id+"/join")
	if strings.Contains(body, "Variables") {
		t.Fatalf("non-member saw the Lesson title: %s", body)
	}
}

func TestLessonShowsItsObjectiveAtTheTop(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "Declare a variable with :=.", "Body text.", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)

	res, body := s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Variables"))
	expectStatus(t, res, body, http.StatusOK, "<strong>Objective:</strong> Declare a variable with :=.")
}

func TestAdminMovesLessonsUpAndDown(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Alpha", "", "a", false)
	s.addTextLesson(id, "Beta", "", "b", false)
	s.addTextLesson(id, "Gamma", "", "c", false)

	order := func() []string {
		_, body := s.get("/admin/courses/" + id + "/lessons")
		titles := []string{"Alpha", "Beta", "Gamma"}
		sort.Slice(titles, func(i, j int) bool { return strings.Index(body, titles[i]) < strings.Index(body, titles[j]) })
		return titles
	}
	want := func(step string, got []string, titles ...string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(titles, ",") {
			t.Fatalf("%s: order %v, want %v", step, got, titles)
		}
	}

	s.post("/admin/lessons/"+s.lessonIDByTitle("Gamma")+"/up", nil)
	want("Gamma up", order(), "Alpha", "Gamma", "Beta")
	s.post("/admin/lessons/"+s.lessonIDByTitle("Alpha")+"/up", nil)
	want("Alpha up at the top", order(), "Alpha", "Gamma", "Beta")
	s.post("/admin/lessons/"+s.lessonIDByTitle("Alpha")+"/down", nil)
	want("Alpha down", order(), "Gamma", "Alpha", "Beta")
	s.post("/admin/lessons/"+s.lessonIDByTitle("Beta")+"/down", nil)
	want("Beta down at the bottom", order(), "Gamma", "Alpha", "Beta")
}

func TestAdminEditsAndRemovesALesson(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Old text.", false)
	lessonID := s.lessonIDByTitle("Variables")

	res, body := s.post("/admin/lessons/"+lessonID, url.Values{"title": {"Variables and constants"}, "objective": {""}, "body": {"New text."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + id + "/lessons")
	expectStatus(t, res, body, http.StatusOK, "Variables and constants")

	res, body = s.post("/admin/lessons/"+lessonID+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	if strings.Contains(body, "Variables and constants") {
		t.Fatal("removed Lesson still listed")
	}
}

// addItemLesson posts the Lesson form as the logged-in Admin with one Gallery item.
func (s *site) addItemLesson(courseID, itemID, title string, draft bool) (*http.Response, string) {
	form := url.Values{"kind": {"item"}, "title": {title}, "gallery_item_id": {itemID}}
	if draft {
		form.Set("draft", "on")
	}
	return s.post("/admin/courses/"+courseID+"/lessons", form)
}

func TestCourseMembersItemIsVisibleToMembersOfAnyCourseUsingIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Diagram", "course_members", pngFile)
	itemID := strings.TrimPrefix(location, "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	s.createCourse("Go Advanced", "Learn more Go.", "")
	basics, advanced := s.courseIDByTitle("Go Basics"), s.courseIDByTitle("Go Advanced")
	for _, c := range []string{basics, advanced} {
		res, body := s.addItemLesson(c, itemID, "Diagram lesson", false)
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	s.logOut()

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+basics+"/join", nil)
	res, body := s.get(location + "/file")
	expectStatus(t, res, body, http.StatusOK, "")
	s.logOut()

	s.loginAsNewLearner("Grace", "grace@example.com", "correct horse battery")
	s.post("/courses/"+advanced+"/join", nil)
	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusOK, "")
	s.logOut()

	s.loginAsNewLearner("Lin", "lin@example.com", "correct horse battery")
	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
	expectStatus(t, res, body, http.StatusForbidden, "Join Go Basics to see it.")
	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusForbidden, "Join Go Advanced to see it.")
	s.logOut()

	res, body = s.get(location + "/file")
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/login" {
		t.Fatalf("Visitor got %d to %q, want 303 to /login", res.StatusCode, loc)
	}
}

func TestDraftLessonIsHiddenFromLearnersEverywhere(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Secret diagram", "course_members", pngFile)
	itemID := strings.TrimPrefix(location, "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Secret lesson", "", "Shh.", true)
	s.addItemLesson(id, itemID, "Secret diagram lesson", true)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)

	_, body := s.get("/courses/" + id)
	for _, title := range []string{"Secret lesson", "Secret diagram lesson"} {
		if strings.Contains(body, title) {
			t.Fatalf("Course page lists the Draft %q: %s", title, body)
		}
	}
	res, body := s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Secret lesson"))
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Secret diagram lesson"))
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get(location + "/file")
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
	res, body = s.get(location)
	expectStatus(t, res, body, http.StatusForbidden, "only for members of its Courses")
}

func TestAdminPublishesAndUnpublishesLessons(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "One", "", "1", true)
	s.addTextLesson(id, "Two", "", "2", true)
	s.addTextLesson(id, "Three", "", "3", true)
	s.post("/admin/lessons/"+s.lessonIDByTitle("One")+"/publish", nil)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	_, body := s.get("/courses/" + id)
	if !strings.Contains(body, "One") || strings.Contains(body, "Two") {
		t.Fatalf("after publishing One, Learner page should list One and not Two: %s", body)
	}
	s.logOut()

	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	res, body := s.post("/admin/courses/"+id+"/lessons/publish-all", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()
	s.loginAsNewLearner("Grace", "grace@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	_, body = s.get("/courses/" + id)
	if !strings.Contains(body, "Two") || !strings.Contains(body, "Three") {
		t.Fatalf("after publish all, Learner page should list every Lesson: %s", body)
	}
	s.logOut()

	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.post("/admin/lessons/"+s.lessonIDByTitle("Two")+"/unpublish", nil)
	s.logOut()
	s.loginAsNewLearner("Lin", "lin@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	res, body = s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Two"))
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestCourseWithLessonsCannotBeDeleted(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", true)

	res, body := s.post("/admin/courses/"+id+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "This Course cannot be deleted while it has Lessons, Learners, Exam attempts or Certificates.")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "<h1>Go Basics</h1>")
}

func TestGalleryItemUsedByALessonCannotBeDeleted(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Diagram", "everyone", pngFile)
	itemID := strings.TrimPrefix(location, "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addItemLesson(id, itemID, "Diagram lesson", false)

	res, body := s.post("/admin/gallery/"+itemID+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "This item cannot be deleted because a Lesson uses it. Remove the Lesson first.")
	if n := s.countGalleryItems(); n != 1 {
		t.Fatalf("Gallery items after refused delete = %d, want 1", n)
	}
	if n := s.storedFiles(); n != 1 {
		t.Fatalf("stored files after refused delete = %d, want 1", n)
	}
}

func TestVideoAndEbookUsedInALessonDefaultToCourseMembersOnly(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	video := strings.TrimPrefix(s.uploadAs("Lecture", "everyone", mp4File), "/gallery/")
	ebook := strings.TrimPrefix(s.uploadAs("Notes", "everyone", pdfFile), "/gallery/")
	picture := strings.TrimPrefix(s.uploadAs("Chart", "everyone", pngFile), "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addItemLesson(id, video, "Lecture lesson", false)
	s.addItemLesson(id, ebook, "Notes lesson", false)
	s.addItemLesson(id, picture, "Chart lesson", false)

	visibility := func(itemID string) string {
		var v string
		if err := s.db.QueryRow(`SELECT visibility FROM gallery_items WHERE id = $1`, itemID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := visibility(video); got != "course_members" {
		t.Fatalf("video visibility = %q, want course_members", got)
	}
	if got := visibility(ebook); got != "course_members" {
		t.Fatalf("ebook visibility = %q, want course_members", got)
	}
	if got := visibility(picture); got != "everyone" {
		t.Fatalf("picture visibility = %q, want everyone (unchanged)", got)
	}
}

func TestLessonAdminPagesRefuseLearners(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	lessonID := s.lessonIDByTitle("Variables")
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	res, body := s.get("/admin/courses/" + id + "/lessons")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.addTextLesson(id, "Sneaky", "", "No.", false)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.post("/admin/lessons/"+lessonID+"/up", nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.post("/admin/lessons/"+lessonID+"/unpublish", nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
}

func TestAdminCourseListLinksToItsLessons(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()

	res, body := s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, `href="/admin/courses/`+id+`/lessons"`)
}
