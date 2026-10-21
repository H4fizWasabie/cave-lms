package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

const ownerEmail = "owner@example.com"

// ownerSite is a site whose OWNER_EMAIL is ownerEmail, logged in as that Admin.
func ownerSite(t *testing.T) *site {
	t.Helper()
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = ownerEmail })
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	return s
}

// breakCourses makes every Course page fail until the returned func puts the table back.
func breakCourses(t *testing.T, s *site) (restore func()) {
	t.Helper()
	if _, err := s.db.Exec(`ALTER TABLE courses RENAME TO courses_gone`); err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := s.db.Exec(`ALTER TABLE courses_gone RENAME TO courses`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheOwnerSeesAServerErrorInTheSystemLog(t *testing.T) {
	s := ownerSite(t)
	restore := breakCourses(t, s)
	res, body := s.get("/courses?token=s3cret")
	expectStatus(t, res, body, http.StatusInternalServerError, "")
	restore()

	res, body = s.get("/admin/log")
	expectStatus(t, res, body, http.StatusOK, "<h1>System log</h1>")
	for _, want := range []string{"GET /courses", `relation &#34;courses&#34; does not exist`, ">error<"} {
		if !strings.Contains(body, want) {
			t.Errorf("System log lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "s3cret") {
		t.Errorf("System log kept the query string:\n%s", body)
	}
}

func TestOnlyTheOwnerFindsTheSystemLog(t *testing.T) {
	s := ownerSite(t)
	_, body := s.get("/admin")
	if !strings.Contains(body, `href="/admin/log"`) {
		t.Errorf("the Owner's Admin area has no System log link:\n%s", body)
	}

	s.logOut()
	res, body := s.get("/admin/log")
	expectStatus(t, res, body, http.StatusNotFound, "")
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body = s.get("/admin/log")
	expectStatus(t, res, body, http.StatusNotFound, "")
	s.logOut()
	s.loginAsNewAdmin("Mini", "mini@example.com", "correct horse battery")
	res, body = s.get("/admin/log")
	expectStatus(t, res, body, http.StatusNotFound, "")
	_, body = s.get("/admin")
	if strings.Contains(body, `href="/admin/log"`) {
		t.Errorf("another Admin's Admin area links to the System log:\n%s", body)
	}
}

func TestWithoutOwnerEmailNobodyFindsTheSystemLog(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	res, body := s.get("/admin/log")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestTheSystemLogShowsStartUpsAndFiltersToErrors(t *testing.T) {
	s := ownerSite(t)
	s.application.Started(":8080")
	restore := breakCourses(t, s)
	s.get("/courses")
	restore()

	_, body := s.get("/admin/log")
	for _, want := range []string{"Cave LMS started", ">info<", "GET /courses"} {
		if !strings.Contains(body, want) {
			t.Errorf("System log lacks %q:\n%s", want, body)
		}
	}
	_, body = s.get("/admin/log?level=error")
	if !strings.Contains(body, "GET /courses") || strings.Contains(body, "Cave LMS started") {
		t.Errorf("errors only should keep the error and drop the start-up:\n%s", body)
	}
}

func TestTheSystemLogKeepsThirtyDays(t *testing.T) {
	s := ownerSite(t)
	if _, err := s.db.Exec(`INSERT INTO system_log (at, level, message) VALUES
		(now() - interval '31 days', 'error', 'too old to keep'),
		(now() - interval '29 days', 'error', 'still within the month')`); err != nil {
		t.Fatal(err)
	}
	if err := s.application.PruneSystemLog(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, body := s.get("/admin/log")
	if strings.Contains(body, "too old to keep") || !strings.Contains(body, "still within the month") {
		t.Errorf("want only the line from 29 days ago kept:\n%s", body)
	}
}
