package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func (s *site) setIC(ic string) (*http.Response, string) {
	return s.post("/my-profile/ic", url.Values{"ic": {ic}})
}

func TestALearnerSetsAMyKadICOnTheirProfileAndSeesItFormatted(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	_, body := s.getAsking("/my-profile", "en")
	expectInOrder(t, content(body), "My profile", "IC or passport number", `action="/my-profile/ic"`)
	_, body = s.getAsking("/my-profile", "ms")
	expectInOrder(t, content(body), "Profil saya", "No. KP atau pasport", `action="/my-profile/ic"`)

	res, body := s.setIC(" 951206 - 08 5555 ")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.getAsking("/my-profile", "en")
	expectInOrder(t, content(body), "IC or passport number", "951206-08-5555", "Only the Admin can change it")
	expectNone(t, content(body), `action="/my-profile/ic"`)
	_, body = s.getAsking("/my-profile", "ms")
	expectInOrder(t, content(body), "951206-08-5555", "Hanya Admin boleh mengubahnya")
}

func TestAPassportNumberIsShownAsStoredWithoutSpacesOrDashes(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	s.setIC("a12 345-678")
	_, body := s.get("/my-profile")
	expectInOrder(t, content(body), "A12345678")
}

func TestMyProfileIsInTheClassroomMenuAndSendsAVisitorToLogIn(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Classroom<", `<a href="/my-profile">My profile</a>`)

	res, body := s.get("/my-profile")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login?next=%2Fmy-profile" {
		t.Fatalf("Visitor My profile redirected to %q", loc)
	}
}

func TestALearnerCannotChangeTheirICOnceSet(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.setIC("951206085555")

	res, body := s.setIC("A1234567")
	expectStatus(t, res, body, http.StatusConflict, "already set. Only the Admin can change it")
	_, body = s.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5555")
	expectNone(t, content(body), "A1234567")
}

func TestAnICAlreadyUsedByAnotherLearnerIsRefused(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.setIC("951206-08-5555")
	bea := s.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")

	res, body := bea.setIC("9512060 85555")
	expectStatus(t, res, body, http.StatusConflict, "already used by another Learner")
	res, body = bea.postAsking("/my-profile/ic", "ms", url.Values{"ic": {"951206085555"}})
	expectStatus(t, res, body, http.StatusConflict, "sudah digunakan oleh Pelajar lain")
	_, body = bea.get("/my-profile")
	expectInOrder(t, content(body), `action="/my-profile/ic"`)
}

func TestAnICThatIsNotLettersAndDigitsIsRefused(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	for _, ic := range []string{" - ", "95/1206", "123456789012345678901"} {
		res, body := s.setIC(ic)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "letters and digits")
		expectFieldError(t, body, "ic", "letters and digits", "</button>")
		expectInOrder(t, mainOf(body), `id="ic"`, `value="`+ic+`"`)
	}
}

var correctICLink = regexp.MustCompile(`action="(/admin/courses/\d+/learners/\d+/ic)"`)

// adminWithLearnerInCourse is an Admin with one Course that Ada has joined, after Ada set her IC.
func adminWithLearnerInCourse(t *testing.T) (admin, ada *site, courseID, correctPath string) {
	t.Helper()
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	courseID = admin.onlyCourseID()
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.post("/courses/"+courseID+"/join", nil)
	ada.setIC("951206085555")
	_, body := admin.get("/admin/courses/" + courseID)
	m := correctICLink.FindStringSubmatch(adminCourseRow(t, body, "ada@example.com"))
	if m == nil {
		t.Fatalf("Ada's row has no IC correction form: %s", body)
	}
	return admin, ada, courseID, m[1]
}

