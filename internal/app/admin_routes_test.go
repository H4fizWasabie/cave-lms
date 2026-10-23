package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ownerRoutes are the /admin routes only the Owner may know of; everyone else gets a 404 there.
var ownerRoutes = map[string]bool{
	"GET /admin/log":            true,
	"GET /admin/cavetext/caps":  true,
	"POST /admin/cavetext/caps": true,
}

var wildcard = regexp.MustCompile(`\{[^}]*\}`)

// Every registered /admin route refuses a Visitor and a Learner, so a route added without its guard fails here.
func TestEveryAdminRouteRefusesNonAdmins(t *testing.T) {
	s := ownerSite(t)
	visitor := s.second()
	learner := s.second()
	learner.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	admin := s.second()
	admin.loginAsNewAdmin("Bea", "bea@example.com", "correct horse battery")

	seen := 0
	for _, route := range s.application.Routes() {
		method, path, _ := strings.Cut(route, " ")
		if path != "/admin" && !strings.HasPrefix(path, "/admin/") {
			continue
		}
		seen++
		path = strings.TrimSuffix(wildcard.ReplaceAllString(path, "1"), "{$}")
		send := func(c *site) *http.Response {
			req, err := http.NewRequest(method, c.server.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, _ := c.do(req)
			return res
		}
		if ownerRoutes[route] {
			for who, c := range map[string]*site{"Visitor": visitor, "Learner": learner, "Admin who isn't the Owner": admin} {
				if res := send(c); res.StatusCode != http.StatusNotFound {
					t.Errorf("%s: %s got %d, want 404", route, who, res.StatusCode)
				}
			}
			continue
		}
		if res := send(visitor); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
			t.Errorf("%s: Visitor got %d to %q, want 303 to /login", route, res.StatusCode, res.Header.Get("Location"))
		}
		if res := send(learner); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: Learner got %d, want 403", route, res.StatusCode)
		}
	}
	if seen < 80 {
		t.Fatalf("walked %d /admin routes, want every registered one", seen)
	}
}
