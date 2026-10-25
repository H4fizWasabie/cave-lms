package app_test

import (
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

func TestTheHomePagesTrustStripShowsPlaceholdersForMissingNumbersOnlyOutsideProduction(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, content(body), "HRD Corp", "MOF", "JPK", "Placeholder", "learners trained")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, content(body), "HRD Corp", "MOF", "JPK", "Pemegang tempat", "peserta dilatih")

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

	for lang, want := range map[string][]string{
		"en": {`id="comparison"`, "<table", "Text expansion", "Personal subdomain", "Depends on plan", "Total yearly cost", "RM1,300+", "</table>"},
		"ms": {`id="comparison"`, "<table", "Pengembangan teks", "Subdomain peribadi", "Bergantung pada pelan", "Jumlah kos setahun", "RM1,300+", "</table>"},
	} {
		_, body = s.getAsking("/pricing", lang)
		expectInOrder(t, content(body), want...)
		expectNone(t, content(body), "cost-comparison.webp")
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
