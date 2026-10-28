package app_test

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestAdminDownloadsAllValidCertificatesOfACourseAsOneZip(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.earnedCertificate()
	s.loginAdmin()
	courseID := s.onlyCourseID()
	variables := s.lessonIDByTitle("Variables")
	var beaCert string
	for _, name := range []string{"Bea Hopper", "Cy O'Neil"} {
		email := strings.ToLower(name[:2]) + "@example.com"
		l := s.second()
		l.loginAsNewLearner(name, email, certPassword)
		l.setICFor(email)
		l.post("/courses/"+courseID+"/join", nil)
		l.markLesson(courseID, variables, true)
		if name == "Bea Hopper" {
			_, body := l.get("/my-certificates")
			beaCert = certificateIDs(body)[0]
		}
	}
	zipPath := "/admin/courses/" + courseID + "/certificates.zip"

	_, body := s.get("/admin/courses/" + courseID)
	if !strings.Contains(body, `href="`+zipPath+`"`) {
		t.Fatalf("the Course page has no Download all Certificates link: %s", body)
	}
	s.post("/admin/certificates/"+beaCert+"/revoke", confirming)
	res, raw := s.get(zipPath)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/zip" ||
		res.Header.Get("Content-Disposition") != `attachment; filename="certificates-course-`+courseID+`.zip"` {
		t.Fatalf("GET %s = %d %q %q", zipPath, res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
	zr, err := zip.NewReader(strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		pdf, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatalf("%s is not a PDF: %.40q", f.Name, pdf)
		}
	}
	if len(names) != 2 || !regexp.MustCompile(`^PCC-LMS-\d{6}-Ada\.pdf$`).MatchString(names[0]) ||
		!regexp.MustCompile(`^PCC-LMS-\d{6}-Cy-O-Neil\.pdf$`).MatchString(names[1]) {
		t.Fatalf("want one PDF each for Ada and Cy, not the revoked Bea; got %v", names)
	}

	s.logOut()
	s.loginAsNewLearner("Dee", "dee@example.com", certPassword)
	res, body = s.get(zipPath)
	expectStatus(t, res, body, http.StatusForbidden, "")
}

func TestCourseWithoutCertificatesHasNoDownloadAllButton(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.learnerEnrolledIn("Go Basics", "Variables")
	s.loginAdmin()
	_, body := s.get("/admin/courses/" + s.onlyCourseID())
	if strings.Contains(body, "certificates.zip") {
		t.Fatalf("a Course with no Certificate offers Download all Certificates: %s", body)
	}
}
