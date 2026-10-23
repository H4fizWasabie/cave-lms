package app_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// at sets the app's clock to a fixed moment, in UTC.
func (s *site) at(moment string) {
	s.t.Helper()
	t, err := time.Parse(time.DateTime, moment)
	if err != nil {
		s.t.Fatal(err)
	}
	s.application.SetClock(func() time.Time { return t })
}

// adaEarns has Ada earn a Certificate for "Go Basics", as earnedCertificate does, and returns the Course and Certificate IDs.
func (s *site) adaEarns() (courseID, certID string) {
	s.t.Helper()
	courseID = s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(courseID, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	return courseID, certificateIDs(body)[0]
}

// earns has a new Learner join the Course made by learnerEnrolledIn and finish its "Variables" Lesson, and returns
// their My certificates page. They stay logged in.
func (s *site) earns(name, email, courseID string) string {
	s.t.Helper()
	s.logOut()
	s.loginAsNewLearner(name, email, certPassword)
	s.setICFor(email)
	s.post("/courses/"+courseID+"/join", nil)
	s.markLesson(courseID, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	return body
}

func TestCertificatesAreNumberedInIssueOrderOnEveryPage(t *testing.T) {
	s := newSite(t)
	s.at("2026-03-01 10:00:00")
	courseID, adaID := s.adaEarns()
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "PCC-2026-00001")
	body = s.earns("Bob", "bob@example.com", courseID)
	if want := "PCC-2026-00002"; !strings.Contains(body, want) {
		t.Fatalf("Bob's certificates lack %q: %s", want, body)
	}

	s.logOut()
	res, body = s.get("/certificates/" + adaID)
	expectStatus(t, res, body, http.StatusOK, "PCC-2026-00001")

	s.loginAdmin()
	res, body = s.get("/admin/certificates")
	expectStatus(t, res, body, http.StatusOK, "")
	expectInOrder(t, body, "PCC-2026-00002", "PCC-2026-00001")
}

func TestCertificateNumberRestartsEachYear(t *testing.T) {
	s := newSite(t)
	s.at("2026-12-31 23:59:00")
	courseID, _ := s.adaEarns()
	s.at("2027-01-01 00:01:00")
	body := s.earns("Bob", "bob@example.com", courseID)
	if want := "PCC-2027-00001"; !strings.Contains(body, want) {
		t.Fatalf("the first Certificate of 2027 lacks %q: %s", want, body)
	}
}

func TestRevokedCertificateKeepsItsNumberAndNoNumberIsReused(t *testing.T) {
	s := newSite(t)
	s.at("2026-03-01 10:00:00")
	courseID, adaID := s.adaEarns()
	s.loginAdmin()
	s.post("/admin/certificates/"+adaID+"/revoke", nil)

	res, body := s.get("/certificates/" + adaID)
	expectStatus(t, res, body, http.StatusOK, "PCC-2026-00001")
	res, body = s.post("/admin/certificates/"+adaID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "PCC-2026-00002")

	body = s.earns("Bob", "bob@example.com", courseID)
	if want := "PCC-2026-00003"; !strings.Contains(body, want) {
		t.Fatalf("a later Certificate lacks %q: %s", want, body)
	}
}

func TestCertificatesIssuedBeforeNumbersAreNumberedByIssueDate(t *testing.T) {
	s := newSite(t)
	courseID, _ := s.adaEarns()
	s.earns("Bob", "bob@example.com", courseID)
	s.earns("Cy", "cy@example.com", courseID)
	if err := app.MigrateDownTo(context.Background(), s.db, 84); err != nil {
		t.Fatal(err)
	}
	for email, issued := range map[string]string{"ada@example.com": "2025-07-01", "bob@example.com": "2025-06-01", "cy@example.com": "2026-01-05"} {
		if _, err := s.db.Exec(`UPDATE certificates SET issued_at = $2 WHERE learner_id = (SELECT id FROM learners WHERE email = $1)`, email, issued); err != nil {
			t.Fatal(err)
		}
	}
	s.restart()

	s.loginAdmin()
	res, body := s.get("/admin/certificates")
	expectStatus(t, res, body, http.StatusOK, "")
	expectInOrder(t, body, "PCC-2026-00001", "Cy", "PCC-2025-00002", "Ada", "PCC-2025-00001", "Bob")

	s.at("2026-02-01 10:00:00")
	body = s.earns("Di", "di@example.com", courseID)
	if want := "PCC-2026-00002"; !strings.Contains(body, want) {
		t.Fatalf("the next Certificate after the backfill lacks %q: %s", want, body)
	}
}
