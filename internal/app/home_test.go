package app_test

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func TestTheHomePagesTrustStripShowsPlaceholdersForMissingNumbersOnlyOutsideProduction(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, content(body), "TCC", "A platform by The Cave Consult", "HRD Corp", "MOF", "JPK", "Placeholder", "learners trained")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, content(body), "TCC", "Platform oleh The Cave Consult", "HRD Corp", "MOF", "JPK", "Pemegang tempat", "peserta dilatih")

	live := newSite(t, func(c *config.Config) { c.Production = true })
	_, body = live.getAsking("/", "en")
	expectInOrder(t, content(body), "HRD Corp", "MOF", "JPK")
	expectNone(t, content(body), "Placeholder", "learners trained", "agencies and companies")
}

func TestEachHomeSectionHasAtMostOnePrimaryButton(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.courseWithContent()
	s.logOut()
	for _, lang := range []string{"en", "ms"} {
		_, body := s.getAsking("/", lang)
		sections := strings.Split(content(body), "<section")[1:]
		if len(sections) < 6 {
			t.Fatalf("%s home page has %d sections, want the hero, trust strip, pathway, Courses, Coach and cost comparison at least", lang, len(sections))
		}
		for _, sec := range sections {
			if n := strings.Count(sec, "btn-solid") + strings.Count(sec, "btn-gold"); n > 1 {
				t.Errorf("%s home section has %d primary buttons, want at most 1:\n<section%s", lang, n, sec)
			}
		}
	}
}

func TestTheHomePageShowsThePCCPathwayAndTheLatestCourseCards(t *testing.T) {
	s := newSite(t)
	s.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	s.courseWithContent()
	s.logOut()
	_, body := s.getAsking("/", "en")
	main := content(body)
	expectInOrder(t, main, "pcc-basic.jpg", "Basic", "1 day", "pcc-foundation.jpg", "Foundation", "2 days",
		"pcc-practitioner.jpg", "Practitioner", "3 days", "pcc-professional.jpg", "Professional", "4 or 5 days")
	expectAll(t, courseCard(t, main, "Go Basics"), `href="/courses/`)
	expectHeadingsInOrder(t, "/ en", body)
	_, body = s.getAsking("/", "ms")
	expectHeadingsInOrder(t, "/ ms", body)
	expectInOrder(t, content(body), "Laluan pensijilan PCC", "1 hari", "4 atau 5 hari", "Kursus terkini")
}

func TestTheCostComparisonIsATableInBothLanguages(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, content(body), "What a year costs", "<table", "Yearly cost", "CAVELMS", "RM99", "Supabase Pro", "RM1,300+", "</table>")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, content(body), "Kos setahun", "<table", "Perkhidmatan", "RM99", "</table>")

}

func TestTheHomeHeroCarriesTheClientsTaglineInBothLanguages(t *testing.T) {
	s := newSite(t)
	for _, lang := range []string{"en", "ms"} {
		_, body := s.getAsking("/", lang)
		_, hero, _ := strings.Cut(content(body), `<section class="home-hero"`)
		hero, _, _ = strings.Cut(hero, "</section>")
		expectInOrder(t, hero, "CAVELMS · Learn AI • Build Skills • Create Impact", "<h1")
	}
}

// The client keeps Certificates off the public front page: the hero shows people building a dashboard, and About CAVELMS comes
// straight after the trust strip so visitors read about the company first.
func TestTheHomeHeroShowsADashboardPhotoAndAboutFollowsTheTrustStripInBothLanguages(t *testing.T) {
	s := newSite(t)
	for lang, about := range map[string]string{"en": "CAVELMS is a digital learning platform", "ms": "CAVELMS ialah platform pembelajaran digital"} {
		_, body := s.getAsking("/", lang)
		main := content(body)
		_, hero, _ := strings.Cut(main, `<section class="home-hero"`)
		hero, _, _ = strings.Cut(hero, "</section>")
		expectAll(t, hero, "home-dashboard.webp", `alt="`+text.T(lang, "home.dashboard-alt")+`"`)
		expectNone(t, hero, "cert-preview", "/certificates")
		expectInOrder(t, main, `class="trust"`, `id="about"`, about, "pcc-basic.jpg")
	}
}

func TestTheMalayHomePagesButtonsAreInMalay(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "ms")
	main := content(body)
	expectAll(t, main, "Sertai Kelas Dalam Talian", "Terokai eBook", "Belajar Membina Dashboard AI")
	expectNone(t, main, "Join Online Classes", "Explore eBooks", "Learn AI Dashboard Development")
	_, body = s.getAsking("/", "en")
	expectAll(t, content(body), "Join Online Classes", "Explore eBooks", "Learn AI Dashboard Development")
}

var homeAnchor = regexp.MustCompile(`href="/#([a-z-]+)"`)

// The header's Home menu jumps to Welcome, Coach Adam and About CAVELMS on the home page: each lands on its section.
func TestEveryHomeMenuLinkLandsOnItsSectionInBothLanguages(t *testing.T) {
	s := newSite(t)
	lands := map[string]map[string]string{
		"en": {"welcome": "Welcome to CAVELMS", "coach": "Coach Adam Daniel", "about": "CAVELMS is a digital learning platform"},
		"ms": {"welcome": "Selamat Datang ke CAVELMS", "coach": "Coach Adam Daniel", "about": "CAVELMS ialah platform pembelajaran digital"},
	}
	for lang, want := range lands {
		_, body := s.getAsking("/", lang)
		links := homeAnchor.FindAllStringSubmatch(header(t, body), -1)
		if len(links) < len(want) {
			t.Fatalf("%s header links to %d home sections, want %d", lang, len(links), len(want))
		}
		for _, m := range links {
			id := m[1]
			_, after, ok := strings.Cut(content(body), `id="`+id+`"`)
			if !ok {
				t.Errorf("%s home page has no section with id %q for the header's /#%s", lang, id, id)
				continue
			}
			section, _, _ := strings.Cut(after, "<section")
			if !strings.Contains(section, want[id]) {
				t.Errorf("%s /#%s lands on a section without %q:\n%s", lang, id, want[id], section)
			}
		}
	}
}

// Coach Adam's write-up is the client's marketing copy: the home page carries all of it, word for word, in both languages.
func TestTheHomePageCarriesCoachAdamsWriteUpInBothLanguages(t *testing.T) {
	s := newSite(t)
	keys := []string{"home.learn.title"}
	for _, n := range []string{"1", "2", "3", "4"} {
		keys = append(keys, "home.learn."+n+".title", "home.learn."+n+".p1", "home.learn."+n+".p2")
	}
	keys = append(keys, "home.static.title", "home.static.p1", "home.static.p2", "home.static.p3",
		"home.dynamic.title", "home.dynamic.p1", "home.dynamic.p2", "home.dynamic.p3", "home.dynamic.p4",
		"home.learn.5.title", "home.learn.5.p1", "home.learn.5.p2",
		"home.apply.title", "home.apply.p1", "home.apply.p2")
	for lang, title := range map[string]string{"en": "What Can You Learn at CAVELMS?", "ms": "Apa Yang Boleh Dipelajari di CAVELMS?"} {
		_, body := s.getAsking("/", lang)
		main := content(body)
		expectAll(t, main, title)
		var wants []string
		for _, k := range keys {
			wants = append(wants, html.EscapeString(text.T(lang, k)))
		}
		wants = append(wants, "LEARN → APPLY → BUILD → SUSTAIN", html.EscapeString(text.T(lang, "home.apply.note")))
		expectInOrder(t, main, wants...)
	}
}
