package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func TestTheCoursePagesAreInMalayAndEnglishWithTheCourseAsWritten(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse("Go Basics", "Learn Go from scratch.", "2026-11-03")
	id := s.onlyCourseID()
	s.addTextLesson(id, "Variables", "", "Body.", false)
	s.addTextLesson(id, "Loops", "", "Body.", false)
	s.logOut()

	_, body := s.getAsking("/courses", "ms")
	expectLanguage(t, body, "ms", "Mula bila-bila masa dan sertai dengan satu klik.")
	expectAll(t, body, "Go Basics", "Learn Go from scratch.", "Dibuka untuk penyertaan baharu", "Tarikh mula: 3 November 2026", "Lihat Kursus")
	_, body = s.getAsking("/courses", "en")
	expectLanguage(t, body, "en", "Start any time and join in one click.")
	expectAll(t, body, "Open to new joins", "Start date: 3 November 2026", "View course")

	_, body = s.getAsking("/courses/"+id, "ms")
	expectLanguage(t, body, "ms", `atau <a href="/register">daftar</a> untuk menyertai Kursus ini.`)
	expectAll(t, body, "Go Basics", "Variables", "Pelajaran")

	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	s.switchLanguage("ms", "/")
	s.post("/courses/"+id+"/join", nil)
	s.markLesson(id, s.lessonIDByTitle("Variables"), true)
	_, body = s.getAsking("/courses/"+id, "en")
	expectLanguage(t, body, "ms", "Kemajuan: 1 daripada 2 Pelajaran selesai")
	expectAll(t, body, "Anda telah menyertai Kursus ini", "Tandakan belum selesai", "Tandakan selesai", "Tinggalkan Kursus ini", "Peperiksaan")
	_, body = s.getAsking("/my-courses", "en")
	expectLanguage(t, body, "ms", "Kursus yang telah anda sertai.")
	expectAll(t, body, "Go Basics", "Sijil Saya", "Tarikh mula: 3 November 2026")

	s.switchLanguage("en", "/")
	_, body = s.getAsking("/courses/"+id, "ms")
	expectLanguage(t, body, "en", "Progress: 1 of 2 Lessons complete")
	_, body = s.getAsking("/my-courses", "ms")
	expectLanguage(t, body, "en", "Courses you have joined.")
}

func TestTheLessonPagesAreInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse("Excel Dashboards", "Dashboards from Excel.", "")
	id := s.courseIDByTitle("Excel Dashboards")
	s.addTextLesson(id, "Pivot tables", "Build a pivot table.", "Drag the *fields*.", false)
	lesson := "/courses/" + id + "/lessons/" + s.lessonIDByTitle("Pivot tables")
	s.logOut()

	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	_, body := s.getAsking(lesson, "ms")
	expectLanguage(t, body, "ms", "Ahli sahaja")
	expectAll(t, body, "Sertai Kursus ini untuk membuka Pelajarannya.", "Sertai Kursus ini</button>")
	_, body = s.postAsking(lesson+"/complete", "ms", nil)
	expectLanguage(t, body, "ms", "Sertai Kursus ini untuk menandakan Pelajarannya selesai.")

	s.post("/courses/"+id+"/join", nil)
	_, body = s.getAsking(lesson, "ms")
	expectLanguage(t, body, "ms", "Kembali ke Kursus")
	expectAll(t, body, "<strong>Objektif:</strong> Build a pivot table.", "<em>fields</em>")
	_, body = s.getAsking(lesson, "en")
	expectLanguage(t, body, "en", "Back to the Course")

	_, body = s.getAsking("/courses/999999", "ms")
	expectLanguage(t, body, "ms", "Kursus tidak dijumpai")
}

