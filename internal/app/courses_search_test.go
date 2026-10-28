package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// searchCourses asks the Course list for q and returns the results only: the page header also lists Courses.
func (s *site) searchCourses(q string) string {
	s.t.Helper()
	res, body := s.get("/courses?q=" + url.QueryEscape(q))
	expectStatus(s.t, res, body, http.StatusOK, "")
	_, results, ok := strings.Cut(body, `<ul class="catalogue">`)
	if !ok {
		s.t.Fatalf("no results list on the Course page: %s", body)
	}
	results, _, _ = strings.Cut(results, "</main>")
	return results
}

// searchableCourses makes three Courses as the Admin, then logs in as a Learner. "Garden Basics" is found by its
// title or description, "Kebun Sayur" by words in its Lessons, and "Cooking Hour" has nothing in common.
func searchableCourses(t *testing.T) *site {
	t.Helper()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Garden Basics", "Grow tomatoes on a balcony.", "")
	s.createCourse("Kebun Sayur", "Belajar berkebun.", "")
	s.createCourse("Cooking Hour", "Boil water.", "")
	_, body := s.get("/admin/courses")
	ids := courseEditLink.FindAllStringSubmatch(body, -1)
	if len(ids) != 3 {
		t.Fatalf("want 3 Courses, got %d", len(ids))
	}
	// The Admin list is ordered by start date then id, so the Courses come back in creation order.
	s.addTextLesson(ids[1][1], "Tanah dan baja", "Memahami tanah.\nMenyiram anak benih.", "Gunakan kompos untuk menyuburkan cili.", false)
	s.addTextLesson(ids[1][1], "Rahsia lama", "", "Sekam padi mengelakkan kulat.", true)
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	return s
}

func TestCourseSearchFindsAWordInTheTitleOrDescription(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	for _, visitor := range []bool{false, true} {
		if visitor {
			s.logOut()
		}
		for _, q := range []string{"garden", "TOMATOES"} {
			body := s.searchCourses(q)
			if !strings.Contains(body, "Garden Basics") || strings.Contains(body, "Kebun Sayur") || strings.Contains(body, "Cooking Hour") {
				t.Errorf("search %q (visitor %v): want only Garden Basics: %s", q, visitor, body)
			}
		}
	}
}

func TestCourseSearchFindsMalayWords(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	for _, q := range []string{"kebun", "berkebun"} {
		body := s.searchCourses(q)
		if !strings.Contains(body, "Kebun Sayur") || strings.Contains(body, "Garden Basics") {
			t.Errorf("search %q: want only Kebun Sayur: %s", q, body)
		}
	}
}

func TestCourseSearchFindsTheCourseOfALessonByTitleOrObjectiveWhoeverAsks(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	for _, visitor := range []bool{false, true} {
		if visitor {
			s.logOut()
		}
		for _, q := range []string{"baja", "memahami", "menyiram"} {
			body := s.searchCourses(q)
			if !strings.Contains(body, "Kebun Sayur") || strings.Contains(body, "Garden Basics") {
				t.Errorf("search %q (visitor %v): want only Kebun Sayur: %s", q, visitor, body)
			}
		}
	}
}

func TestCourseSearchMatchesLessonBodiesOnlyForALearnerEnrolledInThatCourse(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	kebun := s.courseIDByTitle("Kebun Sayur")
	garden := s.courseIDByTitle("Garden Basics")
	// Not enrolled: the Learner cannot open the Lesson, so its words must not find the Course.
	if body := s.searchCourses("kompos"); strings.Contains(body, "Kebun Sayur") {
		t.Errorf("a Learner not enrolled found a Lesson body word: %s", body)
	}
	// Enrolled in another Course does not help.
	s.post("/courses/"+garden+"/join", nil)
	if body := s.searchCourses("kompos"); strings.Contains(body, "Kebun Sayur") {
		t.Errorf("a Learner enrolled elsewhere found a Lesson body word: %s", body)
	}
	s.post("/courses/"+kebun+"/join", nil)
	if body := s.searchCourses("kompos"); !strings.Contains(body, "Kebun Sayur") || strings.Contains(body, "Garden Basics") {
		t.Errorf("an enrolled Learner should find only Kebun Sayur by a Lesson body word: %s", body)
	}
	// A Visitor never does.
	s.logOut()
	if body := s.searchCourses("kompos"); strings.Contains(body, "Kebun Sayur") {
		t.Errorf("a Visitor found a Lesson body word: %s", body)
	}
}

