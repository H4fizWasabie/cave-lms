package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestGenerateWritesANewLessonsTextIntoTheFormAndSavesItOnlyAsADraft(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Setup", "", "Install Go.", false)
	s.addTextLesson(id, "Loops", "", "for.", true)

	_, body := s.get("/admin/courses/" + id + "/lessons/new")
	expectAll(t, body, `formaction="/admin/courses/`+id+`/lessons/generate"`, `<option value="en" selected>`)
	expectAll(t, body, `name="draft" value="on" checked`) // a new Lesson saves as a Draft unless the Admin unticks it

	form := url.Values{"title": {" Variables "}, "objective": {"Declare a variable.\n\nUse := inside functions."}, "body": {""}, "language": {"ms"}}
	res, body := s.post("/admin/courses/"+id+"/lessons/generate", form)
	expectStatus(t, res, body, http.StatusOK, "AI lesson text")
	expectAll(t, body, `value="Variables"`, "Declare a variable.\nUse := inside functions.", `<option value="ms" selected>`,
		`name="draft" value="on" checked`, `action="/admin/courses/`+id+`/lessons"`)
	want := aiwriter.LessonRequest{CourseTitle: "Go Basics", Title: "Variables", Objectives: []string{"Declare a variable.", "Use := inside functions."},
		OtherTitles: []string{"Setup", "Loops"}, Language: "ms"}
	if got := s.ai.LessonRequests(); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("the AI writer was asked %#v, want only %#v", got, want)
	}
	_, body = s.get("/admin/courses/" + id + "/lessons")
	if strings.Contains(body, "Variables") {
		t.Fatalf("Generate saved the Lesson: %s", body)
	}

	form.Set("body", testutil.FakeLessonText)
	form.Set("draft", "on")
	res, body = s.post("/admin/courses/"+id+"/lessons", form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	expectAll(t, body, `Variables <span class="badge badge-draft">`)
}

func TestGenerateRewritesAPublishedLessonsTextWhichStaysPublishedOnSave(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Setup", "", "Install Go.", false)
	s.addTextLesson(id, "Variables", "Declare a variable.", "Old text.", false)
	lessonID := s.lessonIDByTitle("Variables")

	_, body := s.get("/admin/lessons/" + lessonID + "/edit")
	expectAll(t, body, `formaction="/admin/lessons/`+lessonID+`/generate"`, `data-confirm-if="body"`)
	form := url.Values{"title": {"Variables"}, "objective": {"Declare a variable."}, "body": {"Old text."}, "language": {"en"}}
	res, body := s.post("/admin/lessons/"+lessonID+"/generate", form)
	expectStatus(t, res, body, http.StatusOK, "AI lesson text")
	expectAll(t, body, `action="/admin/lessons/`+lessonID+`"`)
	if strings.Contains(body, "Old text.") {
		t.Fatalf("the Text box still holds the old text: %s", body)
	}
	if got := s.ai.LessonRequests(); len(got) != 1 || !reflect.DeepEqual(got[0].OtherTitles, []string{"Setup"}) || got[0].Language != "en" {
		t.Fatalf("the AI writer was asked %#v, want the other Lesson Setup only, in English", got)
	}
	_, body = s.get("/courses/" + id + "/lessons/" + lessonID)
	expectAll(t, body, "Old text.")

	form.Set("body", testutil.FakeLessonText)
	res, body = s.post("/admin/lessons/"+lessonID, form)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/courses/" + id + "/lessons/" + lessonID)
	expectAll(t, body, "AI lesson text")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	if strings.Contains(body, "badge-draft") {
		t.Fatalf("the published Lesson went back to Draft: %s", body)
	}
}

func TestGenerateNeedsATitleAndAnObjectiveBeforeAskingTheAI(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	for _, c := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"title": {"Variables"}, "objective": {" \n "}, "body": {"Kept text."}}, "Write at least one Objective before generating the text."},
		{url.Values{"title": {" "}, "objective": {"Declare a variable."}, "body": {"Kept text."}}, "Enter a title (up to 200 characters)."},
		{url.Values{"title": {"Variables"}, "objective": {"1\n2\n3\n4\n5\n6"}, "body": {"Kept text."}}, "Write at most five Objectives"},
	} {
		res, body := s.post("/admin/courses/"+id+"/lessons/generate", c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectAll(t, body, "Kept text.")
	}
	if n := len(s.ai.LessonRequests()); n != 0 {
		t.Fatalf("the AI writer was asked %d times, want none", n)
	}
}

func TestGenerateExplainsWhenTheAIFailsAndKeepsTheForm(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	form := url.Values{"title": {"Variables"}, "objective": {"Declare a variable."}, "body": {"Kept text."}, "language": {"en"}}
	for _, c := range []struct {
		replies []testutil.FakeLesson
		want    string
	}{
		{[]testutil.FakeLesson{{Err: aiwriter.ErrNotConfigured}}, "The AI writer is not configured yet, so no text was written."},
		{[]testutil.FakeLesson{{Err: fmt.Errorf("%w: timeout", aiwriter.ErrUnavailable)}}, "The AI service is unavailable right now, so no text was written."},
		{[]testutil.FakeLesson{{Err: fmt.Errorf("%w: 402", aiwriter.ErrOverLimit)}}, "The AI key is over its spending limit, so no text was written."},
		{[]testutil.FakeLesson{{Text: "Too short."}, {Text: ""}}, "The AI&#39;s text came back in the wrong shape twice. Try again."},
	} {
		s.ai.ReplyLessons(c.replies...)
		res, body := s.post("/admin/courses/"+id+"/lessons/generate", form)
		expectStatus(t, res, body, http.StatusServiceUnavailable, c.want)
		expectAll(t, body, `value="Variables"`, "Declare a variable.", "Kept text.")
	}
}

func TestLearnersAreRefusedLessonGeneration(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "Declare a variable.", "Text.", false)
	lessonID := s.lessonIDByTitle("Variables")
	s.logOut()
	s.loginAsNewLearner("Mallory", "mallory@example.com", "correct horse battery")
	form := url.Values{"title": {"Variables"}, "objective": {"Declare a variable."}}
	for _, path := range []string{"/admin/courses/" + id + "/lessons/generate", "/admin/lessons/" + lessonID + "/generate"} {
		res, body := s.post(path, form)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	if n := len(s.ai.LessonRequests()); n != 0 {
		t.Fatalf("a Learner's generation asked the AI writer %d times, want none", n)
	}
}
