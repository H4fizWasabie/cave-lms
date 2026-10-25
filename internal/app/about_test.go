package app_test

import (
	"net/http"
	"testing"
)

func TestTheAboutPageGivesTheClientsCompanyBackgroundInBothLanguages(t *testing.T) {
	s := newSite(t)
	res, body := s.getAsking("/about", "en")
	expectStatus(t, res, body, http.StatusOK, "")
	expectLanguage(t, body, "en", "Lifelong Learning for")
	page := content(body)
	expectInOrder(t, page, "HRD Corp Training Provider", "<h1>Lifelong Learning for <em>Efficiency",
		"HRD Corp Registered Training Provider", "Registered with the Ministry of Finance",
		"About us", "registered as an HRD Corp Training Provider and with the Ministry of Finance Malaysia (MOF)",
		"Coach Adam Daniel", "Strategic Intelligence Architect", `href="/#coach"`,
		"including CAVELMS", `href="https://www.thecaveconsult.com/company-profile/"`)
	expectNone(t, page, "Dinor", "ISDeT", "UTM CAIRO")

	_, body = s.getAsking("/about", "ms")
	expectLanguage(t, body, "ms", "Pembelajaran Sepanjang Hayat")
	expectInOrder(t, content(body), "Penyedia Latihan HRD Corp", "<h1>Pembelajaran Sepanjang Hayat untuk <em>Kecekapan",
		"Penyedia Latihan Berdaftar HRD Corp", "Berdaftar dengan Kementerian Kewangan",
		"Tentang kami", "berdaftar sebagai Penyedia Latihan HRD Corp dan dengan Kementerian Kewangan Malaysia (MOF)",
		"Coach Adam Daniel", "Jurulatih NLP Bertauliah", "Pembelajaran Sepanjang Hayat untuk Kecekapan")
}

func TestTheFooterLeadsToTheAboutPageAndTheHomePageDropsItsPlaceholder(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectAll(t, body, `<a href="/about">About TCC</a>`)
	expectNone(t, body, `href="/#background"`, `id="background"`, "Dinor")
}
