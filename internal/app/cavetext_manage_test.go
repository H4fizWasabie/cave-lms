package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// learnerHas maps each email on the Admin CaveText Learners page to what its Has column says, chips joined by ", ".
func learnerHas(t *testing.T, s *site) map[string]string {
	t.Helper()
	res, body := s.get("/admin/cavetext")
	expectStatus(t, res, body, http.StatusOK, "")
	row := regexp.MustCompile(`<tr>\s*<td>[^<]+<small>([^<]+)</small><small>[^<]+</small></td>\s*<td>(.*?)</td>`)
	tags := regexp.MustCompile(`<[^>]+>`)
	got := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(body, -1) {
		got[m[1]] = tags.ReplaceAllString(strings.ReplaceAll(m[2], "</li>", "|"), "")
	}
	return got
}

func shortDate(d time.Time) string { return d.In(malaysiaTime).Format("2 Jan 2006") }

func TestTheCaveTextLearnersPageShowsWhatEachLearnerHasWithWhenItEnds(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.setUpEbooks()
	for _, l := range [][2]string{{"Ada", "ada@example.com"}, {"Bea", "bea@example.com"}, {"Cy", "cy@example.com"}, {"Dee", "dee@example.com"}} {
		s.newCaveTextLearner(l[0], l[1])
	}
	for email, action := range map[string]string{"ada@example.com": "combo", "bea@example.com": "pro-plan", "cy@example.com": "pro"} {
		res, body := s.post(s.proAction(email, action), confirming)
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	year, month := shortDate(time.Now().AddDate(1, 0, 0)), shortDate(time.Now().AddDate(0, 0, 30))
	want := map[string]string{
		"ada@example.com": "Pro until " + year + "|Lab until " + month + "|Ebooks until " + year + "|",
		"bea@example.com": "Pro until " + year + "|Lab until " + month + "|",
		"cy@example.com":  "Pro until " + year + "|",
		"dee@example.com": "Free",
	}
	for email, w := range want {
		if got := learnerHas(t, s)[email]; got != w {
			t.Errorf("%s has %q, want %q", email, got, w)
		}
	}

	// A second year stacks behind the first: the chip shows where the stack ends, a lapsed Pro shows no chip.
	s.givePro("cy@example.com")
	if got, w := learnerHas(t, s)["cy@example.com"], "Pro until "+shortDate(time.Now().AddDate(2, 0, 0))+"|"; got != w {
		t.Errorf("cy@example.com has %q after a second year, want %q", got, w)
	}
	s.endPro("cy@example.com")
	if got := learnerHas(t, s)["cy@example.com"]; got != "Free" {
		t.Errorf("cy@example.com has %q after End Pro, want Free", got)
	}
	if got := learnerHas(t, s)["ada@example.com"]; !strings.Contains(got, "Lab until") {
		t.Errorf("ending Pro for another Learner took Ada's Lab: %q", got)
	}
}

func TestAPaidGiveAsksOnItsConfirmationPageAndGivesNothingUntilConfirmed(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.setUpEbooks()
	ada, token := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	year, month := shortDate(time.Now().AddDate(1, 0, 0)), shortDate(time.Now().AddDate(0, 0, 30))
	manage := s.managePage("ada@example.com")

	for _, c := range []struct{ action, heading, question string }{
		{"pro", "Give CaveText Pro", "Give Ada Lovelace CaveText Pro? It gives a year of CaveText Pro, until " + year + "."},
		{"pro-plan", "Give Pro Plan", "Give Ada Lovelace Pro Plan? It gives a year of CaveText Pro, until " + year + ", and 30 days of the Lab Plan, until " + month + "."},
		{"combo", "Give COMBO Pro Plan", "Give Ada Lovelace COMBO Pro Plan? It gives a year of CaveText Pro, until " + year + ", 30 days of the Lab Plan, until " + month + ", and a year of Ebook access, until " + year + "."},
	} {
		page := s.proAction("ada@example.com", c.action)
		res, body := s.post(page, nil)
		expectRedirect(t, res, body, page)
		if ada.me(token).Pro || len(s.periodsOf("ada@example.com")) != 0 {
			t.Fatalf("an unconfirmed %s gave something", c.action)
		}
		res, body = s.get(page)
		expectStatus(t, res, body, http.StatusOK, c.question)
		expectAll(t, body, "<h1>"+c.heading+"</h1>", `class="btn btn-solid">`+c.heading, `href="`+manage+`"`)
		expectNone(t, body, "btn-danger-solid")
	}

	// Confirming gives, and goes back to the Manage page.
	res, body := s.post(s.proAction("ada@example.com", "combo"), confirming)
	expectRedirect(t, res, body, manage)
	if !ada.me(token).Pro || len(s.periodsOf("ada@example.com")) != 1 {
		t.Fatalf("a confirmed COMBO did not give Pro and a Lab Plan period")
	}

	// A second give starts when the first ends, and the question says so.
	_, body = s.get(s.proAction("ada@example.com", "pro"))
	expectAll(t, body, "a year of CaveText Pro, until "+shortDate(time.Now().AddDate(2, 0, 0)))

	// End Pro is still the red one.
	_, body = s.get(s.proAction("ada@example.com", "end-pro"))
	expectAll(t, body, `btn-danger-solid`)
}

func TestTheManagePageShowsWhatALearnerHasAndTheGivesInPlainWords(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.setUpEbooks()
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	manage := s.managePage("ada@example.com")
	year, month := shortDate(time.Now().AddDate(1, 0, 0)), shortDate(time.Now().AddDate(0, 0, 30))

	res, body := s.get(manage)
	expectStatus(t, res, body, http.StatusOK, "<h1>Ada Lovelace</h1>")
	expectAll(t, body, "Free: no Pro, Lab Plan or Ebook access.",
		"Gives a year of CaveText Pro, until "+year+".",
		"Gives a year of CaveText Pro, until "+year+", and 30 days of the Lab Plan, until "+month+".",
		"Gives a year of CaveText Pro, until "+year+", 30 days of the Lab Plan, until "+month+", and a year of Ebook access, until "+year+".")
	expectNone(t, body, "Email ebook link", "End Pro now")

	s.givePro("ada@example.com")
	_, body = s.get(manage)
	expectAll(t, body, "What they have", "Pro", "until "+time.Now().AddDate(1, 0, 0).In(malaysiaTime).Format("2 January 2006"), "End Pro now", "ctl-danger")
	expectNone(t, body, "Free: no Pro")
	// Pro is running, so a give starts when it ends.
	expectAll(t, body, "Gives a year of CaveText Pro, until "+shortDate(time.Now().AddDate(2, 0, 0)))

	res, body = s.get("/admin/cavetext/learners/999999")
	expectStatus(t, res, body, http.StatusNotFound, "No CaveText Learner")
	res, body = ada.get(manage)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
}

func TestTheManagePageNamesNoLabPlanWhenNoneIsMarked(t *testing.T) {
	t.Parallel()
	s := bclSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	_, body := s.get(s.managePage("ada@example.com"))
	expectAll(t, body, "with no Lab Plan because no Plan is marked as the Lab Plan")
}
