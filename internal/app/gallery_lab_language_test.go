package app_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
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
	visitor := newSite(t)
	_, body := visitor.getAsking("/lab", "ms")
	expectLanguage(t, body, "ms", "Daripada dashboard statik kepada dashboard yang dihoskan")
	expectAll(t, body, "Daftar untuk bermula", "Templat, dipaparkan dengan Sampelnya")
	_, body = visitor.getAsking("/lab", "en")
	expectLanguage(t, body, "en", "From static dashboard to hosted dashboard")

	admin, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	_, body = ada.get("/lab")
	expectLanguage(t, body, "ms", "Deployment aktif")
	expectAll(t, body, "0 daripada 3", "Dashboard saya", "Belum ada Dashboard.")
	_, body = ada.get("/lab/new?t=finance")
	expectLanguage(t, body, "ms", "Apa yang diperlukan oleh hamparan kerja anda")
	expectAll(t, body, "Cipta Dashboard", `value="Dashboard Finance saya"`)
	res, body := ada.post("/lab/dashboards", url.Values{"template": {"finance"}, "name": {""}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Berikan nama kepada Dashboard.")

	path := ada.createDashboard("finance", "Kewangan")
	_, body = ada.get(path)
	expectLanguage(t, body, "ms", "Muat naik Set data anda")
	res, body = ada.postFile(path+"/dataset", "notes.csv", []byte("a,b"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "notes.csv bukan fail .xlsx.")
	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	_, body = ada.get(path + "/columns")
	expectLanguage(t, body, "ms", "Lajur mana yang mengisi setiap ruang?")
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
	_, body := s.get("/lab")
	expectLanguage(t, body, "ms", "Hubungi kami untuk naik taraf")
	res, body := s.get("/lab/new")
	expectStatus(t, res, body, http.StatusForbidden, "Lab dikunci")
}

func TestTheLabsFileAndColumnErrorsAreInMalay(t *testing.T) {
	_, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	path := ada.createDashboard("finance", "Kewangan")
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	bad := testutil.XLSX(t, []string{"Tarikh", "Jumlah"}, []any{day, "dua ratus"}, []any{"31/02/2026", 5}, []any{day, nil})
	res, body := ada.postFile(path+"/dataset", "bad.xlsx", bad)
	for _, want := range []string{"Baris 2, Jumlah: bukan nombor (&#34;dua ratus&#34;).", "Baris 3, Tarikh: bukan tarikh",
		"Baris 4, Jumlah: kosong, tetapi lajur ini wajib diisi."} {
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	res, body = ada.postFile(path+"/dataset", "renamed.xlsx", testutil.XLSX(t, []string{"Tarikh", "Jumlah Baru"}, []any{day, "x"}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Lajur Jumlah tiada. Templat Finance memerlukannya untuk Amount, iaitu lajur nombor.")
	res, body = ada.postFile(path+"/dataset", "fake.xlsx", []byte("not a workbook"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Ini bukan fail .xlsx. Lab membaca buku kerja Excel sahaja.")

	ada.postFile(path+"/dataset", "finance-sample.xlsx", ada.financeSample())
	res, body = ada.post(path+"/columns", url.Values{})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Pilih lajur untuk Date.")
	res, body = ada.post(path+"/columns", url.Values{"date": {"Jumlah"}, "amount": {"Jumlah"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Date memerlukan lajur tarikh, tetapi Jumlah mengandungi nombor.")
}
