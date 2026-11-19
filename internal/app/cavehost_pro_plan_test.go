package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// expectCaveHostMonth checks the Learner has n CaveHost Plan periods, the last starting at starts and lasting the Lab
// Month's 30 days.
func expectCaveHostMonth(t *testing.T, s *site, email string, n int, starts time.Time) []shownPeriod {
	t.Helper()
	periods := s.caveHostPeriodsOf(email)
	if len(periods) != n {
		t.Fatalf("CaveHost Plan periods = %+v, want %d", periods, n)
	}
	last := periods[n-1]
	near(t, "CaveHost Plan period start", last.starts, starts)
	near(t, "CaveHost Plan period end", last.ends, starts.AddDate(0, 0, 30))
	return periods
}

func TestAProPlanPaymentGivesACaveHostPlanPeriodAsLongAsItsLabPlanPeriod(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	expectCaveHostMonth(t, s, "ada@example.com", 1, time.Now())
	res, body := ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusOK, "")
}

func TestAProPlanPaymentWithAPromoCodeGivesACaveHostPlanPeriod(t *testing.T) {
	t.Parallel()
	s := promoSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	expectCaveHostMonth(t, s, "ada@example.com", 1, time.Now())
}

func TestTheAdminsGiveProPlanAndGiveComboGiveACaveHostPlanPeriod(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	res, body := s.post(s.proAction("ada@example.com", "pro-plan"), confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	periods := expectCaveHostMonth(t, s, "ada@example.com", 1, time.Now())

	// The COMBO Pro Plan is a Pro Plan too, and starts when the running CaveHost period ends.
	res, body = s.post(s.proAction("ada@example.com", "combo"), confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	expectCaveHostMonth(t, s, "ada@example.com", 2, periods[0].ends)
}

func TestANewCaveHostPeriodStartsAfterOneStillRunning(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	res, body := s.assignCaveHost("ada@example.com", 10)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	running := s.caveHostPeriodsOf("ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	expectCaveHostMonth(t, s, "ada@example.com", 2, running[0].ends)
}

func TestCaveTextProAndLabPlanBoughtApartGiveNoCaveHost(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paid("LINK-1", "ada@example.com"))
	s.bclPay(paidLabPlan("LINK-2", "ada@example.com"))
	s.givePro("ada@example.com")
	if periods := s.caveHostPeriodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("CaveHost Plan periods = %+v, want none", periods)
	}
	res, body := ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusForbidden, "")
}

func TestWithNoLabPlanMarkedAProPlanGivesNoCaveHostPlanPeriod(t *testing.T) {
	t.Parallel()
	s := bclSite(t)
	res, body := s.savePrices(map[string]string{"pro-plan_link": proPlanFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	if periods := s.caveHostPeriodsOf("ada@example.com"); len(periods) != 0 {
		t.Fatalf("CaveHost Plan periods = %+v, want none", periods)
	}
}

func TestThePricingPageAndTheComboCardListCaveHostOnTheProPlan(t *testing.T) {
	t.Parallel()
	s := promoSite(t)
	visitor := s.second()
	for lang, names := range map[string][2]string{"en": {"Pro Plan", "COMBO Pro Plan"}, "ms": {"Pelan Pro", "Pelan Pro COMBO"}} {
		_, body := visitor.getAsking("/pricing", lang)
		cards, _, _ := strings.Cut(content(body), `id="comparison"`)
		expectInOrder(t, cards, names[0], "CaveHost", `<li class="offer offer-combo">`, names[1], "CaveHost")
	}
}

// capSite is a site whose Google serves a Spreadsheet, with Ada holding a Pro Plan and so CaveHost.
func capSite(t *testing.T) (admin, ada *site, google *fakeGoogle) {
	t.Helper()
	google = newFakeGoogle(t, salesSheet(t))
	admin = bclSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	res, body := admin.savePrices(map[string]string{"pro-plan_link": proPlanFormURL})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.createPlan("Lab Month", "30", "5")
	admin.markLabPlan("Lab Month")
	ada, _ = admin.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	admin.bclPay(paidProPlan("LINK-1", "ada@example.com"))
	return admin, ada, google
}

func TestA4thCaveHostDeploymentIsRefusedAndTheAdminHasNoCap(t *testing.T) {
	t.Parallel()
	admin, ada, google := capSite(t)
	for _, name := range []string{"one-ada", "two-ada", "three-ada"} {
		res, body := ada.linkSheet(sheetLink, name)
		expectRedirect(t, res, body, "/cavehost/"+name)
	}
	asked := len(google.requests())
	res, body := ada.linkSheet(sheetLink, "four-ada")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "You can have at most 3 CaveHost Deployments")
	if got := len(google.requests()); got != asked {
		t.Errorf("the refused 4th link fetched the Spreadsheet: %d requests, want %d", got, asked)
	}
	ada.switchLanguage("ms", "/")
	res, body = ada.linkSheet(sheetLink, "four-ada")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Anda boleh mempunyai paling banyak 3 Deployment CaveHost")

	for _, name := range []string{"a-admin", "b-admin", "c-admin", "d-admin"} {
		res, body := admin.linkSheet(sheetLink, name)
		expectRedirect(t, res, body, "/cavehost/"+name)
	}
}
