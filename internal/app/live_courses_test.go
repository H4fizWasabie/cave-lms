package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// duplicateCourse posts the duplicate form for Course id as the logged-in Admin.
func (s *site) duplicateCourse(id, title, start, end, days string) (*http.Response, string) {
	return s.post("/admin/courses/"+id+"/duplicate", url.Values{"title": {title}, "description": {"For Acme Sdn Bhd."},
		"start_date": {start}, "end_date": {end}, "days": {days}})
}

// copiedCourseID duplicates Course id and returns the copy's ID, read from where the duplicate redirects.
func (s *site) copiedCourseID(id, title, start, end, days string) string {
	s.t.Helper()
	res, body := s.duplicateCourse(id, title, start, end, days)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	m := regexp.MustCompile(`^/admin/courses/(\d+)/edit$`).FindStringSubmatch(res.Header.Get("Location"))
	if m == nil {
		s.t.Fatalf("duplicate redirected to %q, want the copy's edit page", res.Header.Get("Location"))
	}
	return m[1]
}

// courseWithContent makes "Go Basics" with a text Lesson and a one-question Multiple-choice Exam, as the Admin.
func (s *site) courseWithContent() string {
	s.t.Helper()
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "Declare a variable.", "Use var or :=.", false)
	s.saveQuestion("/admin/courses/"+id+"/exam/questions", "What is Go?", [5]string{"A language", "A drink", "", "", ""}, "0", false)
	return id
}

func TestAdminDuplicatesACourseIntoADraftLiveCourseWithItsLessonsAndExam(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+id+"/join", nil)
	s.logOut()
	s.login("admin@example.com", "correct horse battery")

	copyID := s.copiedCourseID(id, "Go Basics for Acme", "2027-02-01", "2027-02-03", "2")

	res, body := s.get("/admin/courses/" + copyID + "/lessons")
	expectStatus(t, res, body, http.StatusOK, "Variables")
	res, body = s.get("/admin/courses/" + copyID + "/exam")
	expectStatus(t, res, body, http.StatusOK, "What is Go?")
	expectStatus(t, res, body, http.StatusOK, "A language (correct)")
	res, body = s.get("/admin/courses/" + copyID)
	expectStatus(t, res, body, http.StatusOK, "Go Basics for Acme")
	if strings.Contains(body, "ada@example.com") {
		t.Fatalf("the copy has the original's Learners: %s", body)
	}
	res, body = s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, "Draft")

	// A Draft is hidden from Learners until the Admin publishes it.
	s.logOut()
	s.login("ada@example.com", "correct horse battery")
	res, body = s.get("/courses")
	if strings.Contains(body, "Go Basics for Acme") {
		t.Fatalf("a Learner sees the Draft copy on the Course list: %s", body)
	}
	res, body = s.get("/courses/" + copyID)
	expectStatus(t, res, body, http.StatusNotFound, "There is no Course at this address.")
	res, body = s.post("/courses/"+copyID+"/join", nil)
	expectStatus(t, res, body, http.StatusNotFound, "")

	s.logOut()
	s.login("admin@example.com", "correct horse battery")
	res, body = s.post("/admin/courses/"+copyID+"/publish", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()
	res, body = s.get("/courses")
	expectStatus(t, res, body, http.StatusOK, "Go Basics for Acme")
}

// firstMatch is the first submatch of re in the body of GET path.
func (s *site) firstMatch(path string, re *regexp.Regexp) string {
	s.t.Helper()
	_, body := s.get(path)
	m := re.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("GET %s has no match for %s: %s", path, re, body)
	}
	return m[1]
}

