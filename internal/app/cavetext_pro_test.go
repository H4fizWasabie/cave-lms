package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// caveTextAdminSite is a CaveText site logged in as an Admin who is not the Owner.
func caveTextAdminSite(t *testing.T) *site {
	t.Helper()
	s := newSite(t, func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.CaveTextExtensionIDs = []string{extensionID}
	})
	s.loginAsNewAdmin("Mini", "mini@example.com", "correct horse battery")
	return s
}

// proAction finds the give or end Pro button for the Learner with this email on the Admin CaveText Learners page.
func (s *site) proAction(email, action string) string {
	s.t.Helper()
	res, body := s.get("/admin/cavetext")
	expectStatus(s.t, res, body, http.StatusOK, "CaveText Learners")
	m := regexp.MustCompile(regexp.QuoteMeta(email) + `[\s\S]*?(?:action|href)="(/admin/cavetext/learners/\d+/` + action + `)"`).FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("no %s button for %s: %s", action, email, body)
	}
	return m[1]
}

func (s *site) givePro(email string) {
	s.t.Helper()
	res, body := s.post(s.proAction(email, "pro"), nil)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

func (s *site) endPro(email string) {
	s.t.Helper()
	res, body := s.post(s.proAction(email, "end-pro"), confirming)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// agePro moves every CaveText Pro period days into the past.
func (s *site) agePro(days int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE cavetext_pro_periods SET starts_at = starts_at - make_interval(days => $1),
		ends_at = ends_at - make_interval(days => $1), ended_early_at = ended_early_at - make_interval(days => $1)`, days); err != nil {
		s.t.Fatal(err)
	}
}

func near(t *testing.T, what string, got, want time.Time) {
	t.Helper()
	if d := got.Sub(want); d < -2*time.Minute || d > 2*time.Minute {
		t.Fatalf("%s = %v, want about %v", what, got, want)
	}
}

func TestProGivenByTheAdminLastsTwelveMonthsWithUpToTwoThousandSnippets(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 5)
	if me := ada.me(token); me.Pro || me.ProUntil != nil {
		t.Fatalf("me = %+v, want free", me)
	}

	s.givePro("ada@example.com")
	me := ada.me(token)
	if !me.Pro || me.ProUntil == nil {
		t.Fatalf("me = %+v, want Pro with an expiry", me)
	}
	near(t, "Pro until", *me.ProUntil, time.Now().AddDate(1, 0, 0))
	res, body := ada.caveText(http.MethodGet, "me", token)
	expectStatus(t, res, body, http.StatusOK, `"cap":2000`)

	for _, c := range "abcdefg" {
		ada.createSnippet(token, snippet{Folder: "Work", Name: "Extra " + string(c), Shortcut: "/extra-" + string(c), Body: "x"})
	}
	if me := ada.me(token); me.SnippetCount != 12 {
		t.Fatalf("me = %+v, want 12 Snippets past the free 5", me)
	}
	var set []snippet
	for i := range 20 {
		set = append(set, snippet{Folder: "Kept", Name: fmt.Sprintf("I%d", i), Shortcut: fmt.Sprintf("/i%02d", i), Body: "x"})
	}
	res, body = ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": set})
	expectStatus(t, res, body, http.StatusOK, `"skipped":0`)
	if n := len(ada.snippets(token)); n != 20 {
		t.Fatalf("Snippets = %d, want all 20 imported", n)
	}
}

func TestRenewingProEarlyStartsTheNewYearWhenTheRunningOneEnds(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	s.agePro(100)
	first := *ada.me(token).ProUntil
	near(t, "first Pro until", first, time.Now().AddDate(1, 0, -100))

	s.givePro("ada@example.com")
	me := ada.me(token)
	if !me.Pro || !me.ProUntil.Equal(first.AddDate(1, 0, 0)) {
		t.Fatalf("after renewing, me = %+v, want Pro until %v", me, first.AddDate(1, 0, 0))
	}
	_, body := s.get("/admin/cavetext")
	if !strings.Contains(body, first.AddDate(1, 0, 0).Format("2006-01-02 15:04 UTC")) {
		t.Fatalf("the Learners page doesn't show the renewed expiry: %s", body)
	}

	// Once Pro has lapsed, Pro given again starts now.
	s.agePro(1000)
	if ada.me(token).Pro {
		t.Fatalf("Pro still active after both years")
	}
	s.givePro("ada@example.com")
	near(t, "Pro until after a lapse", *ada.me(token).ProUntil, time.Now().AddDate(1, 0, 0))
}

func TestTwoRenewalsWhileProRunsChainOneYearAfterAnother(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	s.agePro(100)
	first := *ada.me(token).ProUntil

	s.givePro("ada@example.com")
	s.givePro("ada@example.com")
	if me := ada.me(token); !me.Pro || !me.ProUntil.Equal(first.AddDate(2, 0, 0)) {
		t.Fatalf("after two renewals, me = %+v, want Pro until %v", me, first.AddDate(2, 0, 0))
	}
	// Past the second year, the third is running.
	s.agePro(365 + 366)
	if me := ada.me(token); !me.Pro || !me.ProUntil.Equal(first.AddDate(2, 0, 0).AddDate(0, 0, -365-366)) {
		t.Fatalf("in the third year, me = %+v, want Pro until the third period ends", me)
	}
}

func TestEndingProEarlyTakesEffectAtOnce(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	s.givePro("ada@example.com")
	ada.fillSnippets(token, 6)

	s.endPro("ada@example.com")
	me := ada.me(token)
	if me.Pro || me.ProUntil != nil || me.Cap != 5 || me.SnippetCount != 6 {
		t.Fatalf("after ending Pro, me = %+v, want free with 6 of 5", me)
	}
	res, body := ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "More", Shortcut: "/more", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, "You have 6 of your 5 free Snippets.")
	_, body = s.get("/admin/cavetext")
	if strings.Contains(body, "/end-pro") {
		t.Fatalf("a Learner without Pro still has an End Pro button: %s", body)
	}
	// The renewal queued behind the ended period never starts.
	s.agePro(400)
	if me := ada.me(token); me.Pro || me.ProUntil != nil {
		t.Fatalf("in the queued renewal's year, me = %+v, want free", me)
	}

	// Pro given after ending early starts now.
	s.givePro("ada@example.com")
	near(t, "Pro until", *ada.me(token).ProUntil, time.Now().AddDate(1, 0, 0))
}

func TestALapsedLearnerReturnsToTheFreeCapTheirPlaceGivesAndKeepsEverySnippet(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1000)
	early, earlyToken := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	late, lateToken := s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.givePro("ada@example.com")
	s.givePro("bea@example.com")
	for i := range 12 {
		early.createSnippet(earlyToken, snippet{Folder: "Work", Name: fmt.Sprintf("S%02d", i), Shortcut: fmt.Sprintf("/s%02d", i), Body: "x"})
	}
	late.fillSnippets(lateToken, 7)

	s.agePro(400)
	for _, c := range []struct {
		who   *site
		token string
		want  string
		cap   int
		have  int
	}{{early, earlyToken, "You have 12 of your 10 free Snippets.", 10, 12}, {late, lateToken, "You have 7 of your 5 free Snippets.", 5, 7}} {
		me := c.who.me(c.token)
		if me.Pro || me.Cap != c.cap || me.SnippetCount != c.have {
			t.Fatalf("lapsed: me = %+v, want free with %d of %d", me, c.have, c.cap)
		}
		res, body := c.who.caveTextSend(http.MethodPost, "snippets", c.token, snippet{Folder: "Work", Name: "More", Shortcut: "/more", Body: "x"})
		expectStatus(t, res, body, http.StatusForbidden, c.want)
		list := c.who.snippets(c.token)
		res, body = c.who.caveTextSend(http.MethodPut, fmt.Sprintf("snippets/%d", list[0].ID), c.token, snippet{Folder: "Work", Name: list[0].Name, Shortcut: list[0].Shortcut, Body: "still mine"})
		expectStatus(t, res, body, http.StatusOK, "still mine")
	}
	// Under the cap again, the early Learner may add.
	list := early.snippets(earlyToken)
	for _, sn := range list[:3] {
		res, body := early.caveText(http.MethodDelete, fmt.Sprintf("snippets/%d", sn.ID), earlyToken)
		expectStatus(t, res, body, http.StatusNoContent, "")
	}
	early.createSnippet(earlyToken, snippet{Folder: "Work", Name: "More", Shortcut: "/more", Body: "x"})
}

func TestTheCaveTextLearnersPageShowsPlacesProAndTheEarlyPlacesTaken(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(42)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 3)
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.second().loginAsNewLearner("Cy Tan", "cy@example.com", "correct horse battery")
	s.givePro("bea@example.com")

	for _, path := range []string{"/admin", "/admin/plans"} {
		_, body := s.get(path)
		if !strings.Contains(body, `href="/admin/cavetext"`) {
			t.Errorf("the Admin sidebar on %s has no CaveText Learners link", path)
		}
	}
	res, body := s.get("/admin/cavetext")
	expectStatus(t, res, body, http.StatusOK, "<strong>43 of 1,000</strong> early places taken")
	row := regexp.MustCompile(`<tr>\s*<td>(\d+)[^<]*</td>\s*<td>([^<]+)<br><span class="muted">([^<]+)</span></td>\s*<td>([^<]+)</td>\s*<td>([^<]+)</td>\s*<td>(\d+)</td>`)
	var got []string
	for _, m := range row.FindAllStringSubmatch(body, -1) {
		got = append(got, fmt.Sprintf("%s %s %s %s %s", m[1], m[3], m[4], m[6], strings.Fields(m[5])[0]))
	}
	year := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	if want := []string{"42 ada@example.com Free 3 -", "43 bea@example.com Pro 0 " + year}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if strings.Contains(body, "cy@example.com") {
		t.Fatalf("a Learner who never signed in to CaveText is listed")
	}

	// Lowering the early places caps the counter.
	if _, err := s.db.Exec(`UPDATE cavetext_settings SET early_places = 10`); err != nil {
		t.Fatal(err)
	}
	res, body = s.get("/admin/cavetext")
	expectStatus(t, res, body, http.StatusOK, "<strong>10 of 10</strong> early places taken")
}

func TestOnlyAnAdminReachesTheCaveTextLearnersAndGivesPro(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	give, end := s.proAction("ada@example.com", "pro"), strings.Replace(s.proAction("ada@example.com", "pro"), "/pro", "/end-pro", 1)
	visitor := s.second()
	for _, path := range []string{"/admin/cavetext", "/admin/cavetext/settings"} {
		res, body := visitor.get(path)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	for _, path := range []string{give, end, "/admin/cavetext/settings", "/admin/cavetext/master-prompts"} {
		res, body := visitor.post(path, url.Values{"price": {"0"}})
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.post(path, url.Values{"price": {"0"}})
		expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	}
	if ada.me(token).Pro {
		t.Fatalf("a Learner gave themselves Pro")
	}
	res, body := s.post("/admin/cavetext/learners/999999/pro", nil)
	expectStatus(t, res, body, http.StatusNotFound, "No CaveText Learner")
	res, body = s.post("/admin/cavetext/learners/x/end-pro", nil)
	expectStatus(t, res, body, http.StatusNotFound, "No CaveText Learner")
}

func TestEndingProAsksOnTheConfirmationPageFirst(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	confirmPage := s.proAction("ada@example.com", "end-pro")

	res, body := s.post(confirmPage, nil)
	expectRedirect(t, res, body, confirmPage)
	if !ada.me(token).Pro {
		t.Fatalf("an unconfirmed End Pro ended it")
	}
	res, body = s.get(confirmPage)
	expectStatus(t, res, body, http.StatusOK, "End Pro for ada@example.com now?")
	expectAll(t, body, "This cannot be undone", `href="/admin/cavetext"`)

	res, body = s.post(confirmPage, confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if ada.me(token).Pro {
		t.Fatalf("a confirmed End Pro left Pro running")
	}
}

func TestGivingProAndTheCaveTextSettingsRefuseACrossOriginPost(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	give, givePlan := s.proAction("ada@example.com", "pro"), s.proAction("ada@example.com", "pro-plan")
	for _, c := range []struct {
		path string
		form url.Values
	}{{give, nil}, {givePlan, nil}, {"/admin/cavetext/settings", url.Values{"question": {"Evil question?"}}},
		{"/admin/cavetext/master-prompts", masterPromptForm("Evil", "/evil", "x", "Dash", "/dash", "y")}} {
		for header, value := range map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"} {
			req, _ := http.NewRequest(http.MethodPost, s.server.URL+c.path, strings.NewReader(c.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set(header, value)
			res, body := s.do(req)
			expectStatus(t, res, body, http.StatusForbidden, "cross-origin request refused")
		}
	}
	if ada.me(token).Pro {
		t.Fatalf("a cross-origin post gave Pro")
	}
	res, body := s.get("/admin/cavetext/settings")
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(body, "Evil question?") {
		t.Fatalf("a cross-origin post changed the question")
	}
}

func TestTheCaveTextPageShowsAVisitorWhatProGivesAndAsksThemToLogIn(t *testing.T) {
	s := caveTextAdminSite(t)
	visitor := s.second()
	res, body := visitor.get("/cavetext")
	expectStatus(t, res, body, http.StatusOK, "<h1>CaveText</h1>")
	for _, want := range []string{"Up to 2,000 Snippets", "Save up to 2,000 Snippets.", "RM25 a year", `href="/login?next=%2Fcavetext"`, `href="/register"`, `href="https://cavetext.cavelms.com"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the Visitor's CaveText page lacks %s", want)
		}
	}
	if strings.Contains(body, "Buy CaveText Pro") {
		t.Errorf("a Visitor is offered Buy")
	}
	_, body = visitor.get("/courses")
	if !strings.Contains(body, `<a href="/cavetext">CaveText</a>`) {
		t.Errorf("the site header has no CaveText link")
	}
}

