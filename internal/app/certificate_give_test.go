package app_test

import (
	"net/http"
	"regexp"
	"testing"
)

var giveCertificateLink = regexp.MustCompile(`action="(/admin/courses/\d+/learners/\d+/certificate)"`)

func TestAdminGivesACertificateInALiveCourseThatLooksEarned(t *testing.T) {
	t.Parallel()
	admin, ada, id, _ := liveCourse(t, "1") // Ada has not sat the Exam
	bea := admin.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", certPassword)
	bea.post("/courses/"+id+"/join", nil)

	_, body := admin.get("/admin/courses/" + id)
	give := giveCertificateLink.FindStringSubmatch(adminCourseRow(t, body, "ada@example.com"))
	if give == nil {
		t.Fatalf("Ada's row has no give-Certificate button: %s", body)
	}
	res, body := ada.post(give[1], nil)
	expectStatus(t, res, body, http.StatusForbidden, "")

	res, body = admin.post(give[1], nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/my-certificates")
	ids := certificateIDs(body)
	if len(ids) != 1 {
		t.Fatalf("Ada has %d Certificates after the Admin gave one, want 1: %s", len(ids), body)
	}
	_, body = ada.get("/certificates/" + ids[0])
	expectAll(t, body, "Status: Valid", "PCC Level: Basic")
	expectNone(t, body, "Given by Admin")
	expectNone(t, ada.pdfBody("/certificates/"+ids[0]+"/pdf"), "Given by Admin")

	_, body = admin.get("/admin/certificates")
	expectAll(t, body, "Given by Admin")
	res, body = admin.post(give[1], nil) // again: still one
	expectStatus(t, res, body, http.StatusConflict, "Ada already has a valid Certificate for this Course.")
	if _, body = ada.get("/my-certificates"); len(certificateIDs(body)) != 1 {
		t.Fatalf("giving twice made another Certificate: %s", body)
	}
	admin.post("/admin/certificates/"+ids[0]+"/revoke", confirming)
	res, body = admin.post(give[1], nil)
	expectStatus(t, res, body, http.StatusConflict, "Certificate for this Course is revoked. Issue a corrected one from Certificates.")
	if _, body = ada.get("/my-certificates"); len(certificateIDs(body)) != 1 {
		t.Fatalf("giving after a revoke made another Certificate: %s", body)
	}

	_, body = admin.get("/admin/courses/" + id)
	give = giveCertificateLink.FindStringSubmatch(adminCourseRow(t, body, "bea@example.com"))
	res, body = admin.post(give[1], nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Bea has no IC or passport number")
	if _, body = bea.get("/my-certificates"); len(certificateIDs(body)) != 0 {
		t.Fatalf("Bea got a Certificate without an IC: %s", body)
	}
}

func TestAnEarnedCertificateIsNotMarkedGivenAndASelfPacedCourseHasNoGiveButton(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.earnedCertificate()
	s.loginAdmin()
	_, body := s.get("/admin/certificates")
	expectNone(t, body, "Given by Admin")
	_, body = s.get("/admin/courses/" + s.onlyCourseID())
	if giveCertificateLink.MatchString(body) {
		t.Fatalf("a self-paced Course offers the give-Certificate button: %s", body)
	}
}
