package app_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
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
	expectNone(t, pdf, title, imageSize(904, 1280), "/Width 904\n") // no PCC badge, of any Level
}

// certTitleLine is a Course title line in a Certificate preview: its font size in mm and its text.
var certTitleLine = regexp.MustCompile(`<text class="cert-art-title"[^>]* font-size="([\d.]+)"[^>]*>([^<]*)</text>`)

func TestALongUnbrokenWordInACourseTitleBreaksInsideTheTitleBoxOnThePDFAndThePreview(t *testing.T) {
	s := newSite(t)
	title := strings.Repeat("Kompetensi", 12) // 120 characters, no space
	id := s.learnerEnrolledIn(title, "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")
	pdf := s.pdfBody("/certificates/" + certificateIDs(body)[0] + "/pdf")
	_, body = s.get("/courses/" + id)

	lines := certTitleLine.FindAllStringSubmatch(certArt(t, body), -1)
	measure := fpdf.New("P", "mm", "A4", "")
	var joined string
	for _, l := range lines {
		mm, _ := strconv.ParseFloat(l[1], 64)
		measure.SetFont("Times", "B", mm*72/25.4)
		if w := measure.GetStringWidth(l[2]); w > 150.01 { // the title box is 150 mm wide, from 30 mm
			t.Errorf("the preview's title line %q is %.1f mm wide, want at most 150", l[2], w)
		}
		if _, left := nameOnPDF(t, pdf, l[2]); left < 29.99 {
			t.Errorf("the PDF's title line %q starts at %.1f mm, want inside the box from 30 mm", l[2], left)
		}
		joined += l[2]
	}
	if joined != title {
		t.Fatalf("the title lines %q do not spell the title", lines)
	}
}

// The built-in artwork's wording and its image assets ("(CERTIFICATE)" as fpdf writes the line, so it doesn't match the
// "CERTIFICATE NO." label every Certificate prints): the CAVE logo (drawn from its SVG at 1200 pixels wide), the seal,
// the HRD Corp badge and the two signatures.
var builtInArtwork = []string{"(CERTIFICATE)", "OF COMPETENCY", "THIS IS TO CERTIFY THAT",
	"has successfully completed the competency requirements of", "Professional Competency Credential",
	"the competencies demonstrated throughout the programme", "ADAM BIN YUSUF", "DINOR SABUDIN",
	imageSize(1200, 539), imageSize(316, 312), imageSize(206, 200), imageSize(284, 76), imageSize(146, 90)}

func TestACertificatePDFDrawsTheBuiltInArtworkUntilABackgroundIsUploaded(t *testing.T) {
	s := newSite(t)
	_, certID := s.adaEarns()
	path := "/certificates/" + certID + "/pdf"
	pdf := s.pdfBody(path)
	expectAll(t, pdf, append(builtInArtwork, "CERTIFICATE NO.")...)
	expectNone(t, pdf, "demonsrated", "throughtout", "KEMENTERIAN")

	s.loginAdmin()
	s.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})
	pdf = s.pdfBody(path)
	expectAll(t, pdf, imageSize(3, 5), "Ada", "Go Basics", "CERTIFICATE NO.") // the uploaded artwork has no label
	expectNone(t, pdf, builtInArtwork...)                                     // the upload carries its own
}

// orgCertificate has Ada pass a 2-day Live Course run for an Organisation with logo ("" logo for none) and returns
// the sites and her Certificate's ID.
func orgCertificate(t *testing.T, logo []byte) (admin, ada *site, org, certID string) {
	t.Helper()
	admin, ada, id, qids := liveCourse(t, "2")
	if logo != nil {
		org = admin.addOrganisation("PDRM", logo)
		start, end := liveDates()
		res, body := admin.post("/admin/courses/"+id, url.Values{"title": {"Go for Acme"}, "description": {"For Acme."},
			"start_date": {start}, "end_date": {end}, "days": {"2"}, "organisation_id": {org}})
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	res, _ := ada.startExam(id)
	ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	ada.releaseResults()
	_, body := ada.get("/my-certificates")
	return admin, ada, org, certificateIDs(body)[0]
}

// hasLogo reports whether the Certificate recorded a logo.
func hasLogo(t *testing.T, s *site, certID string) bool {
	t.Helper()
	var has bool
	if err := s.db.QueryRow(`SELECT logo_key IS NOT NULL FROM certificates WHERE id = $1`, certID).Scan(&has); err != nil {
		t.Fatal(err)
	}
	return has
}

func TestALiveCourseCertificatePrintsItsOrganisationsLogoFromIssueUntilReissued(t *testing.T) {
	admin, ada, org, certID := orgCertificate(t, pngOfSize(t, 11, 13))
	path := "/certificates/" + certID + "/pdf"
	if !hasLogo(t, admin, certID) {
		t.Fatal("the Certificate recorded no logo")
	}
	expectAll(t, ada.pdfBody(path), imageSize(11, 13))

	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})
	res, body := admin.saveOrganisation("/admin/organisations/"+org, "PDRM", pngOfSize(t, 17, 19))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	pdf := ada.pdfBody(path)
	expectAll(t, pdf, imageSize(11, 13), imageSize(3, 5)) // on the artwork too, and as issued
	expectNone(t, pdf, imageSize(17, 19))

	admin.post("/admin/certificates/"+certID+"/revoke", confirming)
	res, body = admin.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada Lovelace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/my-certificates")
	ids := certificateIDs(body)
	pdf = ada.pdfBody("/certificates/" + ids[len(ids)-1] + "/pdf")
	expectAll(t, pdf, imageSize(17, 19))
	expectNone(t, pdf, imageSize(11, 13))
}

