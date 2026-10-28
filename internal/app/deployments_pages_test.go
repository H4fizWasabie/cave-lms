package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Deployments live on the Deployments host alone (ADR 0024): the main host never serves one, and the Deployments host
// serves nothing of the LMS, so a page posing as a login screen is never on the host that holds the real one.
func TestTheMainHostNeverServesADeploymentAndTheDeploymentsHostServesNothingElse(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	link := ada.deployAs(ada.readyDashboard("Kewangan"), "kedai-ada")
	visitor := ada.second()
	data := visitor.dataOf(link)
	path := pagesPath(link)

	for _, p := range []string{path, path + "charts.js", path + data} {
		if res, _ := visitor.get(p); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s on the main host = %d, want 404: Deployments are not served there", p, res.StatusCode)
		}
	}

	// Even Ada, with her session, finds none of the LMS on the Deployments host.
	for _, who := range []*site{visitor, ada} {
		for _, p := range []string{"/", "/login", "/register", "/lab", "/courses", "/admin", "/gallery", "/privacy", "/static/cavelms.css", "/ada-example-com", "/ada-example-com/"} {
			res, body := who.get(pagesOrigin + p)
			expectStatus(t, res, body, http.StatusNotFound, "")
			if c := res.Header.Get("Set-Cookie"); c != "" {
				t.Errorf("GET %s on the Deployments host set a cookie: %s", p, c)
			}
			if strings.Contains(body, `href="/`) || strings.Contains(body, "<form") {
				t.Errorf("GET %s on the Deployments host shows the site's links or forms: %s", p, body)
			}
		}
	}
	// A trailing dot or the default port is still the Deployments host, not the LMS.
	for _, host := range []string{"https://pages.cave.example.invalid.", "https://pages.cave.example.invalid:443", "https://PAGES.cave.example.invalid"} {
		if res, _ := visitor.get(host + "/login"); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET /login at %s = %d, want 404: it is the Deployments host", host, res.StatusCode)
		}
	}
	res, body := visitor.post(pagesOrigin+"/login", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}})
	if res.StatusCode != http.StatusMethodNotAllowed || res.Header.Get("Set-Cookie") != "" {
		t.Fatalf("POST /login on the Deployments host = %d, cookie %q: %s", res.StatusCode, res.Header.Get("Set-Cookie"), body)
	}
}

// Every link shared before the move, cavelms.com/d/..., redirects for good to the same path on the Deployments host,
// keeping the file and the query. The Deployments host then redirects a wrong Username, as before.
func TestAnOldDeploymentLinkRedirectsForGoodToTheDeploymentsHost(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	link := ada.deployAs(ada.readyDashboard("Kewangan"), "kedai-ada")
	visitor := ada.second()
	data := visitor.dataOf(link)

	for old, want := range map[string]string{
		"/d/ada-example-com/kedai-ada/":                pagesOrigin + "/ada-example-com/kedai-ada/",
		"/d/ada-example-com/kedai-ada/" + data:         pagesOrigin + "/ada-example-com/kedai-ada/" + data,
		"/d/ada-example-com/kedai-ada/charts.js":       pagesOrigin + "/ada-example-com/kedai-ada/charts.js",
		"/d/ada-example-com/kedai-ada/data.json":       pagesOrigin + "/ada-example-com/kedai-ada/data.json",
		"/d/ada-example-com/kedai-ada/charts.js?v=1&a": pagesOrigin + "/ada-example-com/kedai-ada/charts.js?v=1&a",
		"/d/ada-example-com/kedai-ada":                 pagesOrigin + "/ada-example-com/kedai-ada",
		"/d/kedai-ada/":                                pagesOrigin + "/kedai-ada/",
		"/d/nobody/kedai-ada/" + data:                  pagesOrigin + "/nobody/kedai-ada/" + data,
		"/d/nope/":                                     pagesOrigin + "/nope/",
	} {
		res, _ := visitor.get(old)
		if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != want {
			t.Errorf("GET %s = %d to %q, want 301 to %q", old, res.StatusCode, res.Header.Get("Location"), want)
		}
		// A sandboxed page still open at an old link fetches its data from a null origin, through the redirect.
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("GET %s allows origin %q, want *", old, got)
		}
		if got := res.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s has Cache-Control %q, want no-store", old, got)
		}
	}

	// Two hops from a wrong Username, and the second lands on the Deployment.
	res, _ := visitor.get("/d/nobody/kedai-ada/charts.js")
	res, _ = visitor.get(res.Header.Get("Location"))
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != pagesPath(link)+"charts.js" {
		t.Fatalf("the second hop = %d to %q, want 301 to the current address", res.StatusCode, res.Header.Get("Location"))
	}
	res, body := visitor.get(link + "charts.js")
	expectStatus(t, res, body, http.StatusOK, "function drawDashboard")

	// The old link to a page Deployment's page and data.json redirect the same way.
	pageLink := ada.deployAs(ada.createPage("Jualan", myPage), "jualan-ada")
	res, _ = visitor.get("/d/ada-example-com/jualan-ada/")
	if res.Header.Get("Location") != pageLink {
		t.Fatalf("the old page link went to %q, want %q", res.Header.Get("Location"), pageLink)
	}
}

// The session and language cookies are host-only: with no Domain attribute, a browser never sends them to the
// Deployments host, which shares the parent domain (ADR 0024).
func TestSessionAndLanguageCookiesAreHostOnly(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	s.loginAsNewLearner("Ada Lovelace", "ada@example.com", "correct horse battery")
	login, _ := s.login("ada@example.com", "correct horse battery")
	language, _ := s.switchLanguage("ms", "/")
	for name, res := range map[string]*http.Response{"login": login, "language": language} {
		if len(res.Cookies()) == 0 {
			t.Fatalf("%s set no cookie", name)
		}
		for _, c := range res.Header.Values("Set-Cookie") {
			if strings.Contains(strings.ToLower(c), "domain") {
				t.Errorf("%s set a cookie with a Domain: %s", name, c)
			}
		}
	}
}
