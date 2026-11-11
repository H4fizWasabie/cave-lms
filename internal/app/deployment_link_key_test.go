package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// keyedLink is a Deployment's link with its Link key (ADR 0027): the name, a dash, then 10 lowercase letters and digits.
var keyedLink = regexp.MustCompile(`^` + pagesOrigin + `/ada-example-com/kedai-ada-2026-([a-z0-9]{10})/$`)

// keyless turns every Deployment into one made before Link keys and returns link as it was then: without its key.
func (s *site) keyless(link string) string {
	s.t.Helper()
	s.exec(`UPDATE deployments SET link_key = NULL`)
	return link[:len(link)-len("-0123456789/")] + "/"
}

// A new Deployment's link carries a Link key, and only that exact link opens it: not its bare name, the earlier forms, a
// wrong key or none, and none of them redirects, which would hand out the key. A wrong Username with the right key
// still redirects, keeping the key. A name that holds dashes reads the same.
func TestOnlyADeploymentsKeyedLinkOpensItAndNothingElseRedirectsToIt(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	link := ada.deployAs(ada.readyDashboard("Kewangan"), "kedai-ada-2026")
	m := keyedLink.FindStringSubmatch(link)
	if m == nil {
		t.Fatalf("the link is %s, want the Username, the name, a dash and a 10 character Link key", link)
	}
	key := m[1]
	visitor := ada.second()
	res, body := visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, `fetch("data/`)
	data := visitor.dataOf(link)
	res, body = visitor.get(link + data)
	expectStatus(t, res, body, http.StatusOK, `"Tarikh"`)
	res, body = visitor.get(link + "charts.js")
	expectStatus(t, res, body, http.StatusOK, "function drawDashboard")

	for _, p := range []string{
		"/ada-example-com/kedai-ada-2026/", "/ada-example-com/kedai-ada-2026", "/kedai-ada-2026/", "/kedai-ada-2026/" + data,
		"/ada-example-com/kedai-ada-2026/" + data, "/ada-example-com/kedai-ada-2026/charts.js", "/nobody/kedai-ada-2026/",
		"/ada-example-com/kedai-ada-2026-wrongkey12/", "/ada-example-com/kedai-ada-2026-wrongkey12/" + data,
		"/ada-example-com/kedai-ada-2026-" + key[:9] + "/", "/ada-example-com/kedai-ada-" + key + "/",
		"/ada-example-com/kedai-" + key + "/", "/ada-example-com/" + key + "/", "/ada-example-com/kedai-ada-2026-/",
		"/nobody/kedai-ada-2026-wrongkey12/", "/kedai-ada-2026-wrongkey12/",
	} {
		res, body := visitor.get(pagesOrigin + p)
		if res.StatusCode != http.StatusNotFound || res.Header.Get("Location") != "" || strings.Contains(body, key) {
			t.Errorf("GET %s = %d to %q, want plain not found: %s", p, res.StatusCode, res.Header.Get("Location"), body)
		}
	}
	// The main host's old /d/... link only forwards the path, and the Deployments host then refuses it.
	res, _ = visitor.get("/d/ada-example-com/kedai-ada-2026/")
	if res.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("the old link went %d", res.StatusCode)
	}
	if res, _ = visitor.get(res.Header.Get("Location")); res.StatusCode != http.StatusNotFound {
		t.Fatalf("the old link followed to the Deployments host went %d, want not found", res.StatusCode)
	}

	path := pagesPath(link)
	for old, want := range map[string]string{
		"/ada-example-com/kedai-ada-2026-" + key:         path,
		"/nobody/kedai-ada-2026-" + key + "/":            path,
		"/nobody/kedai-ada-2026-" + key + "/" + data:     path + data,
		"/nobody/kedai-ada-2026-" + key + "/charts.js?v": path + "charts.js?v",
		"/nobody/kedai-ada-2026-" + key + "/data.json":   path + "data.json",
	} {
		res, _ := visitor.get(pagesOrigin + old)
		if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != want {
			t.Errorf("GET %s = %d to %q, want 301 to %q", old, res.StatusCode, res.Header.Get("Location"), want)
		}
	}
}

// With the right key a sleeping or taken-down Deployment says it is inactive, as before; without it the link is not
// found, so the message never confirms a name exists.
func TestAnInactiveDeploymentSaysSoOnlyAtItsKeyedLink(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	link := ada.deployAs(ada.readyDashboard("Kewangan"), "kedai-ada-2026")
	ada.exec(`UPDATE deployments SET taken_down_at = now(), takedown_reason = 'test'`)
	visitor := ada.second()
	res, body := visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, "This Dashboard is asleep")
	res, body = visitor.get(pagesOrigin + "/ada-example-com/kedai-ada-2026/")
	expectStatus(t, res, body, http.StatusNotFound, "There is no Dashboard at this link")
}

