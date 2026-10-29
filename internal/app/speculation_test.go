package app_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestEveryRenderedPageNamesTheSpeculationRules(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, _ := s.get("/login")
	if got := res.Header.Get("Speculation-Rules"); got != `"/speculation-rules.json"` {
		t.Errorf("Speculation-Rules = %q, want the rules' address in quotes", got)
	}
}

func TestTheSpeculationRulesPrerenderOnlyTheClassroomTabsOnHover(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.get("/speculation-rules.json")
	expectStatus(t, res, body, http.StatusOK, "prerender")
	if got := res.Header.Get("Content-Type"); got != "application/speculationrules+json" {
		t.Errorf("Content-Type = %q, want application/speculationrules+json", got)
	}
	if res.Header.Get("Cache-Control") == "" {
		t.Error("the rules carry no Cache-Control")
	}
	var rules struct {
		Prerender []struct {
			Source    string `json:"source"`
			Eagerness string `json:"eagerness"`
			Where     struct {
				SelectorMatches string `json:"selector_matches"`
			} `json:"where"`
		} `json:"prerender"`
		Prefetch []any `json:"prefetch"`
	}
	if err := json.Unmarshal([]byte(body), &rules); err != nil {
		t.Fatalf("the rules are not JSON: %v: %s", err, body)
	}
	if len(rules.Prerender) != 1 || len(rules.Prefetch) != 0 {
		t.Fatalf("want one prerender rule and no others, got %s", body)
	}
	if r := rules.Prerender[0]; r.Source != "document" || r.Eagerness != "moderate" || r.Where.SelectorMatches != ".classroom-tabs a" {
		t.Errorf("rule = %+v, want a document rule at moderate eagerness for .classroom-tabs a only", r)
	}
}
