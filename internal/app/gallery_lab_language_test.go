package app_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func TestTheGalleryPagesAreInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Poster Kursus", "everyone", pngFile)
	s.logOut()

	_, body := s.getAsking("/gallery", "ms")
	expectLanguage(t, body, "ms", "Video, ebook dan gambar.")
	expectAll(t, body, "<h1>Galeri</h1>", ">Tapis</button>", ">Gambar</option>", "<strong>Poster Kursus</strong><span>Gambar</span>")
	_, body = s.getAsking("/gallery", "en")
	expectLanguage(t, body, "en", "Videos, ebooks and pictures.")
	expectAll(t, body, ">Filter</button>", "<strong>Poster Kursus</strong><span>Picture</span>")

	_, body = s.getAsking("/gallery?format=ebook", "ms")
	expectLanguage(t, body, "ms", "eBook akan tiba tidak lama lagi.")
	_, body = s.getAsking("/gallery?format=video", "ms")
	expectLanguage(t, body, "ms", "Belum ada apa-apa di sini.")

	_, body = s.getAsking(location, "ms")
	expectLanguage(t, body, "ms", "Kembali ke Galeri")
	expectAll(t, body, "<h1>Poster Kursus</h1>")
	_, body = s.getAsking(location, "en")
	expectLanguage(t, body, "en", "Back to the Gallery")
}

func TestARefusedMembersItemSaysWhyInMalay(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	location := s.uploadAs("Members picture", "course_members", pngFile)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := ada.getAsking(location, "ms")
	expectStatus(t, res, body, http.StatusForbidden, "Item ini hanya untuk ahli Kursusnya.")
}

func TestTheNotFoundAndErrorPagesAreInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	for _, path := range []string{"/no-such-page", "/gallery/999"} {
		res, body := s.getAsking(path, "ms")
		expectStatus(t, res, body, http.StatusNotFound, "Halaman tidak dijumpai")
		expectLanguage(t, body, "ms", "Tiada apa-apa di alamat ini.")
		res, body = s.getAsking(path, "en")
		expectStatus(t, res, body, http.StatusNotFound, "Page not found")
		expectLanguage(t, body, "en", "There is nothing at this address.")
	}

	restore := breakCourses(t, s)
	defer restore()
	res, body := s.getAsking("/courses", "ms")
	expectStatus(t, res, body, http.StatusInternalServerError, "Berlaku masalah")
	expectLanguage(t, body, "ms", "CAVELMS menghadapi masalah")
	res, body = s.getAsking("/courses", "en")
	expectStatus(t, res, body, http.StatusInternalServerError, "Something went wrong")
	expectLanguage(t, body, "en", "CAVELMS had a problem")
}

func TestTheLabIsInMalayAndEnglish(t *testing.T) {
	admin, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	_, body := ada.get("/lab")
	expectLanguage(t, body, "ms", "Deployment aktif")
	expectAll(t, body, "0 daripada 3", "Dashboard saya", "Belum ada Dashboard.")
	_, body = ada.get("/lab/new")
	expectLanguage(t, body, "ms", "Namakan Dashboard anda, kemudian tampal HTML yang ditulis oleh chatbot AI anda")
	expectAll(t, body, "Cipta Dashboard", "Tampal HTML", "Atau pilih fail .html")
	res, body := ada.post("/lab/dashboards", url.Values{"name": {""}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Berikan nama kepada Dashboard.")

	path := ada.createWork("finance", "Kewangan")
	_, body = ada.get(path)
	expectLanguage(t, body, "ms", "Muat naik Set data anda")
	res, body = ada.postFile(path+"/dataset", "notes.csv", []byte("a,b"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "notes.csv bukan fail .xlsx.")
	ada.confirmFile(path, "finance-sample.xlsx", ada.financeSample())
	_, body = ada.get(path + "/hood")
	expectLanguage(t, body, "ms", "Hamparan kerja anda, sebagai data")
	_, body = ada.get(path + "/deploy")
	expectLanguage(t, body, "ms", "Letakkan Dashboard ini dalam talian")
	link := ada.deploy(path)
	_, body = ada.get(path + "/deploy")
	expectLanguage(t, body, "ms", "Dalam talian")

	admin.ageAllPeriods(31)
	_, body = ada.second().getAsking(link, "ms")
	expectLanguage(t, body, "ms", "Dashboard ini sedang tidur")
	_, body = ada.second().getAsking("/d/nope/", "ms")
	expectLanguage(t, body, "ms", "Tiada Dashboard di pautan ini.")
}

func TestTheLockedLabSaysSoInMalay(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.switchLanguage("ms", "/")
	res, body := s.get("/lab/new")
	expectStatus(t, res, body, http.StatusForbidden, "Lab dikunci")
}

func TestTheLabsFileErrorsAreInMalay(t *testing.T) {
	_, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	path := ada.createWork("finance", "Kewangan")
	res, body := ada.postFile(path+"/dataset", "fake.xlsx", []byte("not a workbook"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Ini bukan fail .xlsx. Lab membaca buku kerja Excel sahaja.")
	res, body = ada.postFile(path+"/dataset", "old.xls", []byte("old"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Ini fail Excel lama (Excel 97-2003, .xls).")
}

func TestTheLabsTemplatesAreNamedAndExplainedInTheVisitorsLanguage(t *testing.T) {
	_, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	ada.createWork("finance", "Duit saya")
	_, body := ada.get("/lab")
	expectAll(t, body, "Templat Kewangan")
	expectNone(t, body, "Templat Finance")
}

func TestEveryTemplateHasItsLabWordsInTheCatalogMatchingItsEnglish(t *testing.T) {
	for _, tp := range lab.Templates {
		words := map[string]string{"name": tp.Name, "blurb": tp.Blurb, "question": tp.Question}
		for _, b := range tp.Blanks {
			words[b.Key+".help"] = b.Help
		}
		for k, en := range words {
			key := "lab.tpl." + tp.Key + "." + k
			if en == "" {
				continue
			}
			if got := text.T("en", key); got != en {
				t.Errorf("%s in English = %q, want the Template's %q", key, got, en)
			}
			if got := text.T("ms", key); got == key || got == "" {
				t.Errorf("%s has no Malay", key)
			}
		}
	}
}
