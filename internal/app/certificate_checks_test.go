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
	t.Parallel()
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
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/certificates/not-a-real-certificate-id")
	expectStatus(t, res, body, http.StatusNotFound, "No Certificate has this ID.")
}

func TestVerifyFormRedirectsToTheCertificate(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()

	res, body := s.post("/admin/certificates/"+certID+"/revoke", confirming)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Hijack"}})
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page.")

	s.logOut()
	res, body = s.post("/admin/certificates/"+certID+"/revoke", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	res, body = s.get("/certificates/" + certID)
	expectStatus(t, res, body, http.StatusOK, "Status: Valid")
}

func TestAdminRevokesACertificateOnlyAfterConfirming(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	confirmPage := "/admin/certificates/" + certID + "/revoke"

	_, body := s.get("/admin/certificates")
	expectAll(t, body, `href="`+confirmPage+`"`)
	res, body := s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "Revoke Ada’s Certificate for “Go Basics” (PCC-")

	res, body = s.post(confirmPage, nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != confirmPage {
		t.Fatalf("a post without confirming went to %q, want the confirmation page", loc)
	}
	_, body = s.get("/certificates/" + certID)
	expectAll(t, body, "Status: Valid")

	res, body = s.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/certificates/" + certID)
	expectAll(t, body, "Status: Revoked")

	res, body = s.get("/admin/certificates/no-such-id/revoke")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestTheRevokeConfirmationOfARevokedCertificateGoesBackToTheListSayingSo(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	confirmPage := "/admin/certificates/" + certID + "/revoke"
	s.post(confirmPage, confirming)

	for lang, notice := range map[string]string{
		"en": "This Certificate is already revoked.",
		"ms": "Sijil ini sudah ditarik balik.",
	} {
		s.switchLanguage(lang, "/")
		res, body := s.get(confirmPage)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		loc := res.Header.Get("Location")
		if !strings.HasPrefix(loc, "/admin/certificates?") {
			t.Fatalf("%s: the confirmation of a revoked Certificate went to %q, want the Certificates list", lang, loc)
		}
		_, body = s.get(loc)
		expectAll(t, content(body), notice)
		expectNone(t, content(body), `action="`+confirmPage+`"`)
	}
}

func TestRevokedCertificateShowsRevokedOnTheCheckPageTheListAndThePDF(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	res, body := s.post("/admin/certificates/"+certID+"/revoke", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.logOut()
	res, body = s.get("/certificates/" + certID)
	expectStatus(t, res, body, http.StatusOK, "Status: Revoked")
	if strings.Contains(body, "Status: Valid") {
		t.Fatalf("revoked Certificate still shows as valid")
	}

	s.login("ada@example.com", certPassword)
	res, body = s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Revoked")
	if ids := certificateIDs(body); len(ids) != 1 || ids[0] != certID {
		t.Fatalf("Learner's list = %v, want the revoked %s", ids, certID)
	}
	for _, who := range []string{"the owner", "the Admin"} {
		pdf := s.pdfBody("/certificates/" + certID + "/pdf")
		for _, want := range []string{"REVOKED", "This Certificate has been revoked"} {
			if !strings.Contains(pdf, want) {
				t.Fatalf("revoked Certificate PDF for %s lacks %q", who, want)
			}
		}
		s.loginAdmin()
	}
}

func TestValidCertificatePDFIsNotMarkedRevoked(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	if pdf := s.pdfBody("/certificates/" + certID + "/pdf"); strings.Contains(pdf, "REVOKED") {
		t.Fatalf("valid Certificate PDF is marked revoked")
	}
}

func TestRevokedCertificateIsNotIssuedAgainAutomatically(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	var courseID string
	if err := s.db.QueryRow(`SELECT course_id::text FROM certificates WHERE id = $1`, certID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	s.loginAdmin()
	s.post("/admin/certificates/"+certID+"/revoke", confirming)

	s.login("ada@example.com", certPassword)
	lessonID := s.lessonIDByTitle("Variables")
	s.markLesson(courseID, lessonID, false)
	s.markLesson(courseID, lessonID, true)
	res, body := s.get("/my-certificates")
	expectStatus(t, res, body, http.StatusOK, "Revoked")
	if ids := certificateIDs(body); len(ids) != 1 || ids[0] != certID {
		t.Fatalf("Learner's list = %v, want only the revoked %s", ids, certID)
	}
}

func TestAdminIssuesACorrectedCertificateWithANewID(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	oldID := s.earnedCertificate()
	s.loginAdmin()
	s.post("/admin/certificates/"+oldID+"/revoke", confirming)

	res, body := s.post("/admin/certificates/"+oldID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	s.login("ada@example.com", certPassword)
	_, body = s.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 2 || ids[0] != oldID {
		t.Fatalf("want the revoked and the new Certificate after reissue, got %v (old %s)", ids, oldID)
	}
	newID := ids[1]
	pdf := s.pdfBody("/certificates/" + newID + "/pdf")
	if !strings.Contains(pdf, "Ada Lovelace") || strings.Contains(pdf, "REVOKED") {
		t.Fatalf("corrected PDF lacks the corrected name or is marked revoked")
	}
	res, body = s.get("/")
	expectStatus(t, res, body, http.StatusOK, "Logged in as Ada Lovelace")

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
	t.Parallel()
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()

	res, body := s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Revoke the Certificate before issuing a corrected one.")

	s.post("/admin/certificates/"+certID+"/revoke", confirming)
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter a name for the Certificate")

	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Byron"}})
	expectStatus(t, res, body, http.StatusConflict, "This Learner already has a valid Certificate for this Course.")
}

func TestAdminFindsCertificatesByNameOrID(t *testing.T) {
	t.Parallel()
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

func TestReissueFormIsPrefilledWithTheLearnersCurrentName(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	oldID := s.earnedCertificate()
	s.loginAdmin()
	s.post("/admin/certificates/"+oldID+"/revoke", confirming)
	res, _ := s.post("/admin/certificates/"+oldID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	newID := strings.TrimPrefix(res.Header.Get("Location"), "/admin/certificates?q=")
	s.post("/admin/certificates/"+newID+"/revoke", confirming)

	res, body := s.get("/admin/certificates")
	expectStatus(t, res, body, http.StatusOK, "Issue corrected Certificate")
	if n := strings.Count(body, `value="Ada Lovelace"`); n != 2 || strings.Contains(body, `value="Ada"`) {
		t.Fatalf("reissue forms are not prefilled with the Learner's current name: %s", body)
	}
}