func TestCourseSearchMatchesLessonBodiesForTheAdminWithoutEnrolling(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	// A Learner who is not enrolled still does not.
	if body := s.searchCourses("kompos"); strings.Contains(body, "Kebun Sayur") {
		t.Errorf("a Learner not enrolled found a Lesson body word: %s", body)
	}
	// The Admin can open every Lesson, so every body is searched, enrolled or not.
	s.logOut()
	res, body := s.login("admin@example.com", "correct horse battery")
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	if body := s.searchCourses("kompos"); !strings.Contains(body, "Kebun Sayur") || strings.Contains(body, "Garden Basics") {
		t.Errorf("the Admin should find only Kebun Sayur by a Lesson body word: %s", body)
	}
}

func TestCourseSearchIgnoresUnpublishedLessons(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	s.post("/courses/"+s.courseIDByTitle("Kebun Sayur")+"/join", nil) // enrolled, so bodies are searched
	if body := s.searchCourses("kompos"); !strings.Contains(body, "Kebun Sayur") {
		t.Fatalf("setup: an enrolled Learner should find a published body word: %s", body)
	}
	if body := s.searchCourses("sekam"); strings.Contains(body, "Kebun Sayur") {
		t.Errorf("a draft Lesson's word found its Course: %s", body)
	}
}

func TestCourseSearchHidesDraftCourses(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Garden Basics", "Grow tomatoes on a balcony.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Compost", "", "Feeds the plants.", false)
	s.copiedCourseID(id, "Acme Orchard", "2027-03-01", "2027-03-02", "2") // a Draft copy with the same Lesson
	_, body := s.get("/admin/courses")
	if !strings.Contains(body, "Acme Orchard") {
		t.Fatalf("setup: no Draft copy: %s", body)
	}
	s.logOut()
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	for _, q := range []string{"orchard", "compost"} {
		got := s.searchCourses(q)
		if strings.Contains(got, "Acme Orchard") {
			t.Errorf("search %q showed a Draft Course: %s", q, got)
		}
	}
	if got := s.searchCourses("compost"); !strings.Contains(got, "Garden Basics") {
		t.Errorf("the published Course should still be found: %s", got)
	}
}

func TestCourseSearchStillShowsAClosedCourseLikeTheList(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Garden Basics", "Grow tomatoes on a balcony.", "")
	s.post("/admin/courses/"+s.onlyCourseID()+"/close", nil)
	s.logOut()
	if body := s.searchCourses("garden"); !strings.Contains(body, "Garden Basics") || !strings.Contains(body, "Closed to new joins") {
		t.Errorf("a Closed Course should be found, marked closed: %s", body)
	}
}

func TestEmptyCourseSearchShowsTheWholeList(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	for _, q := range []string{"", "   "} {
		body := s.searchCourses(q)
		for _, title := range []string{"Garden Basics", "Kebun Sayur", "Cooking Hour"} {
			if !strings.Contains(body, title) {
				t.Errorf("search %q is missing %s", q, title)
			}
		}
	}
}

func TestCourseSearchWithNoMatchSaysSo(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	body := s.searchCourses("xylophone")
	if !strings.Contains(body, "No Courses match your search.") || strings.Contains(body, "Garden Basics") {
		t.Errorf("want the no-match message and no Course: %s", body)
	}
}

func TestCourseSearchSurvivesOddPunctuation(t *testing.T) {
	t.Parallel()
	s := searchableCourses(t)
	for _, q := range []string{`"`, `"garden`, `'`, `&`, `|`, `!`, `(`, `)`, `:*`, `garden &`, `-garden`, `garden OR kebun`, `\`, `%`, `<script>`} {
		s.searchCourses(q) // any 200 will do: input never raises a syntax error
	}
	if body := s.searchCourses(`"grow tomatoes"`); !strings.Contains(body, "Garden Basics") {
		t.Errorf("a quoted phrase should still match: %s", body)
	}
	if body := s.searchCourses(`garden!!!`); !strings.Contains(body, "Garden Basics") {
		t.Errorf("trailing punctuation should be ignored: %s", body)
	}
}
