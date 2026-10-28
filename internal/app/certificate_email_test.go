package app_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

// withSubject keeps the emails sent under subject, leaving out the account's own, such as the verify email.
func withSubject(emails []mailer.Message, subject string) []mailer.Message {
	var got []mailer.Message
	for _, m := range emails {
		if m.Subject == subject {
			got = append(got, m)
		}
	}
	return got
}

// expectCertificateEmail checks there is one Certificate email, with the PDF attached and a link to the Certificate's check page.
func expectCertificateEmail(t *testing.T, all []mailer.Message, id, subject, body string) {
	t.Helper()
	emails := withSubject(all, subject)
	if len(emails) != 1 {
		t.Fatalf("%d emails titled %q, want the one Certificate email", len(emails), subject)
	}
	email := emails[0]
	expectEmail(t, email, subject, body)
	expectAll(t, email.Body, "/certificates/"+id)
	if len(email.Attachments) != 1 {
		t.Fatalf("the Certificate email carries %d attachments, want the PDF", len(email.Attachments))
	}
	pdf := email.Attachments[0]
	if !strings.HasSuffix(pdf.Filename, ".pdf") || !strings.HasPrefix(string(pdf.Content), "%PDF") {
		t.Fatalf("attachment %q starts %.20q, want a PDF", pdf.Filename, pdf.Content)
	}
	expectAll(t, string(pdf.Content), "Go Basics")
}

func TestAnEarnedCertificateIsEmailedOnceWithItsPDFAndCheckLink(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.switchLanguage("en", "/") // saved on Ada's account
	lesson := s.lessonIDByTitle("Variables")
	s.markLesson(id, lesson, true)
	s.markLesson(id, lesson, false)
	s.markLesson(id, lesson, true) // already has it: no second email

	_, body := s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates, want 1: %s", len(ids), body)
	}
	expectCertificateEmail(t, s.emailsTo("ada@example.com"), ids[0], "Your Certificate for Go Basics", "Hi Ada")
}

func TestACertificateIssuedOnSettingTheICIsEmailedInTheSavedLanguage(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.logOut()
	s.loginAsNewLearner("Bea", "bea@example.com", certPassword)
	s.post("/courses/"+id+"/join", nil)
	s.switchLanguage("ms", "/")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	if n := len(withSubject(s.emailsTo("bea@example.com"), "Sijil anda untuk Go Basics")); n != 0 {
		t.Fatalf("%d emails before Bea set her IC, want none", n)
	}

	s.setIC("951206085555")
	_, body := s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("Bea has %d Certificates, want 1: %s", len(ids), body)
	}
	expectCertificateEmail(t, s.emailsTo("bea@example.com"), ids[0], "Sijil anda untuk Go Basics", "Hai Bea")
}

func TestAFailedCertificateEmailIsLoggedAndKeepsTheCertificate(t *testing.T) {
	t.Parallel()
	s := newSite(t, func(c *config.Config) { c.OwnerEmail = "admin@example.com" }) // the Admin learnerEnrolledIn makes
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.mail.FailWith(errors.New("mail server down"))
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)

	_, body := s.get("/my-certificates")
	if ids := certificateIDs(body); len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates after the email failed, want 1: %s", len(ids), body)
	}
	s.logOut()
	s.login("admin@example.com", certPassword)
	_, body = s.get("/admin/log")
	expectAll(t, body, "send Certificate email", "mail server down")
}

func TestAReissuedCertificateIsEmailedAndALearnerWithNoICGetsNone(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	oldID := s.earnedCertificate()
	s.loginAdmin()
	s.post("/admin/certificates/"+oldID+"/revoke", confirming)
	res, body := s.post("/admin/certificates/"+oldID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	newID := strings.TrimPrefix(res.Header.Get("Location"), "/admin/certificates?q=")
	var sent []string
	for _, m := range s.emailsTo("ada@example.com") {
		if len(m.Attachments) == 1 {
			sent = append(sent, m.Body)
		}
	}
	if len(sent) != 2 { // the earned one, then the reissued one
		t.Fatalf("Ada was emailed %d Certificates, want 2", len(sent))
	}
	expectAll(t, sent[1], "/certificates/"+newID)

	s.post("/admin/certificates/"+newID+"/revoke", confirming)
	if _, err := s.db.Exec(`UPDATE learners SET ic = NULL WHERE email = 'ada@example.com'`); err != nil { // from before ICs
		t.Fatal(err)
	}
	res, body = s.post("/admin/certificates/"+newID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Ada Lovelace has no IC or passport number. Set it before issuing a corrected Certificate.")
	_, body = s.get("/admin/certificates")
	if n := strings.Count(body, "<td>Valid</td>"); n != 0 {
		t.Fatalf("a Certificate was reissued without an IC: %s", body)
	}
}