func TestEditingACopysLessonsOrExamNeverChangesTheOriginalOrAnotherCopy(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyA := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-01", "1")
	copyB := s.copiedCourseID(id, "Go for Beta", "2027-03-01", "2027-03-01", "1")

	lessonA := s.firstMatch("/admin/courses/"+copyA+"/lessons", regexp.MustCompile(`/admin/lessons/(\d+)/edit`))
	res, body := s.post("/admin/lessons/"+lessonA, url.Values{"title": {"Acme variables"}, "objective": {""}, "body": {"Acme text."}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	questionA := s.firstMatch("/admin/courses/"+copyA+"/exam", examQuestionID)
	res, body = s.saveQuestion("/admin/courses/"+copyA+"/exam/questions/"+questionA, "Acme question?", [5]string{"Yes", "No", "", "", ""}, "1", false)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	for _, other := range []string{id, copyB} {
		res, body = s.get("/admin/courses/" + other + "/lessons")
		expectStatus(t, res, body, http.StatusOK, "Variables")
		res, body = s.get("/admin/courses/" + other + "/exam")
		expectStatus(t, res, body, http.StatusOK, "What is Go?")
		expectStatus(t, res, body, http.StatusOK, "A language (correct)")
		if strings.Contains(body, "Acme question?") {
			t.Fatalf("editing a copy's question changed Course %s: %s", other, body)
		}
	}
	res, body = s.get("/admin/courses/" + copyA + "/exam")
	expectStatus(t, res, body, http.StatusOK, "Acme question?")
}

func TestAdminSetsAndEditsALiveCoursesDatesAndDaysAndBadValuesAreRefused(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()

	for _, bad := range []struct{ start, end, days, want string }{
		{"", "2027-02-03", "2", "Enter the start date as YYYY-MM-DD."},
		{"2027-02-01", "", "2", "Enter the end date as YYYY-MM-DD, on or after the start date."},
		{"2027-02-03", "2027-02-01", "2", "Enter the end date as YYYY-MM-DD, on or after the start date."},
		{"2027-02-01", "2027-02-03", "0", "Enter the days as a whole number from 1 to 5."},
		{"2027-02-01", "2027-02-03", "6", "Enter the days as a whole number from 1 to 5."},
		{"2027-02-01", "2027-02-03", "two", "Enter the days as a whole number from 1 to 5."},
	} {
		res, body := s.duplicateCourse(id, "Go for Acme", bad.start, bad.end, bad.days)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, bad.want)
	}
	if _, body := s.get("/admin/courses"); strings.Contains(body, "Go for Acme") {
		t.Fatalf("a refused duplicate made a Course: %s", body)
	}

	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "2")
	res, body := s.get("/admin/courses/" + copyID + "/edit")
	expectStatus(t, res, body, http.StatusOK, `name="end_date" value="2027-02-03"`)
	expectStatus(t, res, body, http.StatusOK, `name="days" min="1" max="5" value="2"`)

	// Days need not match the dates: 2 days spread over a week.
	edit := url.Values{"title": {"Go for Acme"}, "description": {"For Acme."}, "start_date": {"2027-02-02"}, "end_date": {"2027-02-09"}, "days": {"2"}}
	res, body = s.post("/admin/courses/"+copyID, edit)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get("/admin/courses/" + copyID + "/edit")
	expectStatus(t, res, body, http.StatusOK, `name="start_date" value="2027-02-02"`)
	expectStatus(t, res, body, http.StatusOK, `name="end_date" value="2027-02-09"`)

	edit.Set("days", "9")
	res, body = s.post("/admin/courses/"+copyID, edit)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the days as a whole number from 1 to 5.")
	edit.Set("days", "2")
	edit.Set("end_date", "")
	res, body = s.post("/admin/courses/"+copyID, edit)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter the end date as YYYY-MM-DD, on or after the start date.")
}

var pccBadge = regexp.MustCompile(`<img src="(/static/pcc-[a-z]+\.jpg[^"]*)" alt="PCC ([A-Za-z]+) badge"`)

