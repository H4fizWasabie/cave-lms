package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// earnedCertificate has Ada earn a Certificate for "Go Basics" and returns its ID. Ada is logged in afterwards.
func (s *site) earnedCertificate() string {
	s.t.Helper()
	id := s.learnerEnrolledIn("Go Basics", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		s.t.Fatalf("want one Certificate after finishing, got %v", ids)
	}
	return ids[0]
}

// loginAdmin logs in as the Admin, whichever Learner was logged in before.
func (s *site) loginAdmin() {
	s.t.Helper()
	s.logOut()
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
}

func TestVisitorSeesOnlyNameCourseDateAndStatus(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.logOut()

	res, body := s.get("/certificates/" + certID)
	expectStatus(t, res, body, http.StatusOK, "Status: Valid")
	for _, want := range []string{"Ada", "Go Basics", "Issued "} {
		if !strings.Contains(body, want) {
			t.Fatalf("verification page lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "ada@example.com") {
		t.Fatalf("verification page shows the Learner's email")
	}
}

func TestUnknownCertificateIDIsNotFound(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/certificates/not-a-real-certificate-id")
	expectStatus(t, res, body, http.StatusNotFound, "No Certificate has this ID.")
}

func TestVerifyFormRedirectsToTheCertificate(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.logOut()

	res, body := s.get("/certificates")
	expectStatus(t, res, body, http.StatusOK, "Check a Certificate")

	res, body = s.get("/certificates?id=" + certID)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/certificates/"+certID {
		t.Fatalf("Location = %q, want /certificates/%s", loc, certID)
	}

	res, body = s.get("/certificates?id=")
	expectStatus(t, res, body, http.StatusOK, "Check a Certificate")
}

func TestOnlyAdminCanRevokeOrReissue(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()

	res, body := s.post("/admin/certificates/"+certID+"/revoke", nil)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Hijack"}})
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")

	s.logOut()
	res, body = s.post("/admin/certificates/"+certID+"/revoke", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	res, body = s.get("/certificates/" + certID)
	expectStatus(t, res, body, http.StatusOK, "Status: Valid")
}

func TestRevokedCertificateShowsRevokedAndLeavesTheLearnersList(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	res, body := s.post("/admin/certificates/"+certID+"/revoke", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	res, body = s.get("/certificates/" + certID)
	expectStatus(t, res, body, http.StatusOK, "Status: Revoked")
	if strings.Contains(body, "Status: Valid") {
		t.Fatalf("revoked Certificate still shows as valid")
	}

	s.login("ada@example.com", certPassword)
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")
	res, body = s.get("/certificates/" + certID + "/pdf")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Certificate at this address.")
}

func TestRevokedCertificateIsNotIssuedAgainAutomatically(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	var courseID string
	if err := s.db.QueryRow(`SELECT course_id::text FROM certificates WHERE id = $1`, certID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	s.loginAdmin()
	s.post("/admin/certificates/"+certID+"/revoke", nil)

	s.login("ada@example.com", certPassword)
	lessonID := s.lessonIDByTitle("Variables")
	s.markLesson(courseID, lessonID, false)
	s.markLesson(courseID, lessonID, true)
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "You have no Certificates yet.")
}

func TestAdminIssuesACorrectedCertificateWithANewID(t *testing.T) {
	s := newSite(t)
	oldID := s.earnedCertificate()
	s.loginAdmin()
	s.post("/admin/certificates/"+oldID+"/revoke", nil)

	res, body := s.post("/admin/certificates/"+oldID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.login("ada@example.com", certPassword)
	_, body = s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 || ids[0] == oldID {
		t.Fatalf("want one new Certificate after reissue, got %v (old %s)", ids, oldID)
	}
	newID := ids[0]
	pdf := s.pdfBody("/certificates/" + newID + "/pdf")
	if !strings.Contains(pdf, "Ada Lovelace") {
		t.Fatalf("corrected PDF lacks the corrected name")
	}

	s.logOut()
	res, body = s.get("/certificates/" + newID)
	expectStatus(t, res, body, http.StatusOK, "Ada Lovelace")
	if !strings.Contains(body, "Status: Valid") {
		t.Fatalf("corrected Certificate is not valid: %s", body)
	}
	res, body = s.get("/certificates/" + oldID)
	expectStatus(t, res, body, http.StatusOK, "Status: Revoked")
}

func TestReissueNeedsARevokedCertificateAndOneValidAtATime(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()

	res, body := s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Revoke the Certificate before issuing a corrected one.")

	s.post("/admin/certificates/"+certID+"/revoke", nil)
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a name for the Certificate.")

	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Byron"}})
	expectStatus(t, res, body, http.StatusConflict, "This Learner already has a valid Certificate for this Course.")
}

func TestAdminFindsCertificatesByNameOrID(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()

	res, body := s.get("/admin/certificates")
	expectStatus(t, res, body, http.StatusOK, "Ada")
	if !strings.Contains(body, certID) {
		t.Fatalf("admin list lacks the Certificate ID")
	}
	res, body = s.get("/admin/certificates?q=ada")
	expectStatus(t, res, body, http.StatusOK, "Ada")
	if !strings.Contains(body, certID) {
		t.Fatalf("search by name lacks the Certificate")
	}
	res, body = s.get("/admin/certificates?q=" + certID)
	expectStatus(t, res, body, http.StatusOK, certID)
	res, body = s.get("/admin/certificates?q=Grace")
	expectStatus(t, res, body, http.StatusOK, "No Certificates match.")

	s.login("ada@example.com", certPassword)
	res, body = s.get("/admin/certificates")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
}
