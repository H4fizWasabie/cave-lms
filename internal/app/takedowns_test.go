package app_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var takedownAction = regexp.MustCompile(`action="(/admin/deployments/\d+/takedown)"`)

func TestAdminSeesEveryDeploymentAwakeOnesFirst(t *testing.T) {
	admin, ada := labLearner(t)
	asleep := ada.deployAs(ada.readyDashboard("Kewangan Lama"), "kewangan-lama")
	admin.ageAllPeriods(31) // Ada's Plan ends
	awake := admin.deploy(admin.readyDashboard("Kewangan Baru"))

	res, body := admin.get("/admin/deployments")
	for _, want := range []string{"Ada", "ada@example.com", "Kewangan Baru", "Awake", "Asleep: the Learner has no active Plan",
		"<code>kewangan-lama</code>", `href="` + asleep + `"`, `href="` + awake + `"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Index(body, awake) > strings.Index(body, asleep) {
		t.Fatalf("the awake Deployment is not listed first: %s", body)
	}
	res, body = admin.get("/admin")
	expectStatus(t, res, body, http.StatusOK, `href="/admin/deployments"`)
}

func TestOnlyTheAdminSeesDeploymentsOrTakesThemDown(t *testing.T) {
	admin, ada := labLearner(t)
	link := ada.deploy(ada.readyDashboard("Kewangan"))
	_, body := admin.get("/admin/deployments")
	action := takedownAction.FindStringSubmatch(body)
	if action == nil {
		t.Fatalf("no Takedown form: %s", body)
	}
	res, body := ada.get("/admin/deployments")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin")
	res, body = ada.post(action[1], url.Values{"reason": {"mine"}})
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin")
	if res, _ := ada.second().get("/admin/deployments"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a Visitor got %d for the Deployments list", res.StatusCode)
	}
	if _, page := ada.second().get(link); !strings.Contains(page, `fetch("data/`) {
		t.Fatalf("a Learner took the Deployment down: %s", page)
	}
}

func TestTakedownNeedsAReasonAndSleepsTheDeploymentAtOnce(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	_, body := admin.get("/admin/deployments")
	action := takedownAction.FindStringSubmatch(body)[1]

	res, body := admin.post(action, url.Values{"reason": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Give a reason")
	if _, page := ada.second().get(link); !strings.Contains(page, `fetch("data/`) {
		t.Fatalf("a Takedown without a reason took the Deployment down: %s", page)
	}

	data := ada.second().dataOf(link)
	reason := "Shared under a real company's name."
	res, body = admin.post(action, url.Values{"reason": {reason}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for _, p := range []string{link, link + data} {
		res, page := ada.second().get(p)
		expectStatus(t, res, page, http.StatusOK, "Cave took it offline early")
		if strings.Contains(page, "Tarikh") {
			t.Fatalf("a taken-down link still shows the data: %s", page)
		}
	}
	res, body = admin.get("/admin/deployments")
	expectStatus(t, res, body, http.StatusOK, "Taken down")
	expectStatus(t, res, body, http.StatusOK, "Shared under a real company&#39;s name.")
	if takedownAction.MatchString(body) {
		t.Fatalf("a taken-down Deployment still offers Takedown: %s", body)
	}

	// The Learner keeps the Dashboard, marked as taken down.
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, `<a href="`+path+`">Kewangan</a>`)
	expectStatus(t, res, body, http.StatusOK, "Taken down")
	res, body = ada.get(path + "/deploy")
	expectStatus(t, res, body, http.StatusOK, "Cave took this Deployment offline on")
	expectStatus(t, res, body, http.StatusOK, "Shared under a real company&#39;s name.")

	// A renewal does not wake it.
	admin.ageAllPeriods(31)
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	res, page := ada.second().get(link + data)
	expectStatus(t, res, page, http.StatusOK, "Cave took it offline early")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 of 3")
}
