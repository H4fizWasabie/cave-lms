package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestANewDashboardAsksOnlyForANameAndItsPage(t *testing.T) {
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

func TestThePreviewOffersGeneralAndOnlyTheTemplatesThatFit(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("general", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get(path)
	for _, want := range []string{`href="` + path + `?template=finance"`, `href="` + path + `?template=sales"`, `name="template" value="general"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	for _, unfit := range []string{"inventory", "hr", "roster"} {
		if strings.Contains(body, "?template="+unfit) {
			t.Fatalf("%s is offered on data it does not fit:\n%s", unfit, body)
		}
	}

	// Switching to Finance shows its blanks filled with Cave's guesses and the blocks it would build.
	res, body = ada.get(path + "?template=finance")
	for _, want := range []string{`name="template" value="finance"`, `<option value="Tarikh" selected>Tarikh</option>`, "Money in", "Latest transactions",
		`href="` + path + `?template=general"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	// A Template the data does not fit is not offered by URL either: General stays picked.
	res, body = ada.get(path + "?template=roster")
	expectStatus(t, res, body, http.StatusOK, `name="template" value="general"`)

	ada.confirmWork(path, url.Values{"template": {"finance"}, "date": {"Tarikh"}, "amount": {"Jumlah"}, "type": {"Jenis"}})
	if _, index := ada.get(path + "/view/"); !strings.Contains(index, `"label": "Money in"`) {
		t.Fatalf("confirming with Finance did not open Finance:\n%s", index)
	}
}

func TestConfirmingATemplateOnDataWithoutItsAmountLeavesOutTheBlocksThatNeedIt(t *testing.T) {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	noAmount := testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori"}, []any{day, "Beras", "Dapur"}, []any{day, "Gula", "Dapur"})
	for tpl, count := range map[string]string{"finance": "Transactions", "sales": "Sales"} {
		t.Run(tpl, func(t *testing.T) {
			_, ada := labLearner(t)
			path := ada.createWork("general", "Kedai")
			res, body := ada.postFile(path+"/dataset", "senarai.xlsx", noAmount)
			expectStatus(t, res, body, http.StatusSeeOther, "")
			res, body = ada.post(path+"/dataset/confirm", url.Values{"template": {tpl}, "date": {"Tarikh"}, "category": {"Kategori"}})
			expectStatus(t, res, body, http.StatusSeeOther, "")
			_, index := ada.get(path + "/view/")
			if !strings.Contains(index, `"label": "`+count+`"`) || strings.Contains(index, `"agg": "sum"`) {
				t.Fatalf("%s on data with no Amount:\n%s", tpl, index)
			}
		})
	}
}

func TestALearnerStartsADashboardFromEachTemplatesSampleWithItsTemplateChosen(t *testing.T) {
	_, ada := labLearner(t)
	for _, key := range []string{"finance", "inventory", "hr", "sales", "roster"} {
		res, body := ada.post("/lab/dashboards", url.Values{"name": {"Cuba " + key}, "sample": {key}})
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(res.Header.Get("Location"))
		for _, want := range []string{key + "-sample.xlsx", `name="template" value="` + key + `"`} {
			expectStatus(t, res, body, http.StatusOK, want)
		}
	}

	// An unknown Sample, or General's, makes no Dashboard.
	for _, key := range []string{"astrology", "general"} {
		res, body := ada.post("/lab/dashboards", url.Values{"name": {"Tiada"}, "sample": {key}})
		expectStatus(t, res, body, http.StatusNotFound, "")
	}
}

func TestConfirmingTheFinanceSampleBuildsFinanceAndCountsAgainstTheStorageAllowance(t *testing.T) {
	_, ada, _ := storageSite(t, "1024")
	res, body := ada.post("/lab/dashboards", url.Values{"name": {"Kewangan"}, "sample": {"finance"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	path := res.Header.Get("Location")
	ada.confirmWork(path, url.Values{"template": {"finance"}, "date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Kategori"}, "type": {"Jenis"}})
	if _, index := ada.get(path + "/view/"); !strings.Contains(index, `"label": "Money in"`) {
		t.Fatalf("confirming the Finance Sample did not open Finance:\n%s", index)
	}
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, " of 1 GB")
	if strings.Contains(body, "0 KB of 1 GB") {
		t.Fatalf("the Sample's Dataset does not count against the Storage allowance:\n%s", body)
	}
}
