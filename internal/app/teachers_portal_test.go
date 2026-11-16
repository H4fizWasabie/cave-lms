package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const teachersPassword = "correct horse battery"

// saveTeacherProfile posts the Teacher profile form as the logged-in Account holder.
func (s *site) saveTeacherProfile(subjects string, years ...string) (*http.Response, string) {
	return s.post("/teachers/profile", url.Values{"subjects": {subjects}, "years": years})
}

// newTeacher makes a site with one Account holder, Ada, logged in and with a Teacher profile.
func newTeacher(t *testing.T) *site {
	t.Helper()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	res, body := s.saveTeacherProfile("Sains, Matematik", "Y4", "Y5")
	expectRedirect(t, res, body, "/teachers")
	return s
}

func TestAVisitorSeesThePublicTeachersPortalPageAndRegistersFromIt(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/teachers")
	expectStatus(t, res, body, http.StatusOK, "Teachers Portal")
	expectAll(t, body, `href="/register?next=%2Fteachers"`, `href="/login?next=%2Fteachers"`)

	res, body = s.get("/register?next=%2Fteachers")
	expectStatus(t, res, body, http.StatusOK, `name="next" value="/teachers"`)
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=%2fteachers"`)
	expectAll(t, body, "Create a free Teachers Portal account")
	_, body = s.getAsking("/login?next=%2Fteachers", "ms")
	expectAll(t, body, "Log masuk ke akaun Teachers Portal anda.")
	expectNone(t, body, "Pelajar")

	res, body = s.post("/register", url.Values{"name": {"Ada"}, "username": {"ada"}, "email": {"ada@example.com"}, "password": {teachersPassword}, "next": {"/teachers"}})
	expectStatus(t, res, body, http.StatusOK, "Check your email")
	link := regexp.MustCompile(`https://cave\.example\.invalid(/verify\?\S+)`).FindStringSubmatch(s.lastEmailTo("ada@example.com").Body)
	if link == nil || !strings.Contains(link[1], "next=%2Fteachers") {
		t.Fatalf("verification link %v does not carry the portal as next", link)
	}
	res, body = s.get(link[1])
	expectStatus(t, res, body, http.StatusOK, `name="next" value="/teachers"`)
	res, body = s.post("/verify", url.Values{"token": {verifyToken.FindStringSubmatch(body)[1]}, "next": {"/teachers"}})
	expectStatus(t, res, body, http.StatusOK, `href="/login?next=%2fteachers"`)

	res, body = s.post("/login", url.Values{"email": {"ada@example.com"}, "password": {teachersPassword}, "next": {"/teachers"}})
	expectRedirect(t, res, body, "/teachers")
}

// Only the portal's own paths ride through registration as next; anything else is dropped, as it always was.
func TestRegistrationCarriesOnlyThePortalAsNext(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	for _, next := range []string{"/courses", "/teachersx", "//evil.example", "/admin"} {
		_, body := s.get("/register?next=" + url.QueryEscape(next))
		expectNone(t, body, `<input type="hidden" name="next" value="`+next+`">`)
	}
	_, body := s.get("/register?next=%2Fteachers%2Fprofile")
	expectAll(t, body, `name="next" value="/teachers/profile"`)
}

