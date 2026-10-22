package app_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// adminIn logs in a new Admin who has chosen lang, so every page they open is in it.
func adminIn(t *testing.T, lang string) *site {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.switchLanguage(lang, "/")
	return s
}

func TestTheAdminCourseScreensAreInMalayAndEnglishWithTitlesAsWritten(t *testing.T) {
	s := adminIn(t, "ms")
	s.createCourse("Asas Go", "Belajar Go.", "")
	id := s.onlyCourseID()
	for path, want := range map[string]string{
		"/admin":                           "Bina Kursus",
		"/admin/courses":                   "Kursus Baharu",
		"/admin/courses/new":               "Penerangan",
		"/admin/courses/" + id:             "Pelajar yang menyertai",
		"/admin/courses/" + id + "/edit":   "Sunting Kursus",
		"/admin/courses/" + id + "/delete": "Padam Kursus",
	} {
		_, body := s.get(path)
		expectLanguage(t, body, "ms", want)
		if path != "/admin" && path != "/admin/courses/new" {
			expectAll(t, body, "Asas Go")
		}
	}

	s.switchLanguage("en", "/")
	_, body := s.get("/admin/courses")
	expectLanguage(t, body, "en", "New Course")
	expectAll(t, body, "Asas Go")
}

func TestTheCourseFormErrorsAreInMalay(t *testing.T) {
	s := adminIn(t, "ms")
	for _, c := range []struct{ title, description, start, want string }{
		{"", "Belajar Go.", "", "Masukkan tajuk."},
		{"Asas Go", "", "", "Masukkan penerangan."},
		{"Asas Go", "Belajar Go.", "esok", "Masukkan tarikh mula"},
	} {
		res, body := s.createCourse(c.title, c.description, c.start)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	s.createCourse("Asas Go", "Belajar Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Pemboleh ubah", "", "Isi.", false)
	res, body := s.post("/admin/courses/"+id+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "Kursus ini tidak boleh dipadam")
}

func TestTheLessonScreensAndTheirErrorsAreInMalay(t *testing.T) {
	s := adminIn(t, "ms")
	s.createCourse("Asas Go", "Belajar Go.", "")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Pemboleh ubah", "Guna :=", "Isi.", true)
	lesson := s.lessonIDByTitle("Pemboleh ubah")
	_, body := s.get("/admin/courses/" + id + "/lessons")
	expectLanguage(t, body, "ms", "Terbitkan semua Pelajaran Draf")
	expectAll(t, body, "Pemboleh ubah", "Guna :=")
	_, body = s.get("/admin/courses/" + id + "/lessons/new")
	expectLanguage(t, body, "ms", "Satu Item Galeri")
	_, body = s.get("/admin/lessons/" + lesson + "/edit")
	expectLanguage(t, body, "ms", "Sunting Pelajaran")

	for _, c := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"kind": {"item"}, "title": {"Gambar"}}, "Pilih Item Galeri."},
		{url.Values{"kind": {"text"}, "title": {"Teks"}}, "Tulis teks Pelajaran ini."},
		{url.Values{"kind": {"text"}, "title": {""}, "body": {"Isi."}}, "Masukkan tajuk (sehingga 200 aksara)."},
	} {
		res, body := s.post("/admin/courses/"+id+"/lessons", c.form)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/courses/" + id + "/lessons")
	expectLanguage(t, body, "en", "Publish all Draft Lessons")
}

func TestTheExamScreensAndTheirErrorsAreInMalay(t *testing.T) {
	s := adminIn(t, "ms")
	s.createCourse("Asas Go", "Belajar Go.", "")
	id := s.onlyCourseID()
	exam := "/admin/courses/" + id + "/exam"
	s.saveQuestion(exam+"/questions", "Apakah Go?", [5]string{"Bahasa", "Ular"}, "0", false)
	_, body := s.get(exam)
	expectLanguage(t, body, "ms", "Sedia untuk Pelajar.")
	expectAll(t, body, "Tiada had masa", "Apakah Go?", "Bahasa (betul)")
	_, body = s.get(exam + "/questions/new")
	expectLanguage(t, body, "ms", "Soalan Baharu untuk Asas Go")

	res, body := s.saveExamSettings(id, "101", "", "")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan markah lulus")
	for _, c := range []struct {
		prompt  string
		options [5]string
		correct string
		want    string
	}{
		{"", [5]string{"A", "B"}, "0", "Masukkan soalan."},
		{"Apakah Go?", [5]string{"A"}, "0", "Masukkan antara 2 hingga 5 pilihan."},
		{"Apakah Go?", [5]string{"A", "B"}, "3", "Tandakan pilihan yang betul"},
	} {
		res, body := s.saveQuestion(exam+"/questions", c.prompt, c.options, c.correct, false)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}

	s.switchLanguage("en", "/")
	_, body = s.get(exam)
	expectLanguage(t, body, "en", "Ready for Learners.")
}

