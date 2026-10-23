package app_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
)

// pngOfSize is a placeholder for the client's artwork, w by h pixels, so a test can tell which image a PDF carries.
func pngOfSize(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// imageSize is how a PDF records an image w by h pixels.
func imageSize(w, h int) string { return fmt.Sprintf("/Width %d\n/Height %d\n", w, h) }

func TestAdminUploadsAndReplacesTheCertificateBackground(t *testing.T) {
	s := newSite(t)
	_, certID := s.adaEarns()
	path := "/certificates/" + certID + "/pdf"
	s.loginAdmin()

	res, body := s.get("/admin/certificate-settings")
	expectStatus(t, res, body, http.StatusOK, "No background image yet.")
	expectNone(t, body, `name="logo"`, `name="signature"`)

	res, body = s.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/certificate-settings")
	expectAll(t, body, "Background image set.", `<img class="cert-background" src="/admin/certificate-settings/background`)
	res, body = s.get("/admin/certificate-settings/background")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || body != string(pngOfSize(t, 3, 5)) {
		t.Fatalf("background preview = %d %q, want the uploaded PNG", res.StatusCode, res.Header.Get("Content-Type"))
	}
	expectAll(t, s.pdfBody(path), imageSize(3, 5))

	res, body = s.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 7, 9)})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	pdf := s.pdfBody(path)
	expectAll(t, pdf, imageSize(7, 9))
	expectNone(t, pdf, imageSize(3, 5))
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("after replacing the background %d files are stored, want 1", len(entries))
	}
}

func TestLiveCourseCertificatePDFPrintsItsDetailsAndPCCBadge(t *testing.T) {
	admin, ada, id, qids := liveCourse(t, "4")
	res, _ := ada.startExam(id)
	ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	admin.at("2026-07-14 16:30:00") // 15 July in Malaysia
	ada.releaseResults()
	_, body := ada.get("/my-certificates")
	certID := certificateIDs(body)[0]
	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})

	pdf := ada.pdfBody("/certificates/" + certID + "/pdf")
	expectAll(t, pdf, "PCC-2026-00001", "Ada", "951206-08-5555", "Go for Acme",
		"Issued on: 15 July 2026", "https://cave.example.invalid/certificates/"+certID,
		imageSize(3, 5), imageSize(904, 1280)) // the background and the Professional badge
}

func TestSelfPacedCertificateHasNoBadgeAndALongTitleWraps(t *testing.T) {
	s := newSite(t)
	title := "Pengintegrasian Dalam Tadbir Urus Dan Automasi Pedagogi STEM Bagi Program Pembangunan Kompetensi Digital Pegawai"
	id := s.learnerEnrolledIn(title, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")

	pdf := s.pdfBody("/certificates/" + certificateIDs(body)[0] + "/pdf")
	expectAll(t, pdf, "Pengintegrasian", "Pegawai")
	expectNone(t, pdf, title, imageSize(904, 1280))
	if strings.Count(pdf, "/Subtype /Image") != 1 {
		t.Fatalf("self-paced Certificate PDF without a background has %d images, want only the QR code", strings.Count(pdf, "/Subtype /Image"))
	}
}