func TestCertificatesWithoutAnOrganisationRecordAndPrintNoLogo(t *testing.T) {
	admin, ada, _, certID := orgCertificate(t, nil)
	if hasLogo(t, admin, certID) {
		t.Fatal("a Live Course without an Organisation recorded a logo")
	}
	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)}) // so every image is known
	if n := strings.Count(ada.pdfBody("/certificates/"+certID+"/pdf"), "/Subtype /Image"); n != 5 {
		t.Fatalf("Certificate PDF has %d images, want the background and the badge, each with its transparency mask, and the QR code", n)
	}

	s := newSite(t)
	_, certID = s.adaEarns()
	if hasLogo(t, s, certID) {
		t.Fatal("a self-paced Certificate recorded a logo")
	}
}

// nameOnPDF finds name printed in pdf and returns its font size in points and its left edge in mm.
func nameOnPDF(t *testing.T, pdf, name string) (size, left float64) {
	t.Helper()
	at := strings.Index(pdf, " Td ("+name+") Tj")
	tf := strings.LastIndex(pdf[:at], " Tf ET")
	if at < 0 || tf < 0 {
		t.Fatalf("the PDF does not print %q", name)
	}
	fmt.Sscan(pdf[strings.LastIndex(pdf[:tf], " ")+1:tf], &size)
	fmt.Sscan(pdf[strings.LastIndex(pdf[:at], "BT ")+3:], &left)
	return size, left / 72 * 25.4
}

func TestTheLearnersNamePrintsAt14PtAsOnTheSampleAndShrinksToFitItsRule(t *testing.T) {
	admin, ada, _, certID := orgCertificate(t, nil)
	if size, _ := nameOnPDF(t, ada.pdfBody("/certificates/"+certID+"/pdf"), "Ada"); size != 14 {
		t.Fatalf("a short name prints at %v pt, want 14", size)
	}

	long := "Puteri Nur Aisyah Humaira binti Mohd Shahrizal Abdul Wahhabi Al-Haddad Syed Zainal Abidin" // 89 characters
	admin.post("/admin/certificates/"+certID+"/revoke", confirming)
	res, body := admin.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {long}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/my-certificates")
	ids := certificateIDs(body)
	size, left := nameOnPDF(t, ada.pdfBody("/certificates/"+ids[len(ids)-1]+"/pdf"), long)
	if size >= 14 || left < 21 { // the rule under the name runs from 21 mm to 189 mm, centred on the page
		t.Fatalf("an 89-character name prints at %v pt from %.1f mm, want it shrunk to start on the rule", size, left)
	}
}

func TestAnSVGLogoPrintsOnTheCertificate(t *testing.T) {
	_, ada, _, certID := orgCertificate(t, svgLogo)
	expectAll(t, ada.pdfBody("/certificates/"+certID+"/pdf"), imageSize(1200, 600))
}

func TestACourseTitleWithAccentsAndDashesPrintsOnTheCertificate(t *testing.T) {
	s := newSite(t)
	id := s.learnerEnrolledIn("Café — “Basics”", "Variables")
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body := s.get("/my-certificates")

	// The PDF's core fonts store text as Windows-1252 bytes: é is \xe9, the em dash \x97, curly quotes \x93 and \x94.
	expectAll(t, s.pdfBody("/certificates/"+certificateIDs(body)[0]+"/pdf"), "Caf\xe9 \x97 \x93Basics\x94")
}

// The minutely release job issues Live Course Certificates outside any request, where a panic would end the server.
func TestTheReleaseJobIssuesACertificateForACourseTitleWithAnEmDash(t *testing.T) {
	admin, ada, id, qids := liveCourse(t, "1")
	start, end := liveDates()
	res, body := admin.post("/admin/courses/"+id, url.Values{"title": {"Go — for Acme"}, "description": {"For Acme."},
		"start_date": {start}, "end_date": {end}, "days": {"1"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, _ = ada.startExam(id)
	ada.submitExam(res.Header.Get("Location"), map[string]string{qids[0]: "0"})
	ada.releaseResults()

	_, body = ada.get("/my-certificates")
	certID := certificateIDs(body)[0]
	expectAll(t, ada.pdfBody("/certificates/"+certID+"/pdf"), "Go \x97 for Acme")
	emails := ada.emailsTo("ada@example.com") // Malay, the default language
	certEmails := withSubject(emails, "Sijil anda untuk Go — for Acme")
	if len(certEmails) != 1 || len(certEmails[0].Attachments) != 1 {
		t.Fatalf("%d Certificate emails, want one with the PDF", len(certEmails))
	}
	expectAll(t, string(certEmails[0].Attachments[0].Content), "Go \x97 for Acme")
	if len(withSubject(emails, "Keputusan Peperiksaan Objektif anda untuk Go — for Acme")) != 1 {
		t.Fatal("the Result email was not sent")
	}
}