func TestTheAdminGalleryScreensAndTheirErrorsAreInMalay(t *testing.T) {
	s := adminIn(t, "ms")
	item := s.uploadAs("Rajah Go", "everyone", pngFile)
	_, body := s.get("/admin/gallery")
	expectLanguage(t, body, "ms", "Muat naik ke Galeri")
	expectAll(t, body, "Rajah Go", "<td>gambar</td>", "<td>Semua orang</td>", "Bahan promosi")
	_, body = s.get("/admin/gallery/new")
	expectLanguage(t, body, "ms", "Pelajar yang log masuk")
	_, body = s.get("/admin" + item + "/edit")
	expectLanguage(t, body, "ms", "Sunting Item Galeri")
	_, body = s.get("/admin" + item + "/delete")
	expectLanguage(t, body, "ms", "Padam &ldquo;Rajah Go&rdquo; dan failnya?")

	res, body := s.upload(titleFields("", "everyone"), pngFile)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan tajuk (sehingga 200 aksara).")
	res, body = s.upload(titleFields("Nota", "everyone"), []byte("plain text"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Muat naik video MP4")
	res, body = s.post("/admin"+item, url.Values{"title": {"Rajah Go"}, "visibility": {"nobody"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Pilih siapa yang boleh melihat item ini.")

	s.createCourse("Asas Go", "Belajar Go.", "")
	s.addItemLesson(s.onlyCourseID(), item[len("/gallery/"):], "Rajah", false)
	res, body = s.post("/admin"+item+"/delete", nil)
	expectStatus(t, res, body, http.StatusConflict, "Item ini tidak boleh dipadam")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/gallery")
	expectLanguage(t, body, "en", "Upload to the Gallery")
	expectAll(t, body, "<td>picture</td>", "<td>Everyone</td>")
}

func TestTheLearnerAndCertificateScreensAndTheirErrorsAreInMalay(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	s.loginAdmin()
	s.switchLanguage("ms", "/")
	today := text.Date("ms", time.Now().UTC())

	_, body := s.get("/admin/courses/" + s.onlyCourseID())
	expectLanguage(t, body, "ms", "<td>1 daripada 1</td>")
	expectAll(t, body, "<td>Dikeluarkan "+today+"</td>", "Buang")
	_, body = s.get("/admin/certificates")
	expectLanguage(t, body, "ms", "Cari mengikut nama atau ID")
	expectAll(t, body, "<td>"+today+"</td>", "<td>Sah</td>")
	_, body = s.get("/admin/certificate-settings")
	expectLanguage(t, body, "ms", "Nama organisasi")

	res, body := s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {"Ada"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Batalkan Sijil ini sebelum mengeluarkan Sijil yang diperbetulkan.")
	s.post("/admin/certificates/"+certID+"/revoke", nil)
	_, body = s.get("/admin/certificates")
	expectAll(t, body, "<td>Dibatalkan</td>", "Keluarkan Sijil yang diperbetulkan")
	res, body = s.post("/admin/certificates/"+certID+"/reissue", url.Values{"learner_name": {""}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan nama untuk Sijil")

	res, body = s.postCertificateSettings(map[string]string{"organisation_name": "", "signatory_name": ""}, nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Masukkan nama organisasi dan nama penandatangan.")
	res, body = s.postCertificateSettings(map[string]string{"organisation_name": "TCC", "signatory_name": "Adam"}, map[string][]byte{"logo": []byte("not an image")})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Logo mestilah imej JPEG atau PNG.")

	s.switchLanguage("en", "/")
	_, body = s.get("/admin/certificates")
	expectLanguage(t, body, "en", "Search by name or ID")
	expectAll(t, body, "<td>Revoked</td>")
}

func TestTheAdminOnlyAndQuestionNotFoundPagesAreInMalay(t *testing.T) {
	s := adminIn(t, "ms")
	s.createCourse("Asas Go", "Belajar Go.", "")
	res, body := s.get("/admin/courses/" + s.onlyCourseID() + "/exam/questions/999/edit")
	expectStatus(t, res, body, http.StatusNotFound, "Soalan tidak ditemui")

	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.switchLanguage("ms", "/")
	res, body = ada.get("/admin")
	expectStatus(t, res, body, http.StatusForbidden, "Hanya Pentadbir boleh membuka halaman ini.")
}
