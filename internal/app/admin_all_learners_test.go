package app_test

import (
	"net/http"
	"regexp"
	"testing"
)

// joinedAt sets when an account was made, so the list's order and dates do not depend on the clock.
func (s *site) joinedAt(email, at string) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE learners SET created_at = ($2 || '+08')::timestamptz WHERE lower(email) = lower($1)`, email, at); err != nil {
		s.t.Fatal(err)
	}
}

func TestAdminSeesEveryAccountNewestFirst(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.createCourse("Go Basics", "Learn Go from scratch.", "")
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+s.onlyCourseID()+"/join", nil)
	cy := s.second()
	cy.loginAsNewLearner("Cy Turing", "cy@example.com", "correct horse battery") // verified, enrolled in no Course
	res, body := s.second().register("Bea Hopper", "bea@example.com", "correct horse battery")
	expectStatus(t, res, body, http.StatusOK, "Check your email") // never verified
	s.joinedAt("admin@example.com", "2026-01-05 10:00")
	s.joinedAt("cy@example.com", "2026-02-01 10:00")
	s.joinedAt("bea@example.com", "2026-03-10 10:00")
	s.joinedAt("ada@example.com", "2026-04-20 10:00")

	res, body = s.get("/admin/learners")
	expectStatus(t, res, body, http.StatusOK, "<h1>Learners</h1>")
	want := regexp.MustCompile(`(?s)Ada Lovelace.*?ada@example\.com.*?<td>Yes</td>\s*<td>20 April 2026</td>` +
		`.*?Bea Hopper.*?bea@example\.com.*?<td>No</td>\s*<td>10 March 2026</td>` +
		`.*?Cy Turing.*?cy@example\.com.*?<td>Yes</td>\s*<td>1 February 2026</td>` +
		`.*?Abah <span class="badge">Admin</span>.*?admin@example\.com.*?<td>Yes</td>\s*<td>5 January 2026</td>`)
	if !want.MatchString(body) {
		t.Fatalf("Learners page does not list every account newest first with its state; body:\n%s", body)
	}
}

func TestOnlyTheAdminSeesAllLearners(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada := s.second()
	ada.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")

	res, body := ada.get("/admin/learners")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
}
