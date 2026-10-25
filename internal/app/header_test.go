package app_test

import (
	"net/http"
	"strings"
	"testing"
)

// header is the part of a page above its content: the site header.
func header(t *testing.T, body string) string {
	t.Helper()
	head, _, ok := strings.Cut(body, "<main>")
	if !ok {
		t.Fatalf("page has no <main>; body:\n%s", body)
	}
	return head
}

// content is the part of a page below its header.
func content(body string) string {
	_, c, _ := strings.Cut(body, "<main>")
	return c
}

// expectInOrder fails unless each of wants appears in body after the one before it.
func expectInOrder(t *testing.T, body string, wants ...string) {
	t.Helper()
	rest := body
	for _, want := range wants {
		_, after, ok := strings.Cut(rest, want)
		if !ok {
			t.Fatalf("lacks %q after the ones before it; body:\n%s", want, body)
		}
		rest = after
	}
}

func TestTheHeaderShowsTheEightSectionsInMalayAndEnglish(t *testing.T) {
	s := newSite(t)
	for _, path := range []string{"/", "/courses", "/certificates"} {
		_, body := s.getAsking(path, "en")
		head := header(t, body)
		expectInOrder(t, head, ">Home<", ">Courses<", ">Lab<", ">Classroom<", ">Pricing<", ">Program<", ">Certification<", ">Community<")
		_, body = s.getAsking(path, "ms")
		head = header(t, body)
		expectInOrder(t, head, ">Utama<", ">Kursus<", ">Lab<", ">Kelas<", ">Harga<", ">Program<", ">Pensijilan<", ">Komuniti<")
	}
}

func TestTheCoursesMenuListsTheThreeLatestOpenCoursesThenAllCourses(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse("Alpha", "First.", "")
	_, body := s.get("/lab")
	expectInOrder(t, header(t, body), ">Alpha<", ">All courses<")
	for _, title := range []string{"Beta", "Gamma", "Delta", "Epsilon"} {
		s.createCourse(title, "More.", "")
	}
	s.post("/admin/courses/"+s.courseIDByTitle("Epsilon")+"/close", nil)
	s.logOut()

	_, body = s.get("/lab")
	head := header(t, body)
	expectInOrder(t, head, ">Delta<", ">Gamma<", ">Beta<", ">All courses<")
	expectNone(t, head, "Alpha", "Epsilon")
}

func TestTheErrorPageKeepsTheCoursesMenuAndDropsItWhenCoursesFail(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", certPassword)
	s.createCourse("Alpha", "First.", "")
	s.logOut()

	if _, err := s.db.Exec(`ALTER TABLE programmes RENAME TO programmes_gone`); err != nil {
		t.Fatal(err)
	}
	res, body := s.get("/programmes")
	expectStatus(t, res, body, http.StatusInternalServerError, "")
	expectInOrder(t, header(t, body), ">Courses<", ">Alpha<", ">All courses<")

	restore := breakCourses(t, s)
	defer restore()
	res, body = s.get("/courses")
	expectStatus(t, res, body, http.StatusInternalServerError, "")
	expectInOrder(t, header(t, body), ">Courses<", ">All courses<")
}

func TestMyCertificatesIsInTheCertificationMenuOnlyWhenLoggedIn(t *testing.T) {
	s := newSite(t)
	_, body := s.get("/")
	head := header(t, body)
	expectInOrder(t, head, ">Certification<", `href="/certificates"`)
	expectNone(t, head, `href="/my-certificates"`)

	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	_, body = s.get("/")
	expectInOrder(t, header(t, body), ">Certification<", `href="/certificates"`, `href="/my-certificates"`)
}

func TestTheHomeMenuReachesTheHomePageSectionsAndTheLabMenuSaysWhatTheLabDoes(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/lab", "en")
	head := header(t, body)
	expectInOrder(t, head, `href="/#welcome"`, `href="/about"`, `href="/#coach"`, `href="/#about"`, `href="/gallery"`)
	expectAll(t, head, ">Company background<")
	expectInOrder(t, head, ">Lab<", "From static dashboard to hosted dashboard", `href="/lab"`)
	expectAll(t, content(body), "<h1>From static dashboard to hosted dashboard</h1>")
	_, body = s.getAsking("/lab", "ms")
	expectAll(t, body, "<h1>Daripada dashboard statik kepada dashboard yang dihoskan</h1>")

	_, body = s.getAsking("/", "en")
	expectAll(t, body, `id="welcome"`, `id="coach"`, `id="about"`)
	_, body = s.getAsking("/", "ms")
	expectAll(t, header(t, body), ">Latar belakang syarikat<")
}
