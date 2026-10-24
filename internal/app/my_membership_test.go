package app_test

import (
	"net/http"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

func TestMyMembershipIsInThePricingMenuAndSendsAVisitorToLogIn(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Pricing<", `href="/cavetext"`, `<a href="/my-membership">My membership</a>`, ">Program<")
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, header(t, body), ">Harga<", `<a href="/my-membership">Keahlian saya</a>`)

	res, body := s.get("/my-membership")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/login?next=%2Fmy-membership" {
		t.Fatalf("Visitor My membership redirected to %q, want /login?next=%%2Fmy-membership", loc)
	}
}

func TestMyMembershipWithNoPlanSaysSoAndLinksToThePlans(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")

	_, body := s.getAsking("/my-membership", "en")
	main := content(body)
	expectInOrder(t, main, "My membership", "Plan", "No Plan", `href="/pricing"`, "CaveText Pro", "Free", "Snippet cap")
	_, body = s.getAsking("/my-membership", "ms")
	expectInOrder(t, content(body), "Keahlian saya", "Pelan", "Tiada Pelan", `href="/pricing"`, "CaveText Pro", "Percuma", "Had Snippet")
}

func TestMyMembershipShowsTheActivePlanUntilTheLastQueuedPeriodEnds(t *testing.T) {
	s, id := adminWithPlanAndLearner(t)
	ada := s.second()
	ada.login("ada@example.com", "correct horse battery")

	s.assignPlan("ada@example.com", id)
	ends := s.periodsOf("ada@example.com")[0].ends
	_, body := ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "Lab Monthly", "Until "+text.Date("en", ends))
	expectNone(t, content(body), "No Plan")

	s.assignPlan("ada@example.com", id)
	last := s.periodsOf("ada@example.com")[1].ends
	_, body = ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "Lab Monthly", "Until "+text.Date("en", last))
	_, body = ada.getAsking("/my-membership", "ms")
	expectInOrder(t, content(body), "Lab Monthly", "Hingga "+text.Date("ms", last))
}

func TestMyMembershipShowsCaveTextProAndTheSnippetCapButNoPlanFromCaveTextProAlone(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada", "ada@example.com")

	_, body := ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "CaveText Pro", "Free", "Snippet cap", "10 Snippets")

	s.givePro("ada@example.com")
	until := expectProForAYear(t, ada, token)
	_, body = ada.getAsking("/my-membership", "en")
	expectInOrder(t, content(body), "No Plan", `href="/pricing"`, "CaveText Pro", "Until "+text.Date("en", until.UTC()), "Snippet cap", "Unlimited")
	_, body = ada.getAsking("/my-membership", "ms")
	expectInOrder(t, content(body), "CaveText Pro", "Hingga "+text.Date("ms", until.UTC()), "Had Snippet", "Tanpa had")
}
