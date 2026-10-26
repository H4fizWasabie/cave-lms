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
	form := url.Values{"title": {title}, "objective": {objective}, "body": {body}}
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
	for _, banned := range []string{"<script>alert", "javascript:"} {
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
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/login?next="+url.QueryEscape(lessonPath) {
		t.Fatalf("Visitor got %d to %q, want 303 to /login with next", res.StatusCode, loc)
	}
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get(lessonPath)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to open its Lessons.")
	expectStatus(t, res, body, http.StatusForbidden, "/courses/"+id+"/join")
	if strings.Contains(body, "Variables") {
		t.Fatalf("non-member saw the Lesson title: %s", body)
	}
}

func TestLessonShowsItsObjectivesAsAListAtTheTop(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	itemID := strings.TrimPrefix(s.uploadAs("Diagram", "everyone", pngFile), "/gallery/")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	res, body := s.post("/admin/courses/"+id+"/lessons", url.Values{"title": {"Variables"}, "gallery_item_id": {itemID}, "body": {"Body text."},
		"objective": {"Declare a variable with :=.\r\n\r\n  Name a constant.  \n"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)

	res, body = s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Variables"))
	expectStatus(t, res, body, http.StatusOK, "<li>Declare a variable with :=.</li><li>Name a constant.</li></ul>")
	objectives, media, text := strings.Index(body, "Name a constant."), strings.Index(body, "/gallery/"+itemID+"/file"), strings.Index(body, "Body text.")
	if !(objectives < media && media < text) {
		t.Fatalf("want the Objectives, then the Gallery item, then the text: %s", body)
	}
	_, body = s.get("/courses/" + id)
	expectAll(t, body, "Declare a variable with :=.</li>", "Name a constant.</li>")
}

func TestLessonTakesUpToFiveObjectivesOfOneSentenceEach(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	const refused = "Write at most five Objectives, one sentence of at most 200 characters per line."

	res, body := s.addTextLesson(id, "Six", "One.\nTwo.\nThree.\nFour.\nFive.\nSix.", "Text.", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, refused)
	expectAll(t, body, "Six.</textarea>")
	res, body = s.addTextLesson(id, "Long", "Short.\n"+strings.Repeat("a", 201), "Text.", false)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, refused)
	res, body = s.addTextLesson(id, "Five", "One.\n\nTwo.\nThree.\n\nFour.\n"+strings.Repeat("a", 200), "Text.", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.addTextLesson(id, "None", "", "Text.", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
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

	res, body = s.post("/admin/lessons/"+lessonID+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	if strings.Contains(body, "Variables and constants") {
		t.Fatal("removed Lesson still listed")
	}
}

// confirming is the form a confirmation page posts: without it, Remove lesson and Revoke certificate do nothing.
var confirming = url.Values{"confirm": {"yes"}}

func TestAdminRemovesALessonOnlyAfterConfirming(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	lessonID := s.lessonIDByTitle("Variables")
	confirmPage := "/admin/lessons/" + lessonID + "/delete"

	_, body := s.get("/admin/courses/" + id + "/lessons")
	expectAll(t, body, `href="`+confirmPage+`"`)
	res, body := s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "Remove “Variables” from “Go Basics”? This cannot be undone.")

	res, body = s.post(confirmPage, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != confirmPage {
		t.Fatalf("a post without confirming went to %q, want the confirmation page", loc)
	}
	_, body = s.get("/admin/courses/" + id + "/lessons")
	expectAll(t, body, "Variables")

	res, body = s.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	expectNone(t, body, "Variables")

	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.addTextLesson(id, "Loops", "", "Use `for`.", false)
	res, body = ada.post("/admin/lessons/"+s.lessonIDByTitle("Loops")+"/delete", confirming)
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	expectAll(t, body, "Loops")
}

// addItemLesson posts the Lesson form as the logged-in Admin with one Gallery item.
func (s *site) addItemLesson(courseID, itemID, title string, draft bool) (*http.Response, string) {
	form := url.Values{"title": {title}, "gallery_item_id": {itemID}}
	if draft {
		form.Set("draft", "on")
	}
	return s.post("/admin/courses/"+courseID+"/lessons", form)
}

func TestLessonKeepsTextWrittenUnderItsGalleryItem(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	itemID := strings.TrimPrefix(s.uploadAs("Diagram", "everyone", pngFile), "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()

	res, body := s.post("/admin/courses/"+id+"/lessons", url.Values{"title": {"Diagram lesson"}, "gallery_item_id": {itemID}, "body": {"Notes under the diagram."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Diagram lesson"))
	expectStatus(t, res, body, http.StatusOK, `src="/gallery/`+itemID+`/file"`)
	if media, notes := strings.Index(body, "/gallery/"+itemID+"/file"), strings.Index(body, "Notes under the diagram."); notes < media {
		t.Fatalf("want the Gallery item, then the text: %s", body)
	}
}

func TestLessonWithNeitherTextNorAGalleryItemIsRefused(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()

	res, body := s.post("/admin/courses/"+id+"/lessons", url.Values{"title": {"Empty"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Write the text of the Lesson or choose a Gallery item.")
	s.addTextLesson(id, "Variables", "", "Old text.", false)
	res, body = s.post("/admin/lessons/"+s.lessonIDByTitle("Variables"), url.Values{"title": {"Variables"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Write the text of the Lesson or choose a Gallery item.")
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

	res, body := s.post("/admin/courses/"+id+"/delete", confirming)
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

	res, body := s.post("/admin/gallery/"+itemID+"/delete", confirming)
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

func TestEveryoneSeesTheOrderedPublishedLessonTitlesOnTheCoursePage(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Alpha lesson", "", "a", false)
	s.addTextLesson(id, "Secret lesson", "", "s", true)
	s.addTextLesson(id, "Beta lesson", "", "b", false)
	s.logOut()

	check := func(who string) {
		t.Helper()
		res, body := s.get("/courses/" + id)
		expectStatus(t, res, body, http.StatusOK, "Alpha lesson")
		if a, b := strings.Index(body, "Alpha lesson"), strings.Index(body, "Beta lesson"); b < a {
			t.Fatalf("%s: Course page does not list the Lessons in order: %s", who, body)
		}
		if strings.Contains(body, "Secret lesson") || strings.Contains(body, "/lessons/") {
			t.Fatalf("%s: Course page shows a Draft or links into a Lesson: %s", who, body)
		}
	}
	check("a Visitor")
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	check("a non-member")
	res, body := s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Alpha lesson"))
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to open its Lessons.")
}

func TestVideoBecomesCourseMembersOnlyOnlyOnceItsLessonIsPublished(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	video := s.uploadAs("Lecture", "everyone", mp4File)
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addItemLesson(id, strings.TrimPrefix(video, "/gallery/"), "Lecture lesson", true)
	visitor := s.second()

	res, body := visitor.get(video)
	expectStatus(t, res, body, http.StatusOK, "Lecture")

	s.post("/admin/lessons/"+s.lessonIDByTitle("Lecture lesson")+"/publish", nil)
	res, body = visitor.get(video)
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/login" {
		t.Fatalf("Visitor got %d to %q after the Lesson was published, want 303 to /login", res.StatusCode, loc)
	}
}

func TestLessonFormSaysWhenAVideoBecomesCourseMembersOnly(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	res, body := s.get("/admin/courses/" + s.onlyCourseID() + "/lessons/new")
	expectStatus(t, res, body, http.StatusOK, "becomes Course members only once a published Lesson uses it")
}

func TestTheNewAndEditLessonFormsShowEveryFieldOnceWithNoContentKind(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Text.", false)
	for _, path := range []string{"/admin/courses/" + id + "/lessons/new", "/admin/lessons/" + s.lessonIDByTitle("Variables") + "/edit"} {
		res, body := s.get(path)
		expectStatus(t, res, body, http.StatusOK, "")
		if strings.Contains(body, `name="kind"`) || strings.Contains(body, "Lesson content") {
			t.Errorf("%s still asks for the Lesson content kind: %s", path, body)
		}
		for _, field := range []string{`name="title"`, `name="objective"`, `name="gallery_item_id"`, `name="body"`} {
			if n := strings.Count(body, field); n != 1 {
				t.Errorf("%s shows %s %d times, want once", path, field, n)
			}
		}
	}
}

func TestAdminAddsAndRemovesTheTextOrGalleryItemOfASavedLesson(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	itemID := strings.TrimPrefix(s.uploadAs("Diagram", "everyone", pngFile), "/gallery/")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addItemLesson(id, itemID, "Diagram lesson", false)
	lessonID := s.lessonIDByTitle("Diagram lesson")
	lessonPage := "/courses/" + id + "/lessons/" + lessonID
	edit := func(form url.Values) {
		t.Helper()
		form.Set("title", "Diagram lesson")
		res, body := s.post("/admin/lessons/"+lessonID, form)
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}

	edit(url.Values{"gallery_item_id": {itemID}, "body": {"Added notes."}})
	_, body := s.get(lessonPage)
	expectAll(t, body, "/gallery/"+itemID+"/file", "Added notes.")

	edit(url.Values{"body": {"Added notes."}})
	_, body = s.get(lessonPage)
	expectAll(t, body, "Added notes.")
	if strings.Contains(body, "/gallery/"+itemID+"/file") {
		t.Fatalf("removed Gallery item still shown: %s", body)
	}

	edit(url.Values{"gallery_item_id": {itemID}})
	_, body = s.get(lessonPage)
	expectAll(t, body, "/gallery/"+itemID+"/file")
	if strings.Contains(body, "Added notes.") {
		t.Fatalf("removed text still shown: %s", body)
	}
}
