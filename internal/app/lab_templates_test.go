package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestVisitorCannotBuild(t *testing.T) {
	s := newSite(t)
	res, _ := s.get("/lab/new")
	if res.StatusCode == http.StatusOK {
		t.Fatalf("a Visitor opened New Dashboard")
	}
	res, _ = s.post("/lab/dashboards", url.Values{"template": {"inventory"}, "name": {"Stok"}})
	if loc := res.Header.Get("Location"); strings.HasPrefix(loc, "/lab/dashboards/") {
		t.Fatalf("a Visitor created a Dashboard: %s", loc)
	}
}

func TestLearnerWithoutAccessCannotBuild(t *testing.T) {
	_, ada, _, _ := labSetup(t)
	res, body := ada.get("/lab/new")
	expectRedirect(t, res, body, "/pricing")
	res, body = ada.post("/lab/dashboards", url.Values{"name": {"Staf"}, "html": {myPage}})
	expectStatus(t, res, body, http.StatusForbidden, "The Lab is locked")
}

func TestGeneralNeedsNoColumnChoices(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("general", "Mine")
	res, body := ada.postFile(path+"/dataset", "general.xlsx", labFile(t, "general.xlsx"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, preview := ada.get(path)
	expectStatus(t, res, preview, http.StatusOK, "")
	if strings.Contains(preview, `class="field map-row"`) {
		t.Errorf("General's upload preview asks for columns:\n%s", preview)
	}
	ada.confirmWork(path, nil)
	res, body = ada.get(path + "/view/")
	for _, want := range []string{`"Jualan by Cawangan"`, `"Jualan by month"`, `"Latest rows"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
