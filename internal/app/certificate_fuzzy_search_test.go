package app_test

import (
	"net/http"
	"net/url"
	"testing"
)

// namedCertificate is passedLiveCourse with the Certificate's name copy set to name, as if the Learner had that
// name when it was issued. The Admin is still logged in.
func namedCertificate(t *testing.T, name string) (admin *site, certID string) {
	t.Helper()
	admin, _, _, certID = passedLiveCourse(t)
	if _, err := admin.db.Exec(`UPDATE certificates SET learner_name = $1`, name); err != nil {
		t.Fatal(err)
	}
	return admin, certID
}

func TestAdminFindsACertificateByPartOfTheName(t *testing.T) {
	t.Parallel()
	admin, certID := namedCertificate(t, "Nurul Aisyah")
	for _, q := range []string{"aisy", "NURUL", "rul ais"} {
		res, body := admin.get("/admin/certificates?q=" + url.QueryEscape(q))
		expectStatus(t, res, body, http.StatusOK, certID)
	}
	res, body := admin.get("/admin/certificates?q=Zainal")
	expectStatus(t, res, body, http.StatusOK, "No Certificates match.")
	expectNone(t, body, certID)
}

func TestAdminFindsACertificateDespiteATypo(t *testing.T) {
	t.Parallel()
	admin, certID := namedCertificate(t, "Nurul Aisyah")
	for _, q := range []string{"Nurl Aisyah", "Nurul Aisya", "nurul aysah"} {
		res, body := admin.get("/admin/certificates?q=" + url.QueryEscape(q))
		expectStatus(t, res, body, http.StatusOK, certID)
	}
}

func TestWildcardsInTheCertificateSearchAreLiteral(t *testing.T) {
	t.Parallel()
	admin, certID := namedCertificate(t, "Nurul Aisyah")
	for _, q := range []string{"%", "_", "Nu%h", `\`} {
		res, body := admin.get("/admin/certificates?q=" + url.QueryEscape(q))
		expectStatus(t, res, body, http.StatusOK, "No Certificates match.")
		expectNone(t, body, certID)
	}
}
