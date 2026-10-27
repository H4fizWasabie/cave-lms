package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestANewDashboardOffersNoTemplateOrKindAndAPastedOneIsItsPage(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab/new")
	expectStatus(t, res, body, http.StatusOK, `name="name"`)
	for _, gone := range []string{`name="template"`, `name="kind"`, "/lab/new?t=", "kind=work"} {
		if strings.Contains(body, gone) {
			t.Fatalf("the New Dashboard page still offers a choice (%s):\n%s", gone, body)
		}
	}

	// Whatever else the form says, the Dashboard is its page.
	for _, form := range []url.Values{{"name": {"Kedai"}}, {"name": {"Lama"}, "template": {"finance"}}, {"name": {"Kerja"}, "kind": {"practice"}}} {
		form.Set("html", myPage)
		res, body = ada.post("/lab/dashboards", form)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		path := res.Header.Get("Location")
		res, body = ada.get(path + "/columns")
		expectStatus(t, res, body, http.StatusNotFound, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusOK, "Your page")
	}
}

// A built Dashboard, on General, is offered no Template, even on data a Template fits, and its blocks come from the
// General rules whatever its confirm names (ADR 0019).
func TestABuiltDashboardIsOfferedNoTemplateAndBuildsByTheGeneralRules(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")

	for _, p := range []string{path, path + "?template=finance"} {
		res, body = ada.get(p)
		expectStatus(t, res, body, http.StatusOK, `action="`+path+`/dataset/confirm"`)
		if strings.Contains(body, `href="`+path+`?template=`) || strings.Contains(body, `name="template"`) || strings.Contains(body, "Money in") {
			t.Fatalf("%s offers a Template:\n%s", p, body)
		}
	}

	ada.confirmWork(path, url.Values{"template": {"finance"}, "date": {"Tarikh"}, "amount": {"Jumlah"}, "type": {"Jenis"}})
	if _, index := ada.get(path + "/view/"); strings.Contains(index, `"label": "Money in"`) {
		t.Fatalf("confirming with Finance named opened Finance:\n%s", index)
	}
}

// The Samples are gone (ADR 0019): naming one starts no Dashboard, as the form then has no page.
func TestASampleNoLongerStartsADashboard(t *testing.T) {
	_, ada := labLearner(t)
	for _, key := range []string{"finance", "general"} {
		res, body := ada.post("/lab/dashboards", url.Values{"name": {"Cuba"}, "sample": {key}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	}
	if _, body := ada.get("/lab"); !strings.Contains(body, "No Dashboards yet.") {
		t.Fatalf("naming a Sample made a Dashboard:\n%s", body)
	}
}