func TestTheExamPagesAreInMalayAndEnglishWithTheQuestionsAsWritten(t *testing.T) {
	s := newSite(t)
	id, qids := s.examCourse("50", "10", "2", "What does := do?")
	s.switchLanguage("ms", "/")
	exam := "/courses/" + id + "/exam"

	_, body := s.get(exam)
	expectLanguage(t, body, "ms", "Peperiksaan untuk Go Basics")
	expectAll(t, body, "Markah lulus: 50%", "Had masa: 10 minit", "Percubaan berbaki: 2", "Belum ada markah", "Mulakan Peperiksaan", "Kembali ke Kursus")

	res, _ := s.startExam(id)
	attempt := res.Header.Get("Location")
	_, body = s.get(attempt)
	expectLanguage(t, body, "ms", "Had masa: 10 minit, dikira dari saat anda bermula.")
	expectAll(t, body, "What does := do?", "Right answer", "Jawab setiap soalan, kemudian hantar.", "Hantar jawapan", "Kembali ke Peperiksaan")
	_, body = s.get(exam)
	expectLanguage(t, body, "ms", "Teruskan percubaan anda")

	s.submitExam(attempt, map[string]string{qids[0]: "1"})
	_, body = s.get(attempt)
	expectLanguage(t, body, "ms", "Tahniah, jawapan anda telah dihantar")
	expectAll(t, body, "Keputusan anda sedia pada ", "Ia juga akan dihantar kepada anda melalui e-mel.")
	_, body = s.get(exam)
	expectAll(t, body, "Telah dihantar, Keputusan anda sedia pada ")
	s.releaseResults()
	_, body = s.get(attempt)
	expectLanguage(t, body, "ms", "Gagal (markah lulus 50%)")
	expectAll(t, body, "Markah: 0%")
	_, body = s.get(exam)
	expectAll(t, body, "Percubaan berbaki: 1", "Markah terbaik: 0%")

	s.switchLanguage("en", "/")
	_, body = s.get(attempt)
	expectLanguage(t, body, "en", "Failed (pass mark 50%)")

	res, _ = s.startExam(id)
	attempt = res.Header.Get("Location")
	s.switchLanguage("ms", "/")
	s.submitExam(attempt, map[string]string{qids[0]: "0"})
	s.releaseResults()
	_, body = s.get(attempt)
	expectLanguage(t, body, "ms", "Lulus (markah lulus 50%)")
	_, body = s.get(exam)
	expectLanguage(t, body, "ms", "Anda telah lulus Peperiksaan ini.")
	res, body = s.startExam(id)
	expectStatus(t, res, body, http.StatusConflict, "Anda telah lulus Peperiksaan ini.")
	expectLanguage(t, body, "ms", "Percubaan berbaki: 0")
}

func TestAnExamWithNoLimitsAndATimedOutAttemptAreInMalay(t *testing.T) {
	s := newSite(t)
	id, _ := s.examCourse("50", "", "", "What does := do?")
	s.switchLanguage("ms", "/")
	_, body := s.get("/courses/" + id + "/exam")
	expectLanguage(t, body, "ms", "Tiada had masa")
	expectAll(t, body, "Percubaan berbaki: Tanpa had")

	s2 := newSite(t)
	id, _ = s2.examCourse("50", "1", "", "What does := do?")
	s2.switchLanguage("ms", "/")
	res, _ := s2.startExam(id)
	attempt := res.Header.Get("Location")
	s2.ageAttempt(attempt, 600)
	_, body = s2.get(attempt)
	expectLanguage(t, body, "ms", "Masa tamat sebelum jawapan anda dihantar, jadi tiada jawapan yang dikira.")
}

func TestCertificatesAndTheirCheckPageAreInTheRequestsLanguage(t *testing.T) {
	s := newSite(t)
	certID := s.earnedCertificate()
	var issued time.Time
	if err := s.db.QueryRow(`SELECT issued_at FROM certificates WHERE id = $1`, certID).Scan(&issued); err != nil {
		t.Fatal(err)
	}
	issuedMS, issuedEN := text.Date("ms", issued.In(time.FixedZone("MYT", 8*3600))), text.Date("en", issued.In(time.FixedZone("MYT", 8*3600)))

	_, body := s.getAsking("/my-certificates", "ms")
	expectLanguage(t, body, "ms", "Sijil Saya")
	expectAll(t, body, "Go Basics", "Dikeluarkan "+issuedMS, "Muat turun PDF", "Setiap sijil mempunyai ID")

	s.switchLanguage("ms", "/")
	pdf := s.pdfBody("/certificates/" + certID + "/pdf")
	expectAll(t, pdf, "Ada", "Go Basics", "Dikeluarkan pada: "+issuedMS)
	s.switchLanguage("en", "/")
	pdf = s.pdfBody("/certificates/" + certID + "/pdf")
	expectAll(t, pdf, "Issued on: "+issuedEN)
	s.logOut()

	_, body = s.getAsking("/certificates", "ms")
	expectLanguage(t, body, "ms", "Semak Sijil")
	expectAll(t, body, "ID Sijil", "Semak</button>")
	other := s.second()
	_, body = other.getAsking("/certificates/"+certID, "ms")
	expectLanguage(t, body, "ms", "Status: Sah")
	expectAll(t, body, "Dengan ini disahkan bahawa <strong>Ada</strong>", "telah menyempurnakan <strong>Go Basics</strong>", "Dikeluarkan "+issuedMS)
	_, body = other.getAsking("/certificates/"+certID, "en")
	expectLanguage(t, body, "en", "Status: Valid")
	expectAll(t, body, "Issued "+issuedEN)
	res, body := other.getAsking("/certificates/nope", "ms")
	expectStatus(t, res, body, http.StatusNotFound, "Sijil tidak dijumpai")

	s.loginAdmin()
	s.post("/admin/certificates/"+certID+"/revoke", confirming)
	_, body = other.getAsking("/certificates/"+certID, "ms")
	expectLanguage(t, body, "ms", "Status: Dibatalkan")
}
