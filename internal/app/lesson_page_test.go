package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

// lessonPageCourse is the Admin making "Go Basics" with the published text Lessons "Variables", "Functions" and
// "Loops", and a draft "Methods" between the last two; then Ada joining it. It returns Ada's site, logged in, and
// the Course ID.
func lessonPageCourse(t *testing.T) (ada *site, id string) {
	t.Helper()
	admin := newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id = admin.onlyCourseID()
	admin.addTextLesson(id, "Variables", "", "Use `:=`.", false)
	admin.addTextLesson(id, "Functions", "", "Use func.", false)
	admin.addTextLesson(id, "Methods", "", "Later.", true)
	admin.addTextLesson(id, "Loops", "", "Use for.", false)
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+id+"/join", nil)
	return ada, id
}

func TestMarkDoneAndNextGoesToTheNextLessonThenBackToTheCourse(t *testing.T) {
	ada, id := lessonPageCourse(t)
	lesson := func(title string) string { return "/courses/" + id + "/lessons/" + ada.lessonIDByTitle(title) }
	doneAndNext := url.Values{"then": {"next"}}

	res, body := ada.post(lesson("Functions")+"/complete", doneAndNext)
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != lesson("Loops") {
		t.Fatalf("Mark done and next on Functions got %d to %q, want 303 to Loops (the draft is skipped): %s", res.StatusCode, loc, content(body))
	}
	res, body = ada.post(lesson("Loops")+"/complete", doneAndNext)
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != "/courses/"+id {
		t.Fatalf("Mark done and next on the last Lesson got %d to %q, want 303 to the Course page: %s", res.StatusCode, loc, content(body))
	}
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, "Progress: 2 of 3 Lessons complete")
}

var pagerLink = regexp.MustCompile(`<a class="lp-(prev|next)" href="([^"]*)" rel="(?:prev|next)">`)

// lessonPager reads a Lesson page's previous and next links, "" where there is none.
func lessonPager(body string) (prev, next string) {
	for _, m := range pagerLink.FindAllStringSubmatch(body, -1) {
		if m[1] == "prev" {
			prev = m[2]
		} else {
			next = m[2]
		}
	}
	return prev, next
}

func TestLessonPageSaysWhereItIsAndLinksThePreviousAndNextLessonsInOrder(t *testing.T) {
	ada, id := lessonPageCourse(t)
	lesson := func(title string) string { return "/courses/" + id + "/lessons/" + ada.lessonIDByTitle(title) }

	for _, c := range []struct{ title, n, prev, next string }{
		{"Variables", "Lesson 1 of 3", "", lesson("Functions")},
		{"Functions", "Lesson 2 of 3", lesson("Variables"), lesson("Loops")},
		{"Loops", "Lesson 3 of 3", lesson("Functions"), ""},
	} {
		res, body := ada.get(lesson(c.title))
		expectStatus(t, res, body, http.StatusOK, c.n)
		if prev, next := lessonPager(body); prev != c.prev || next != c.next {
			t.Fatalf("%s links previous %q and next %q, want %q and %q", c.title, prev, next, c.prev, c.next)
		}
	}
}

func TestMarkDoneControlsAreNamedForTheirLesson(t *testing.T) {
	ada, id := lessonPageCourse(t)
	path := "/courses/" + id + "/lessons/" + ada.lessonIDByTitle("Functions")

	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `action="`+path+`/complete"`)
	expectStatus(t, res, body, http.StatusOK, `<input type="hidden" name="then" value="next">`)
	expectStatus(t, res, body, http.StatusOK, `Mark done and next<span class="sr">: Functions</span>`)

	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `Mark complete<span class="sr">: Functions</span>`)

	ada.markLesson(id, ada.lessonIDByTitle("Functions"), true)
	res, body = ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `Mark incomplete<span class="sr">: Functions</span>`)
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `action="`+path+`/incomplete"`)
	expectStatus(t, res, body, http.StatusOK, `Mark incomplete<span class="sr">: Functions</span>`)
	expectStatus(t, res, body, http.StatusOK, `id="next-step" href="/courses/`+id+`/lessons/`+ada.lessonIDByTitle("Variables")+`"`)

	res, body = ada.post(path+"/incomplete", url.Values{"then": {"lesson"}})
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || loc != path {
		t.Fatalf("Mark incomplete on the Lesson page got %d to %q, want 303 back to the Lesson: %s", res.StatusCode, loc, content(body))
	}
}

func TestAMarkdownTableInALessonRendersAsATableThatScrollsInItsOwnBox(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Types", "", "| Type | Zero value |\n|:-----|-----------:|\n| int | `0` |\n| string | `\"\"` |", false)

	res, body := s.get("/courses/" + id + "/lessons/" + s.lessonIDByTitle("Types"))
	expectStatus(t, res, body, http.StatusOK, `<div class="table-wrap"><table>`)
	for _, want := range []string{`<th align="left">Type</th>`, `<th align="right">Zero value</th>`, `<td align="left">int</td>`, `<td align="right"><code>0</code></td>`, `</table></div>`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