func TestThePCCLevelFollowsFromDaysAndItsBadgeShowsToTheAdminAndLearners(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-05", "1")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	edit := url.Values{"title": {"Go for Acme"}, "description": {"For Acme."}, "start_date": {"2027-02-01"}, "end_date": {"2027-02-05"}}

	for _, c := range []struct{ days, level string }{{"1", "Basic"}, {"2", "Foundation"}, {"3", "Practitioner"}, {"4", "Professional"}, {"5", "Professional"}} {
		edit.Set("days", c.days)
		res, body := s.post("/admin/courses/"+copyID, edit)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		for _, path := range []string{"/admin/courses/" + copyID + "/edit", "/courses/" + copyID} {
			res, body = s.get(path)
			expectStatus(t, res, body, http.StatusOK, "PCC Level: "+c.level)
			m := pccBadge.FindStringSubmatch(body)
			if m == nil || m[2] != c.level {
				t.Fatalf("%s with %s days shows badge %v, want PCC %s", path, c.days, m, c.level)
			}
			img, _ := s.get(m[1])
			if img.StatusCode != http.StatusOK || img.Header.Get("Content-Type") != "image/jpeg" {
				t.Fatalf("GET %s = %d %s, want the badge JPEG", m[1], img.StatusCode, img.Header.Get("Content-Type"))
			}
		}
	}

	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := s.get("/courses/" + copyID)
	expectStatus(t, res, body, http.StatusOK, "PCC Level: Professional")
	if !pccBadge.MatchString(body) {
		t.Fatalf("a Learner sees no PCC badge: %s", body)
	}
	if _, body := s.get("/courses/" + id); pccBadge.MatchString(body) || strings.Contains(body, "PCC Level") {
		t.Fatalf("a self-paced Course shows a PCC Level: %s", body)
	}
}

func TestLiveCoursesAreListedInTheCoursesSectionWithTheirDates(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "2")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	res, body := s.get("/admin/courses")
	expectStatus(t, res, body, http.StatusOK, "2027-02-01 – 2027-02-03")

	s.logOut()
	res, body = s.get("/courses")
	expectStatus(t, res, body, http.StatusOK, "Go for Acme")
	expectStatus(t, res, body, http.StatusOK, "Dates: 1 February 2027 to 3 February 2027")
	expectStatus(t, res, body, http.StatusOK, "PCC Level: Foundation")
	res, body = s.get("/courses/" + copyID)
	expectStatus(t, res, body, http.StatusOK, "Dates: 1 February 2027 to 3 February 2027")
}

