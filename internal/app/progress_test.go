package app_test

import (
	"net/http"
	"strings"
	"testing"
)

// markLesson posts the logged-in Learner's mark (done true) or unmark (done false) for a Lesson.
func (s *site) markLesson(courseID, lessonID string, done bool) (*http.Response, string) {
	action := "/incomplete"
	if done {
		action = "/complete"
	}
	return s.post("/courses/"+courseID+"/lessons/"+lessonID+action, nil)
}

func TestEnrolledLearnerMarksALessonCompleteAndUndoesIt(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.addTextLesson(id, "Functions", "", "Use func.", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	variables := s.lessonIDByTitle("Variables")

	res, body := s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 2 Lessons complete")

	res, body = s.markLesson(id, variables, true)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 2 Lessons complete")

	res, body = s.markLesson(id, variables, false)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 2 Lessons complete")
}

func TestCoursePageOffersMarkButtonForEachPublishedLesson(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	variables := s.lessonIDByTitle("Variables")
	completeAction := `action="/courses/` + id + `/lessons/` + variables + `/complete"`
	undoAction := `action="/courses/` + id + `/lessons/` + variables + `/incomplete"`

	res, body := s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, completeAction)

	s.markLesson(id, variables, true)
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, undoAction)
	if strings.Contains(body, completeAction) {
		t.Fatalf("completed Lesson still offers Mark complete: %s", body)
	}
}

func TestDraftLessonIsNotCountedAndCannotBeMarked(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.addTextLesson(id, "Secret", "", "Not yet.", true)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	secret := s.lessonIDByTitle("Secret")

	res, body := s.markLesson(id, secret, true)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 1 Lessons complete")

	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 1 Lessons complete")
}

func TestAddingOrPublishingLessonsLowersProgressAndRemovingOneStopsCounting(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := admin.onlyCourseID()
	admin.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	ada.markLesson(id, admin.lessonIDByTitle("Variables"), true)

	admin.addTextLesson(id, "Functions", "", "Use func.", false)
	res, body := ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 2 Lessons complete")

	admin.addTextLesson(id, "Methods", "", "Later.", true)
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 2 Lessons complete")

	admin.post("/admin/lessons/"+admin.lessonIDByTitle("Methods")+"/publish", nil)
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 3 Lessons complete")

	admin.post("/admin/lessons/"+admin.lessonIDByTitle("Variables")+"/delete", nil)
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 2 Lessons complete")
}

func TestOnlyEnrolledLearnersCanMarkLessons(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	variables := s.lessonIDByTitle("Variables")
	s.logOut()

	res, body := s.markLesson(id, variables, true)
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/login" {
		t.Fatalf("Visitor got %d to %q, want 303 to /login", res.StatusCode, loc)
	}
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.markLesson(id, variables, true)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to mark its Lessons complete.")
	s.post("/courses/"+id+"/join", nil)
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 1 Lessons complete")
}

func TestLeavingAndRejoiningKeepsCompletions(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	s.addTextLesson(id, "Functions", "", "Use func.", false)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	s.leaveCourse(id)
	res, body := s.markLesson(id, s.lessonIDByTitle("Functions"), true)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to mark its Lessons complete.")
	s.post("/courses/"+id+"/join", nil)
	res, body = s.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 2 Lessons complete")
}

func TestLearnerWhoLeftOrWasRemovedCannotOpenLessons(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := admin.onlyCourseID()
	admin.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	lessonPath := "/courses/" + id + "/lessons/" + admin.lessonIDByTitle("Variables")
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	res, body := ada.get(lessonPath)
	expectStatus(t, res, body, http.StatusOK, "<h1>Variables</h1>")

	ada.leaveCourse(id)
	res, body = ada.get(lessonPath)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to open its Lessons.")

	ada.post("/courses/"+id+"/join", nil)
	res, body = ada.get(lessonPath)
	expectStatus(t, res, body, http.StatusOK, "<h1>Variables</h1>")
	_, body = admin.get("/admin/courses/" + id)
	m := removeLearnerLink.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("Course page has no remove button for Ada: %s", body)
	}
	admin.post("/admin/courses/"+id+"/learners/"+m[1]+"/remove", nil)
	res, body = ada.get(lessonPath)
	expectStatus(t, res, body, http.StatusForbidden, "Join this Course to open its Lessons.")
}

func TestCompletionsBelongToEachLearner(t *testing.T) {
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := admin.onlyCourseID()
	admin.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	ada := admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	ada.markLesson(id, admin.lessonIDByTitle("Variables"), true)

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	bob.post("/courses/"+id+"/join", nil)
	res, body := bob.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 0 of 1 Lessons complete")
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 1 of 1 Lessons complete")
}
