package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

func (s *site) setIC(ic string) (*http.Response, string) {
	return s.post("/my-profile/ic", url.Values{"ic": {ic}})
}

func TestALearnerSetsAMyKadICOnTheirProfileAndSeesItFormatted(t *testing.T) {
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
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	s.setIC("a12 345-678")
	_, body := s.get("/my-profile")
	expectInOrder(t, content(body), "A12345678")
}

func TestMyProfileIsInTheClassroomMenuAndSendsAVisitorToLogIn(t *testing.T) {
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
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	for _, ic := range []string{" - ", "95/1206", "123456789012345678901"} {
		res, body := s.setIC(ic)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "letters and digits")
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
	_, ada, _, correct := adminWithLearnerInCourse(t)
	res, body := ada.post(correct, url.Values{"ic": {"A1234567"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = ada.get("/my-profile")
	expectInOrder(t, content(body), "951206-08-5555")
}