func TestTheCaveTextPageShowsALearnersCapAndProAndBuysWithThePaymentLink(t *testing.T) {
	s := caveTextAdminSite(t)
	s.nextPlace(1001)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.fillSnippets(token, 3)

	// No payment link: contact us, as on the Lab page.
	res, body := ada.get("/cavetext")
	expectStatus(t, res, body, http.StatusOK, "3 of 5 Snippets")
	for _, want := range []string{"Free", "RM25 a year", `href="mailto:adam.caveconsult@gmail.com?subject=CaveText%20Pro">Contact us to buy CaveText Pro</a>`, `href="https://cavetext.cavelms.com"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the free Learner's CaveText page lacks %s", want)
		}
	}
	if strings.Contains(body, "Buy CaveText Pro</a>") {
		t.Errorf("Buy shows with no payment link: %s", body)
	}
	if strings.Contains(body, "processing fee") {
		t.Errorf("the gateway fee note shows with no payment link")
	}

	// With a payment link and a new price, Buy opens it.
	s.savePrices(map[string]string{"cavetext-pro_price": "30.50", "cavetext-pro_link": "https://bcl.my/pay/cavetext"})
	_, body = ada.get("/cavetext")
	for _, want := range []string{"RM30.50 a year", "A payment processing fee, which depends on the payment method you choose, is added at checkout.", `href="https://bcl.my/pay/cavetext"`, "Buy CaveText Pro</a>"} {
		if !strings.Contains(body, want) {
			t.Errorf("with a payment link the CaveText page lacks %s", want)
		}
	}
	if strings.Contains(body, "Contact us to buy CaveText Pro") {
		t.Errorf("the page still says Contact us with a payment link set")
	}

	// Pro: up to 2,000, with the expiry.
	s.givePro("ada@example.com")
	until := text.Date("en", ada.me(token).ProUntil.UTC())
	_, body = ada.get("/cavetext")
	for _, want := range []string{"CaveText Pro until " + until, "3 Snippets, up to 2,000"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Pro Learner's CaveText page lacks %q", want)
		}
	}

	// Lapsed over the free cap: keeps every Snippet, can't add.
	for _, c := range "defg" {
		ada.createSnippet(token, snippet{Folder: "Work", Name: "Extra " + string(c), Shortcut: "/extra-" + string(c), Body: "x"})
	}
	s.agePro(400)
	_, body = ada.get("/cavetext")
	for _, want := range []string{"7 of 5 Snippets", "keep, use and edit all 7", "Free"} {
		if !strings.Contains(body, want) {
			t.Errorf("the lapsed Learner's CaveText page lacks %q", want)
		}
	}
}

func TestTheCaveTextPageAsksALearnerNewToCaveTextToSignIn(t *testing.T) {
	s := caveTextAdminSite(t)
	cy := s.second()
	cy.loginAsNewLearner("Cy Tan", "cy@example.com", "correct horse battery")
	res, body := cy.get("/cavetext")
	expectStatus(t, res, body, http.StatusOK, "Sign in to CaveText")
	for _, want := range []string{"RM25 a year", `href="https://cavetext.cavelms.com"`, "10 Snippets free"} {
		if !strings.Contains(body, want) {
			t.Errorf("the new Learner's CaveText page lacks %s", want)
		}
	}
	if n := strings.Count(body, "Get the CaveText extension"); n != 1 {
		t.Errorf("the new Learner's CaveText page links the extension %d times, want once", n)
	}
}

func TestAProImportStopsAtTwoThousandSnippetsAndAFullProLearnerCannotAddAnother(t *testing.T) {
	s := caveTextAdminSite(t)
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	set := make([]snippet, 2001)
	for i := range set {
		set[i] = snippet{Folder: "Kept", Name: fmt.Sprintf("I%d", i), Shortcut: fmt.Sprintf("/i%04d", i), Body: "x"}
	}
	res, body := ada.caveTextSend(http.MethodPut, "snippets", token, map[string]any{"snippets": set})
	expectStatus(t, res, body, http.StatusOK, `"skipped":1`)
	if want := "Imported 2000 Snippets, the most CaveText Pro keeps; the other one was skipped."; !strings.Contains(body, want) {
		t.Fatalf("answer %s, want message %q", body, want)
	}
	if n := len(ada.snippets(token)); n != 2000 {
		t.Fatalf("Snippets = %d, want 2000", n)
	}
	res, body = ada.caveTextSend(http.MethodPost, "snippets", token, snippet{Folder: "Work", Name: "More", Shortcut: "/more", Body: "x"})
	expectStatus(t, res, body, http.StatusForbidden, "You have 2000 Snippets, the most CaveText Pro keeps. Delete one to add another.")
}