// ageTrial moves every Teacher trial's start days into the past.
func (s *site) ageTrial(days int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE teacher_profiles SET trial_started_at = now() - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

func TestTheFirstVisitAsksForSubjectsAndYearsAndSavingStartsTheTrialOnce(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)

	res, body := s.get("/teachers")
	expectRedirect(t, res, body, "/teachers/profile")
	res, body = s.get("/teachers/profile")
	expectStatus(t, res, body, http.StatusOK, "Saving starts your free 14-day trial")
	expectAll(t, body, `name="subjects"`, `name="years" value="Y1"`, `name="years" value="F5"`)
	expectNone(t, body, "School", "school")

	// Nothing is saved, and no trial started, until both are given.
	res, body = s.saveTeacherProfile(" , ,", "Y4")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter at least one subject")
	res, body = s.saveTeacherProfile("Sains")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tick at least one year")
	res, body = s.saveTeacherProfile("Sains", "Y9")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tick at least one year")
	res, body = s.saveTeacherProfile(strings.Repeat("x", 61), "Y4")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Enter at least one subject")
	res, body = s.get("/teachers")
	expectRedirect(t, res, body, "/teachers/profile")

	res, body = s.saveTeacherProfile("Sains,  Matematik\nsains", "Y5", "Y4", "F1")
	expectRedirect(t, res, body, "/teachers")
	res, body = s.get("/teachers")
	expectStatus(t, res, body, http.StatusOK, "14 days left in your free trial.")
	expectAll(t, body, "Sains, Matematik", "Year 4, Year 5, Form 1")

	// The form comes back as saved.
	_, body = s.get("/teachers/profile")
	expectAll(t, body, `value="Sains, Matematik"`, `value="Y4" checked`, `value="F1" checked`, "Change your subjects and years any time.")
	expectNone(t, body, `value="Y1" checked`)

	// Changing it later does not start another trial.
	s.ageTrial(10)
	res, body = s.saveTeacherProfile("Bahasa Melayu", "Y6")
	expectRedirect(t, res, body, "/teachers")
	_, body = s.get("/teachers")
	expectAll(t, body, "4 days left in your free trial.", "Bahasa Melayu", "Year 6")
	expectNone(t, body, "<dd>Sains</dd>")

	s.ageTrial(13)
	_, body = s.get("/teachers")
	expectAll(t, body, "1 day left in your free trial.")
	s.ageTrial(14)
	_, body = s.get("/teachers")
	expectAll(t, body, "Your free trial has ended.")
	expectNone(t, body, "left in your free trial")
	// A lapsed Teacher is left with the pay page: the profile is kept as it is, and saving does not bring the trial back.
	res, body = s.saveTeacherProfile("Sains", "Y1")
	expectRedirect(t, res, body, "/teachers")
	_, body = s.get("/teachers")
	expectAll(t, body, "Your free trial has ended.")
	s.ageTrial(0) // the profile came through unchanged
	_, body = s.get("/teachers")
	expectAll(t, body, "<dd>Bahasa Melayu</dd>", "Year 6")
	expectNone(t, body, "<dd>Sains</dd>")
}

func TestOnlyAnAccountHolderWhoIsLoggedInReachesTheProfileForm(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/teachers/profile")
	expectRedirect(t, res, body, "/login?next=%2Fteachers%2Fprofile")
	res, body = s.saveTeacherProfile("Sains", "Y4")
	expectRedirect(t, res, body, "/login")
}

func TestThePortalHasItsOwnNavigationAndWordsInMalayAndEnglish(t *testing.T) {
	t.Parallel()
	teacher := newTeacher(t)
	visitor := teacher.second()
	for lang, want := range map[string][]string{
		"en": {"RPH prompt", "Teacher profile", "Teachers Portal account: Ada"},
		"ms": {"Prompt RPH", "Profil guru", "Akaun Teachers Portal: Ada"},
	} {
		for _, path := range []string{"/teachers", "/teachers/profile"} {
			res, body := teacher.getAsking(path, lang)
			expectStatus(t, res, body, http.StatusOK, `<html lang="`+lang+`">`)
			expectAll(t, body, want...)
			expectInOrder(t, header(t, body), `href="/teachers"`, `href="/teachers/profile"`)
			if n := strings.Count(body, `href="/"`); n != 1 {
				t.Errorf("%s in %s has %d links to CAVELMS, want 1", path, lang, n)
			}
			// The learning side's menus and words stay out of it.
			expectNone(t, body, `href="/courses`, `href="/my-courses"`, `href="/lab"`, `href="/gallery"`, `href="/community"`, "Pelajar", "Learner", "My courses")
		}
		res, body := visitor.getAsking("/teachers", lang)
		expectStatus(t, res, body, http.StatusOK, `<html lang="`+lang+`">`)
		expectNone(t, body, `href="/courses`, "Pelajar", "Learner")
		if n := strings.Count(body, `href="/"`); n != 1 {
			t.Errorf("the public page in %s has %d links to CAVELMS, want 1", lang, n)
		}
	}
}

func TestTheLoginPageFromThePortalKeepsItsNextForRegistering(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.get("/login?next=%2Fteachers")
	expectAll(t, body, `href="/register?next=%2fteachers"`)
	_, body = s.get("/login?next=%2Fcourses")
	expectNone(t, body, `href="/register?next=`)
}

