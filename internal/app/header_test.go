package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// header is the part of a page above its content: the site header.
func header(t *testing.T, body string) string {
	t.Helper()
	head, _, ok := strings.Cut(body, `<main id="main">`)
	if !ok {
		t.Fatalf("page has no <main>; body:\n%s", body)
	}
	return head
}

// content is the part of a page below its header.
func content(body string) string {
	_, c, _ := strings.Cut(body, `<main id="main">`)
	return c
}

// mainOf is a page's own content: between its header and its footer.
func mainOf(body string) string {
	c, _, _ := strings.Cut(content(body), "</main>")
	return c
}

// expectFieldError fails unless the form's field with id is marked invalid and want is said once, after that field and
// before next (the field after it, or the form's button).
func expectFieldError(t *testing.T, body, id, want, next string) {
	t.Helper()
	m := mainOf(body)
	if n := strings.Count(m, want); n != 1 {
		t.Fatalf("says %q %d times, want once; body:\n%s", want, n, m)
	}
	if !regexp.MustCompile(`<(input|textarea)[^>]*\bid="` + id + `"[^>]*aria-invalid="true"`).MatchString(m) {
		t.Fatalf("field %q is not marked invalid; body:\n%s", id, m)
	}
	expectInOrder(t, m, `id="`+id+`"`, want, next)
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

func TestTheHomeMenuReachesTheHomeSectionsAndTheAboutPage(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/lab", "en")
	head := header(t, body)
	expectInOrder(t, head, `href="/#welcome"`, `href="/about"`, `href="/#coach"`, `href="/#about"`, `href="/gallery"`)
	expectAll(t, head, ">Company background<")

	_, body = s.getAsking("/", "en")
	expectAll(t, body, `id="welcome"`, `id="coach"`, `id="about"`)
	_, body = s.getAsking("/", "ms")
	expectAll(t, header(t, body), ">Latar belakang syarikat<")
}

// The header holds the nav twice: once for wide screens and once in the narrow-screen Menu.
func TestLabIsAPlainLinkInTheHeaderAndItsHeroLineStaysOnTheLabPage(t *testing.T) {
	s := newSite(t)
	for lang, hero := range map[string]string{
		"en": "From static dashboard to hosted dashboard",
		"ms": "Daripada dashboard statik kepada dashboard yang dihoskan",
	} {
		_, body := s.getAsking("/lab", lang)
		head := header(t, body)
		if n := strings.Count(head, `<a class="hdr-link" href="/lab">Lab</a>`); n != 2 {
			t.Errorf("%s: header has %d plain Lab links, want 2 (wide and Menu); header:\n%s", lang, n, head)
		}
		expectNone(t, head, `<summary class="hdr-link">Lab</summary>`, hero)
		expectAll(t, content(body), "<h1>"+hero+"</h1>")
	}
}

// A section with one link is that link; Certification has a second, My certificates, once logged in.
func TestHeaderSectionsWithOneLinkArePlainLinks(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	head := header(t, body)
	for _, link := range []string{
		`<a class="hdr-link" href="/programmes">Program</a>`,
		`<a class="hdr-link" href="/community">Community</a>`,
		`<a class="hdr-link" href="/certificates">Certification</a>`,
	} {
		if n := strings.Count(head, link); n != 2 {
			t.Errorf("header has %d of %s, want 2 (wide and Menu); header:\n%s", n, link, head)
		}
	}
	expectNone(t, head, `<summary class="hdr-link">Program<`, `<summary class="hdr-link">Community<`, `<summary class="hdr-link">Certification<`)

	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	_, body = s.getAsking("/", "en")
	head = header(t, body)
	expectInOrder(t, head, `<summary class="hdr-link">Certification<`, `href="/certificates"`, `href="/my-certificates"`)
	expectNone(t, head, `<a class="hdr-link" href="/certificates">Certification</a>`)
}

func TestTheLearnerFooterReachesTheirPagesAndLogsOut(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", certPassword)
	_, body := s.getAsking("/courses", "en")
	_, foot, _ := strings.Cut(content(body), "</main>")
	expectInOrder(t, foot, `href="/my-courses">My courses<`, `href="/my-exams">My exams<`, `href="/my-certificates">My certificates<`,
		`href="/my-profile">My profile<`, `<form method="post" action="/logout"><button>Log out</button></form>`)
}

func TestEveryPageOpensWithALinkThatSkipsTheHeaderToTheContent(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, body, "<body>", `<a class="skip" href="#main">Skip to content</a>`, `<header class="site-header">`, `<main id="main">`)
	_, body = s.getAsking("/courses", "ms")
	expectInOrder(t, body, `<a class="skip" href="#main">Langkau ke kandungan</a>`, `<main id="main">`)
}