func TestJoiningALiveCourseWorksUntilTheEndOfItsEndDateInMalaysiaThenItShowsClosed(t *testing.T) {
	now := time.Date(2027, 1, 20, 9, 0, 0, 0, time.UTC) // before the start date: joining is open from publishing
	s := newSite(t)
	s.application.SetClock(func() time.Time { return now })
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "2")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	s.logOut()

	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := s.post("/courses/"+copyID+"/join", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.logOut()

	s.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	now = time.Date(2027, 2, 3, 15, 59, 0, 0, time.UTC) // 23:59 on the end date in Malaysia
	res, body = s.get("/courses/" + copyID)
	expectStatus(t, res, body, http.StatusOK, `action="/courses/`+copyID+`/join"`)
	now = time.Date(2027, 2, 3, 16, 0, 0, 0, time.UTC) // midnight after the end date in Malaysia
	res, body = s.post("/courses/"+copyID+"/join", nil)
	expectStatus(t, res, body, http.StatusConflict, "This Course is closed to new joins.")
	if n := s.enrollmentCount("bob@example.com", copyID); n != 0 {
		t.Fatalf("a late join made %d Enrollments", n)
	}
	res, body = s.get("/courses/" + copyID)
	expectStatus(t, res, body, http.StatusOK, "Closed: the Course has ended")
	expectStatus(t, res, body, http.StatusOK, "Variables")
	if strings.Contains(body, `/join"`) {
		t.Fatalf("an ended Live Course still offers joining: %s", body)
	}
	s.logOut()
	res, body = s.get("/courses")
	expectStatus(t, res, body, http.StatusOK, "Closed: the Course has ended")

	// A Learner who joined in time still opens its Lessons.
	s.login("ada@example.com", "correct horse battery")
	res, body = s.get("/courses/" + copyID + "/lessons/" + s.firstMatch("/courses/"+copyID, regexp.MustCompile(`/courses/\d+/lessons/(\d+)"`)))
	expectStatus(t, res, body, http.StatusOK, "Use var or :=.")

	// The self-paced original has no end: it stays open.
	res, body = s.post("/courses/"+id+"/join", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

func TestALiveCoursesExamOpensFromItsStartDateToTheEndOfItsEndDateWithoutTheLessonsFirst(t *testing.T) {
	now := time.Date(2027, 1, 20, 9, 0, 0, 0, time.UTC)
	s := newSite(t)
	s.application.SetClock(func() time.Time { return now })
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	id := s.courseWithContent()
	copyID := s.copiedCourseID(id, "Go for Acme", "2027-02-01", "2027-02-03", "2")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	qid := s.examQuestionIDs(copyID)[0]
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.setICFor("ada@example.com")
	s.post("/courses/"+copyID+"/join", nil)

	now = time.Date(2027, 1, 31, 15, 59, 0, 0, time.UTC) // 23:59 the day before the start date in Malaysia
	res, body := s.get("/courses/" + copyID + "/exam")
	expectStatus(t, res, body, http.StatusOK, "The Exam opens on 1 February 2027.")
	expectNone(t, body, "Start the Exam")
	res, body = s.startExam(copyID)
	expectStatus(t, res, body, http.StatusConflict, "The Exam opens on 1 February 2027.")

	// From midnight on the start date the Exam opens, with no Lesson complete.
	now = time.Date(2027, 1, 31, 16, 0, 0, 0, time.UTC)
	res, body = s.startExam(copyID)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	attempt := res.Header.Get("Location")

	// The attempt started inside the window is still submitted after it closes.
	now = time.Date(2027, 2, 3, 16, 0, 0, 0, time.UTC) // midnight after the end date in Malaysia
	res, body = s.submitExam(attempt, map[string]string{qid: "0"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.releaseResults()
	res, body = s.get(attempt)
	expectStatus(t, res, body, http.StatusOK, "Score: 100%")
	res, body = s.get("/courses/" + copyID + "/exam") // passing outranks the closed window
	expectStatus(t, res, body, http.StatusOK, "You have passed this Exam.")
	expectNone(t, body, "The Exam closed on")

	// The Learner who keeps failing can't start again after the end date.
	s.logOut()
	s.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	now = time.Date(2027, 2, 2, 9, 0, 0, 0, time.UTC)
	s.post("/courses/"+copyID+"/join", nil)
	now = time.Date(2027, 2, 3, 16, 0, 0, 0, time.UTC)
	res, body = s.startExam(copyID)
	expectStatus(t, res, body, http.StatusConflict, "The Exam closed on 3 February 2027.")
	res, body = s.get("/courses/" + copyID + "/exam")
	expectStatus(t, res, body, http.StatusOK, "The Exam closed on 3 February 2027.")
	expectNone(t, body, "Start the Exam")

	// The self-paced original still wants its Lessons first.
	s.post("/courses/"+id+"/join", nil)
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "Complete every Lesson before you sit the Exam (0 of 1 complete).")
}

func TestALiveCoursesExamWindowIsShownInMalay(t *testing.T) {
	now := time.Date(2027, 1, 20, 9, 0, 0, 0, time.UTC)
	s := newSite(t)
	s.application.SetClock(func() time.Time { return now })
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	copyID := s.copiedCourseID(s.courseWithContent(), "Go for Acme", "2027-02-01", "2027-02-03", "2")
	s.post("/admin/courses/"+copyID+"/publish", nil)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.post("/courses/"+copyID+"/join", nil)

	res, body := s.postAsking("/courses/"+copyID+"/exam/attempts", "ms", nil)
	expectStatus(t, res, body, http.StatusConflict, "Peperiksaan dibuka pada 1 Februari 2027.")
	now = time.Date(2027, 2, 4, 9, 0, 0, 0, time.UTC)
	res, body = s.postAsking("/courses/"+copyID+"/exam/attempts", "ms", nil)
	expectStatus(t, res, body, http.StatusConflict, "Peperiksaan ditutup pada 3 Februari 2027.")
}
