package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// orgLogoImage is the Organisation's logo in a Certificate preview: in the PDF's box, scaled to fit and centred in it.
var orgLogoImage = regexp.MustCompile(`<image href="(/certificate-logo/[^"]+)" x="45" y="10" width="55" height="30" preserveAspectRatio="xMidYMid meet"/>`)

func TestThePreviewDrawsTheOrganisationsLogoInThePDFsBoxAndTheCertificateKeepsItsOwn(t *testing.T) {
	t.Parallel()
	admin, ada, org, _ := orgCertificate(t, pngOfSize(t, 11, 13))
	var courseID string
	if err := admin.db.QueryRow(`SELECT id FROM courses WHERE organisation_id = $1`, org).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	logoOn := func(path string) string {
		t.Helper()
		_, body := ada.get(path)
		m := orgLogoImage.FindStringSubmatch(certArt(t, body))
		if m == nil {
			t.Fatalf("%s: preview does not draw the Organisation's logo: %s", path, certArt(t, body))
		}
		return m[1]
	}
	issued := logoOn("/my-certificates")
	if sample := logoOn("/courses/" + courseID); sample != issued {
		t.Fatalf("the Course page's sample draws %s, want the logo %s", sample, issued)
	}

	visitor := ada.second() // the logo is public, like the artwork
	res, body := visitor.get(issued)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(body, "\x89PNG") {
		t.Fatalf("GET %s = %d %q, want the logo PNG", issued, res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res, _ := visitor.get("/certificate-logo/nosuchkey"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET an unknown logo = %d, want 404", res.StatusCode)
	}
	admin.postCertificateSettings(nil, map[string][]byte{"background": pngOfSize(t, 3, 5)})
	_, body = ada.get("/my-certificates")
	bg := regexp.MustCompile(`/certificate-background/([^"]+)`).FindStringSubmatch(body)
	if bg == nil {
		t.Fatalf("no background address on My certificates: %s", body)
	}
	if res, _ := visitor.get("/certificate-logo/" + bg[1]); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET the background under /certificate-logo = %d, want 404", res.StatusCode)
	}

	res, body = admin.saveOrganisation("/admin/organisations/"+org, "PDRM", pngOfSize(t, 17, 19)) // after issue
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if again := logoOn("/my-certificates"); again != issued {
		t.Fatalf("the issued Certificate's preview draws %s after the logo changed, want %s as issued", again, issued)
	}
	if sample := logoOn("/courses/" + courseID); sample == issued {
		t.Fatal("the Course page's sample still draws the old logo, want the Organisation's current one")
	}
	if res, _ := visitor.get(issued); res.StatusCode != http.StatusOK {
		t.Fatalf("GET the logo as issued after it changed = %d, want 200: the Certificate still prints it", res.StatusCode)
	}
}

func TestThePreviewDrawsNoLogoWithoutAnOrganisation(t *testing.T) {
	t.Parallel()
	admin, ada, _, _ := orgCertificate(t, nil)
	var courseID string
	if err := admin.db.QueryRow(`SELECT id FROM courses WHERE title = 'Go for Acme'`).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/my-certificates", "/courses/" + courseID} {
		_, body := ada.get(path)
		expectNone(t, certArt(t, body), "certificate-logo")
	}
}