func TestTheAdminCorrectsALearnersIC(t *testing.T) {
	t.Parallel()
	admin, ada, id, correct := adminWithLearnerInCourse(t)
	_, body := admin.getAsking("/admin/courses/"+id, "en")
	expectInOrder(t, body, "IC or passport number", "951206-08-5555")
	_, body = admin.getAsking("/admin/courses/"+id, "ms")
	expectInOrder(t, body, "No. KP atau pasport", "951206-08-5555")

	res, body := admin.post(correct, url.Values{"ic": {"951206-08-5556"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5556", "Only the Admin can change it")
	_, body = admin.get("/admin/courses/" + id)
	expectCells(t, adminCourseRow(t, body, "ada@example.com"), "951206-08-5556")

	admin.post(correct, url.Values{"ic": {""}})
	_, body = ada.get("/my-profile")
	expectInOrder(t, content(body), `action="/my-profile/ic"`)
}

func TestTheAdminCannotGiveALearnerAnICAnotherLearnerUses(t *testing.T) {
	t.Parallel()
	admin, ada, _, correct := adminWithLearnerInCourse(t)
	bea := admin.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	bea.setIC("A1234567")

	res, body := admin.post(correct, url.Values{"ic": {"a123 4567"}})
	expectStatus(t, res, body, http.StatusConflict, "already used by another Learner")
	_, body = ada.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5555")
}

func TestLearnersAreRefusedTheAdminsICCorrection(t *testing.T) {
	t.Parallel()
	_, ada, _, correct := adminWithLearnerInCourse(t)
	res, body := ada.post(correct, url.Values{"ic": {"A1234567"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = ada.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5555")
}

func TestMyProfileIsOneCardWithPictureUsernameAndICSectionsEachSavingWithAGhostButton(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	for _, lang := range []string{"en", "ms"} {
		_, body := s.getAsking("/my-profile", lang)
		m := mainOf(body)
		if n := strings.Count(m, `class="card`); n != 1 {
			t.Fatalf("%s: My profile has %d cards, want 1; body:\n%s", lang, n, m)
		}
		if n := strings.Count(m, `class="profile-section"`); n != 3 {
			t.Fatalf("%s: My profile has %d sections, want 3; body:\n%s", lang, n, m)
		}
		if strings.Contains(m, "btn-solid") {
			t.Fatalf("%s: My profile has a primary button, want none", lang)
		}
		expectInOrder(t, m, `action="/my-profile/picture"`, `action="/my-profile/username"`, `action="/my-profile/ic"`)
		for _, id := range []string{"picture", "username", "ic"} {
			expectAll(t, m, `<label class="sr" for="`+id+`">`)
		}
	}
}

// profileSection is the My profile section whose heading has this id (picture-h, username-h or ic-h), up to its end.
func profileSection(t *testing.T, body, heading string) string {
	t.Helper()
	_, rest, ok := strings.Cut(mainOf(body), `aria-labelledby="`+heading+`"`)
	if !ok {
		t.Fatalf("no section %s: %s", heading, body)
	}
	section, _, _ := strings.Cut(rest, "</section>")
	return section
}

func TestMyProfileShowsTheSavedMessageAndEachErrorInTheSectionItBelongsTo(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	_, body := s.get("/my-profile?saved")
	expectAll(t, profileSection(t, body, "username-h"), `<p class="field-ok" role="status">Saved.</p>`)
	for _, other := range []string{"picture-h", "ic-h"} {
		expectNone(t, profileSection(t, body, other), "Saved.")
	}

	_, body = s.setIC("-")
	expectAll(t, profileSection(t, body, "ic-h"), "letters and digits", `id="ic-err"`)
	for _, other := range []string{"picture-h", "username-h"} {
		expectNone(t, profileSection(t, body, other), "letters and digits")
	}

	res, body := s.post("/my-profile/username", url.Values{"username": {"x"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, profileSection(t, body, "username-h"), "must be 3 to 40 characters", `id="username-err"`)
	for _, other := range []string{"picture-h", "ic-h"} {
		expectNone(t, profileSection(t, body, other), "must be 3 to 40 characters")
	}

	s.setIC("951206085555")
	_, body = s.setIC("951206085555") // already set: refused, and the refusal sits beside the locked IC
	expectAll(t, profileSection(t, body, "ic-h"), `<p class="field-err" role="alert">`, "951206-08-5555")
	expectNone(t, profileSection(t, body, "username-h"), `role="alert"`)
}