// A Deployment from before Link keys keeps its keyless link and the redirects of ADR 0023 until its Learner presses the button.
func TestADeploymentMadeBeforeLinkKeysKeepsItsKeylessLinkAndRedirects(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.keyless(ada.deployAs(path, "kedai-ada-2026"))
	if link != pagesOrigin+"/ada-example-com/kedai-ada-2026/" {
		t.Fatalf("keyless link %s", link)
	}
	visitor := ada.second()
	res, body := visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, `fetch("data/`)
	for _, old := range []string{"/kedai-ada-2026/", "/nobody/kedai-ada-2026/"} {
		res, _ := visitor.get(pagesOrigin + old)
		if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != pagesPath(link) {
			t.Errorf("GET %s = %d to %q, want 301 to %q", old, res.StatusCode, res.Header.Get("Location"), pagesPath(link))
		}
	}
	// A keyless Deployment's Deploy tab says its link can be guessed, and a keyed one's does not.
	res, body = ada.get(path + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "Anyone who guesses this link can open it")
	expectStatus(t, res, body, http.StatusOK, "New secret link")
}

// New secret link asks first, then gives the Deployment a fresh key: the old link is not found, the new one opens, and a
// new Dataset keeps the key. Only the owner can do it. It works any number of times.
func TestNewSecretLinkReplacesTheKeyAfterAConfirmationAndOnlyTheOwnerCanDoIt(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deployAs(path, "kedai-ada-2026")
	visitor := ada.second()

	res, body := ada.get(path + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "New secret link")
	if strings.Contains(body, "Anyone who guesses this link") {
		t.Fatalf("a keyed Deployment's Deploy tab says its link can be guessed: %s", body)
	}
	res, body = ada.get(path + "/deploy?confirm")
	for _, want := range []string{"Links you have shared will stop working", "typed in full", "./data.json", `action="` + path + `/deploy/key"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	// The confirm page is only a page: nothing changed.
	if res, _ := visitor.get(link); res.StatusCode != http.StatusOK {
		t.Fatalf("the link stopped working before the confirmation: %d", res.StatusCode)
	}

	bob := admin.second()
	bob.loginAsNewLearner("Bob", "bob@example.com", "correct horse battery")
	admin.assignPlan("bob@example.com", admin.onlyPlanID())
	if res, _ := bob.post(path+"/deploy/key", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner's new secret link went %d, want not found", res.StatusCode)
	}
	if res, _ := visitor.post(path+"/deploy/key", nil); res.StatusCode == http.StatusSeeOther && res.Header.Get("Location") == path+"/deploy" {
		t.Fatalf("a Visitor replaced the key")
	}
	if res, _ := visitor.get(link); res.StatusCode != http.StatusOK {
		t.Fatalf("the link stopped working for a stranger's request: %d", res.StatusCode)
	}

	seen := map[string]bool{link: true}
	for range 2 {
		res, body = ada.post(path+"/deploy/key", url.Values{})
		expectStatus(t, res, body, http.StatusSeeOther, "")
		_, tab := ada.get(path + "/deploy")
		fresh := deploymentLink.FindStringSubmatch(tab)[1]
		if seen[fresh] || !keyedLink.MatchString(fresh) {
			t.Fatalf("the new link is %s, want a fresh keyed link (seen %v)", fresh, seen)
		}
		for old := range seen {
			if res, _ := visitor.get(old); res.StatusCode != http.StatusNotFound {
				t.Fatalf("the old link %s went %d, want not found", old, res.StatusCode)
			}
		}
		seen[fresh] = true
		res, body = visitor.get(fresh)
		expectStatus(t, res, body, http.StatusOK, `fetch("data/`)
		link = fresh
	}

	// A new Dataset keeps the key.
	res, body = ada.postFile(path+"/dataset", "new.xlsx", ada.financeSample())
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if _, tab := ada.get(path + "/deploy"); !strings.Contains(tab, `href="`+link+`"`) {
		t.Fatalf("a new Dataset changed the link from %s: %s", link, tab)
	}
}

// A keyless Deployment gets a key from the button, and the Admin's list shows the full link.
func TestNewSecretLinkKeysAKeylessDeploymentAndTheAdminSeesTheFullLink(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	ada.keyless(ada.deployAs(path, "kedai-ada-2026"))
	res, body := ada.post(path+"/deploy/key", url.Values{})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, tab := ada.get(path + "/deploy")
	link := deploymentLink.FindStringSubmatch(tab)[1]
	if !keyedLink.MatchString(link) {
		t.Fatalf("the link after the button is %s", link)
	}
	res, body = admin.get("/admin/deployments")
	expectStatus(t, res, body, http.StatusOK, `href="`+link+`"`)
}
