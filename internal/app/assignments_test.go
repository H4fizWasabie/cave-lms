package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// addAssignment posts the Assignment form for Course id as the logged-in Admin.
func (s *site) addAssignment(courseID, title, instructions string) (*http.Response, string) {
	return s.post("/admin/courses/"+courseID+"/assignments", url.Values{"title": {title}, "instructions": {instructions}})
}

// assignmentIDByTitle reads the ID of the Assignment titled title from the database.
func (s *site) assignmentIDByTitle(title string) string {
	s.t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id::text FROM assignments WHERE title = $1`, title).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

// assignmentCourse makes "Go Basics" with Ada enrolled, and returns the Admin, Ada and the Course's ID.
func assignmentCourse(t *testing.T) (admin, ada *site, id string) {
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id = admin.onlyCourseID()
	ada = admin.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	return admin, ada, id
}

func TestAnAssignmentIsADraftUntilTheAdminPublishesIt(t *testing.T) {
	admin, ada, id := assignmentCourse(t)
	res, body := admin.addAssignment(id, "Write a prompt", "Write one prompt.\nUpload it as a PDF. <b>Not bold</b>")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	aid := admin.assignmentIDByTitle("Write a prompt")
	res, body = admin.get("/admin/courses/" + id + "/assignments")
	expectStatus(t, res, body, http.StatusOK, "Write a prompt")
	expectStatus(t, res, body, http.StatusOK, "Draft")

	_, body = ada.get("/courses/" + id)
	if strings.Contains(body, "Write a prompt") {
		t.Fatalf("a Draft Assignment shows on the Course page: %s", body)
	}
	if res, _ := ada.get("/courses/" + id + "/assignments/" + aid); res.StatusCode != http.StatusNotFound {
		t.Fatalf("a Draft Assignment opened for a Learner: %d", res.StatusCode)
	}

	res, body = admin.post("/admin/assignments/"+aid+"/publish", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `href="/courses/`+id+`/assignments/`+aid+`"`)
	res, body = ada.get("/courses/" + id + "/assignments/" + aid)
	expectStatus(t, res, body, http.StatusOK, "<h1>Write a prompt</h1>")
	expectStatus(t, res, body, http.StatusOK, "Write one prompt.\nUpload it as a PDF. &lt;b&gt;Not bold&lt;/b&gt;")
}

func TestOnlyEnrolledLearnersSeeAssignments(t *testing.T) {
	admin, _, id := assignmentCourse(t)
	admin.addAssignment(id, "Write a prompt", "Write one prompt.")
	aid := admin.assignmentIDByTitle("Write a prompt")
	admin.post("/admin/assignments/"+aid+"/publish", nil)
	path := "/courses/" + id + "/assignments/" + aid

	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	_, body := bea.get("/courses/" + id)
	if strings.Contains(body, "Write a prompt") {
		t.Fatalf("the Course page shows Assignments to a Learner who has not joined: %s", body)
	}
	res, body := bea.get(path)
	expectStatus(t, res, body, http.StatusForbidden, "")
	if strings.Contains(body, "Write one prompt.") {
		t.Fatalf("the Assignment shows to a Learner who has not joined: %s", body)
	}
	res, _ = admin.second().get(path)
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/login") {
		t.Fatalf("a Visitor got %d to %q, want sent to log in", res.StatusCode, res.Header.Get("Location"))
	}
	res, body = admin.get(path)
	expectStatus(t, res, body, http.StatusOK, "Write one prompt.")
}

func TestTheAdminEditsReordersAndDeletesAssignments(t *testing.T) {
	admin, ada, id := assignmentCourse(t)
	admin.addAssignment(id, "First", "One.")
	admin.addAssignment(id, "Second", "Two.")
	first, second := admin.assignmentIDByTitle("First"), admin.assignmentIDByTitle("Second")

	res, body := admin.post("/admin/assignments/"+first, url.Values{"title": {"Prompt one"}, "instructions": {"Changed."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = admin.post("/admin/assignments/"+first, url.Values{"title": {""}, "instructions": {"Changed."}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a title")

	admin.post("/admin/assignments/"+second+"/up", nil)
	_, body = admin.get("/admin/courses/" + id + "/assignments")
	if i, j := strings.Index(body, "Second"), strings.Index(body, "Prompt one"); i < 0 || j < i {
		t.Fatalf("Second did not move above Prompt one: %s", body)
	}

	res, body = admin.get("/admin/assignments/" + second + "/delete")
	expectStatus(t, res, body, http.StatusOK, "Learners&#39; Submissions to it are deleted with it")
	res, body = admin.post("/admin/assignments/"+second+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = admin.get("/admin/courses/" + id + "/assignments")
	if strings.Contains(body, "Second") {
		t.Fatalf("the deleted Assignment is still listed: %s", body)
	}

	if res, _ := ada.post("/admin/assignments/"+first+"/delete", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("a Learner deleted an Assignment: %d", res.StatusCode)
	}
}

func TestAssignmentPagesAreInMalay(t *testing.T) {
	admin, _, id := assignmentCourse(t)
	req, _ := http.NewRequest(http.MethodGet, admin.server.URL+"/admin/courses/"+id+"/assignments", nil)
	req.Header.Set("Accept-Language", "ms")
	res, body := admin.do(req)
	expectStatus(t, res, body, http.StatusOK, "Tugasan Baharu")
}

func TestDuplicatingACourseCopiesItsAssignments(t *testing.T) {
	admin, _, id := assignmentCourse(t)
	admin.addAssignment(id, "Write a prompt", "Write one prompt.")
	admin.addAssignment(id, "Still a draft", "Later.")
	admin.post("/admin/assignments/"+admin.assignmentIDByTitle("Write a prompt")+"/publish", nil)

	copyID := admin.copiedCourseID(id, "Go Basics for Acme", "2027-02-01", "2027-02-03", "2")
	res, body := admin.get("/admin/courses/" + copyID + "/assignments")
	expectStatus(t, res, body, http.StatusOK, "Write a prompt")
	var published, drafts int
	if err := admin.db.QueryRow(`SELECT count(*) FILTER (WHERE published), count(*) FILTER (WHERE NOT published)
		FROM assignments WHERE course_id = $1`, copyID).Scan(&published, &drafts); err != nil {
		t.Fatal(err)
	}
	if published != 1 || drafts != 1 {
		t.Fatalf("the copy has %d published and %d Draft Assignments, want 1 and 1", published, drafts)
	}
}
