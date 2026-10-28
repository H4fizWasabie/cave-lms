package app_test

import (
	"net/http"
	"net/url"
	"testing"
)

// adminWithTwoLearners has Nurul Aisyah and Nurul Huda registered, plus Ada, and one 30-day Plan.
func adminWithTwoLearners(t *testing.T) (*site, string) {
	s, planID := adminWithPlanAndLearner(t)
	s.second().loginAsNewLearner("Nurul Aisyah", "aisyah@example.com", "correct horse battery")
	s.second().loginAsNewLearner("Nurul Huda", "huda@kampung.example", "correct horse battery")
	return s, planID
}

func TestAdminFindsALearnerForPlanPeriodsByPartOfTheNameOrEmail(t *testing.T) {
	t.Parallel()
	s, _ := adminWithTwoLearners(t)
	for q, want := range map[string]string{
		"aisy":               "Plan periods: Nurul Aisyah",
		"AISYAH@exa":         "Plan periods: Nurul Aisyah",
		"kampung":            "Plan periods: Nurul Huda",
		"aisyah@example.com": "Plan periods: Nurul Aisyah",
	} {
		res, body := s.get("/admin/plans/periods?email=" + url.QueryEscape(q))
		expectStatus(t, res, body, http.StatusOK, want)
		expectNone(t, body, "Plan periods: Ada")
	}
	res, body := s.get("/admin/plans/periods?email=zainal")
	expectStatus(t, res, body, http.StatusNotFound, "No Learner has the email zainal")
}

func TestSeveralLearnersMatchingAreListedToChooseFrom(t *testing.T) {
	t.Parallel()
	s, _ := adminWithTwoLearners(t)
	res, body := s.get("/admin/plans/periods?email=nurul")
	expectStatus(t, res, body, http.StatusOK, "aisyah@example.com")
	expectAll(t, body, "huda@kampung.example", "Nurul Huda")
	expectNone(t, body, "ada@example.com")
}

func TestWildcardsInTheLearnerLookupAreLiteral(t *testing.T) {
	t.Parallel()
	s, _ := adminWithTwoLearners(t)
	for _, q := range []string{"%", "_", "Nu%h"} {
		res, body := s.get("/admin/plans/periods?email=" + url.QueryEscape(q))
		expectStatus(t, res, body, http.StatusNotFound, "No Learner has")
		expectNone(t, body, "aisyah@example.com", "ada@example.com")
	}
}

func TestAPlanIsOnlyAssignedToAnExactEmail(t *testing.T) {
	t.Parallel()
	s, planID := adminWithTwoLearners(t)
	res, body := s.assignPlan("aisy", planID)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "No Learner has the email aisy")
	if got := s.periodsOf("aisyah@example.com"); len(got) != 0 {
		t.Fatalf("a partial lookup assigned a Plan: %+v", got)
	}
	res, body = s.assignPlan("AISYAH@example.com", planID)
	expectStatus(t, res, body, http.StatusSeeOther, "")
}