// loginLands logs the account in again, with no next, and says where it went.
func loginLands(t *testing.T, s *site) string {
	t.Helper()
	s.post("/logout", nil)
	res, body := s.login("ada@example.com", teachersPassword)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return res.Header.Get("Location")
}

func TestATeacherWithNothingElseLandsOnThePortalAfterLoginAndEveryoneElseAsToday(t *testing.T) {
	t.Parallel()
	admin, planID := adminWithPlanAndLearner(t)
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	courseID := admin.courseIDByTitle("Go Basics")
	ada := admin.second()
	ada.login("ada@example.com", teachersPassword)

	if got := loginLands(t, ada); got != "/" {
		t.Errorf("an account with no Teacher profile landed on %q, want /", got)
	}
	ada.saveTeacherProfile("Sains", "Y4")
	if got := loginLands(t, ada); got != "/teachers" {
		t.Errorf("a Teacher with nothing else landed on %q, want /teachers", got)
	}
	// A next the login came with still wins.
	ada.post("/logout", nil)
	res, body := ada.post("/login", url.Values{"email": {"ada@example.com"}, "password": {teachersPassword}, "next": {"/pricing"}})
	expectRedirect(t, res, body, "/pricing")

	// CaveText Pro and a Plan count while they run, and not once they have ended.
	if _, err := admin.db.Exec(`INSERT INTO cavetext_accounts (learner_id) SELECT id FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.db.Exec(`INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at) SELECT id, now() - interval '2 days', now() - interval '1 day' FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if got := loginLands(t, ada); got != "/teachers" {
		t.Errorf("a Teacher whose CaveText Pro ended landed on %q, want /teachers", got)
	}
	if _, err := admin.db.Exec(`UPDATE cavetext_pro_periods SET ends_at = now() + interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	if got := loginLands(t, ada); got != "/" {
		t.Errorf("a Teacher with CaveText Pro landed on %q, want /", got)
	}
	if _, err := admin.db.Exec(`UPDATE cavetext_pro_periods SET ended_early_at = now()`); err != nil {
		t.Fatal(err)
	}
	if got := loginLands(t, ada); got != "/teachers" {
		t.Errorf("a Teacher whose CaveText Pro was ended landed on %q, want /teachers", got)
	}

	admin.assignPlan("ada@example.com", planID)
	if got := loginLands(t, ada); got != "/" {
		t.Errorf("a Teacher with a Lab Plan landed on %q, want /", got)
	}
	if _, err := admin.db.Exec(`UPDATE plan_periods SET ended_early_at = now()`); err != nil {
		t.Fatal(err)
	}
	if got := loginLands(t, ada); got != "/teachers" {
		t.Errorf("a Teacher whose Plan period ended landed on %q, want /teachers", got)
	}

	ada.post("/courses/"+courseID+"/join", nil)
	if got := loginLands(t, ada); got != "/" {
		t.Errorf("a Teacher enrolled in a Course landed on %q, want /", got)
	}
}

func TestOnlyLoggedInAccountsSeeOneTeachersPortalLinkInTheHeader(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.get("/")
	expectNone(t, header(t, body), `href="/teachers"`)

	s.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	for _, path := range []string{"/", "/courses", "/my-courses"} {
		_, body = s.get(path)
		head := header(t, body)
		expectAll(t, head, `href="/teachers">Teachers Portal<`)
		// It sits with the account's links, not among the site's menus.
		nav := regexp.MustCompile(`(?s)<nav class="site-nav hdr-wide".*?</nav>`).FindString(head)
		if nav == "" || strings.Contains(nav, `href="/teachers"`) {
			t.Errorf("%s has the Teachers Portal link in the site menus or no menus: %s", path, nav)
		}
	}
}

func TestTheAdminPassesThePortalWithoutAProfileOrTrial(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", teachersPassword)
	res, body := s.get("/teachers")
	expectStatus(t, res, body, http.StatusOK, "the portal is open to you without a trial")
	expectNone(t, body, "left in your free trial", "has ended")
	res, body = s.get("/teachers/profile")
	expectStatus(t, res, body, http.StatusOK, "Subjects")
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM teacher_profiles`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("looking at the portal made %d profiles (%v), want 0", n, err)
	}
}
