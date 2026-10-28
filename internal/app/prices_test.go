package app_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// priceForm is the Prices form with every product's price, Normal price and payment link, as the page starts.
func priceForm() url.Values {
	return url.Values{
		"cavetext-pro_price": {"25"}, "cavetext-pro_normal": {""}, "cavetext-pro_link": {""},
		"lab-plan_price": {"89"}, "lab-plan_normal": {""}, "lab-plan_link": {""},
		"pro-plan_price": {"99"}, "pro-plan_normal": {"119"}, "pro-plan_link": {""},
	}
}

// savePrices posts the Prices form as the logged-in Admin, the form as the page starts with changes set over it.
func (s *site) savePrices(changes map[string]string) (*http.Response, string) {
	s.t.Helper()
	form := priceForm()
	for k, v := range changes {
		form.Set(k, v)
	}
	return s.post("/admin/prices", form)
}

func TestAnyAdminSetsThePricesNormalPricesAndPaymentLinksOfTheThreeProducts(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	res, body := s.get("/admin/prices")
	expectStatus(t, res, body, http.StatusOK, "<h1>Admin: Prices</h1>")
	expectAll(t, body, "CaveText Pro", "Lab Plan", "Pro Plan", `href="/admin/prices" aria-current="page"`,
		`name="cavetext-pro_price" value="25"`, `name="cavetext-pro_normal" value=""`, `name="cavetext-pro_link" value=""`,
		`name="lab-plan_price" value="89"`, `name="lab-plan_normal" value=""`, `name="lab-plan_link" value=""`,
		`name="pro-plan_price" value="99"`, `name="pro-plan_normal" value="119"`, `name="pro-plan_link" value=""`)

	res, body = s.savePrices(map[string]string{
		"cavetext-pro_price": "35", "cavetext-pro_normal": "49.90", "cavetext-pro_link": "https://bcl.my/pay/cavetext",
		"lab-plan_price": "79.50", "lab-plan_link": "https://bcl.my/pay/lab",
		"pro-plan_normal": "", "pro-plan_link": "https://bcl.my/pay/pro"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = s.get(res.Header.Get("Location"))
	expectStatus(t, res, body, http.StatusOK, "Saved.")
	expectAll(t, body,
		`name="cavetext-pro_price" value="35"`, `name="cavetext-pro_normal" value="49.90"`, `name="cavetext-pro_link" value="https://bcl.my/pay/cavetext"`,
		`name="lab-plan_price" value="79.50"`, `name="lab-plan_normal" value=""`, `name="lab-plan_link" value="https://bcl.my/pay/lab"`,
		`name="pro-plan_price" value="99"`, `name="pro-plan_normal" value=""`, `name="pro-plan_link" value="https://bcl.my/pay/pro"`)
}

func TestThePricesPageRefusesANormalPriceNotAboveThePriceAndOtherMistakes(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	for _, c := range []struct {
		changes map[string]string
		want    string
	}{
		{map[string]string{"pro-plan_normal": "99"}, "Pro Plan: Enter a Normal price above the price, or leave it empty."},
		{map[string]string{"cavetext-pro_normal": "20"}, "CaveText Pro: Enter a Normal price above the price"},
		{map[string]string{"lab-plan_normal": "abc"}, "Lab Plan: Enter the Normal price in RM"},
		{map[string]string{"lab-plan_price": "abc"}, "Lab Plan: Enter the price in RM, like 25 or 25.50."},
		{map[string]string{"lab-plan_price": ""}, "Lab Plan: Enter the price in RM"},
		{map[string]string{"cavetext-pro_price": "100000"}, "CaveText Pro: Enter the price in RM"},
		{map[string]string{"cavetext-pro_price": "0"}, "CaveText Pro: Enter a price more than 0"},
		{map[string]string{"pro-plan_link": "javascript:alert(1)"}, "Pro Plan: Enter the payment link"},
		{map[string]string{"lab-plan_link": "ftp://bcl.my/pay"}, "http:// or https://"},
		{map[string]string{"lab-plan_link": "https://bcl.my/pay/same", "pro-plan_link": "https://bcl.my/pay/same"},
			"Pro Plan: Enter a payment link no other product uses"}, // the webhook would give the first product it matches
		{map[string]string{"lab-plan_link": "https://bcl.my/pay/same", "pro-plan_link": "https://BCL.my/pay/same/"},
			"Pro Plan: Enter a payment link no other product uses"}, // the webhook ignores case and a trailing slash too
	} {
		res, body := s.savePrices(c.changes)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		for name, value := range c.changes {
			expectStatus(t, res, body, http.StatusUnprocessableEntity, `name="`+name+`" value="`+value+`"`) // kept to correct
		}
	}
	_, body := s.get("/admin/prices")
	expectAll(t, body, `name="pro-plan_normal" value="119"`, `name="cavetext-pro_price" value="25"`, `name="lab-plan_link" value=""`)
}

func TestOnlyAnAdminSetsPricesAndNeverCrossOrigin(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	visitor := s.second()
	cheap := priceForm()
	cheap.Set("pro-plan_price", "1")
	res, body := visitor.get("/admin/prices")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/admin/prices")
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	res, body = visitor.post("/admin/prices", cheap)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post("/admin/prices", cheap)
	expectStatus(t, res, body, http.StatusForbidden, "Only the Admin can open this page")
	for header, value := range map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"} {
		req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/admin/prices", strings.NewReader(cheap.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(header, value)
		res, body := s.do(req)
		expectStatus(t, res, body, http.StatusForbidden, "cross-origin request refused")
	}
	_, body = s.get("/admin/prices")
	expectAll(t, body, `name="pro-plan_price" value="99"`)
}

func TestThePricesPageIsInMalayAndEnglish(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	s.switchLanguage("ms", "/")
	_, body := s.get("/admin/prices")
	expectLanguage(t, body, "ms", "Pentadbir: Harga")
	expectAll(t, body, "Pelan Lab", "Pelan Pro", "Harga biasa")
	res, body := s.savePrices(map[string]string{"pro-plan_normal": "90"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Pelan Pro: Masukkan harga biasa yang melebihi harga")
	s.switchLanguage("en", "/")
	_, body = s.get("/admin")
	expectAll(t, body, `href="/admin/prices"`, "Prices")
}

func TestTheMoveToPricesKeepsTodaysProPriceLinkAndProPlanMark(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	s.createPlan("Lab Year", "365", "5")
	s.createPlan("Staff", "30", "1")
	if err := app.MigrateDownTo(context.Background(), s.db, 71); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE cavetext_settings SET pro_price_sen = 3500, pro_payment_link = 'https://bcl.my/form/19548'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE plans SET pro = (name = 'Lab Year'), payment_link = 'https://bcl.my/form/old'`); err != nil {
		t.Fatal(err)
	}
	s.restart()

	_, body := s.get("/admin/prices")
	expectAll(t, body, `name="cavetext-pro_price" value="35"`, `name="cavetext-pro_normal" value=""`, `name="cavetext-pro_link" value="https://bcl.my/form/19548"`,
		`name="lab-plan_price" value="89"`, `name="lab-plan_normal" value=""`, `name="lab-plan_link" value=""`,
		`name="pro-plan_price" value="99"`, `name="pro-plan_normal" value="119"`, `name="pro-plan_link" value=""`)
	if got := s.labPlanName(); got != "Lab Year" {
		t.Fatalf("Lab Plan after the move = %q, want Lab Year", got)
	}
	_, body = s.get("/admin/plans")
	if strings.Contains(body, "bcl.my/form/old") {
		t.Errorf("a Plan still shows its own payment link: %s", body)
	}
	_, body = s.get("/cavetext")
	expectAll(t, body, "RM35", `href="https://bcl.my/form/19548"`)
}

func TestThePricesPageSavesOptionalStorageAddOnPricesWithTheThreeProducts(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	_, body := s.get("/admin/prices")
	expectAll(t, body, "Storage add-ons", `name="storage-3gb_price" value=""`, `name="storage-5gb_price" value=""`)
	expectNone(t, body, "storage-3gb_normal", "storage-3gb_link", "storage-5gb_normal", "storage-5gb_link")

	// Blank add-on prices save, and the three products keep what was posted.
	res, body := s.savePrices(map[string]string{"lab-plan_price": "79.50", "storage-3gb_price": "", "storage-5gb_price": ""})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/prices?saved")
	expectAll(t, body, `name="lab-plan_price" value="79.50"`, `name="cavetext-pro_price" value="25"`, `name="pro-plan_price" value="99"`,
		`name="storage-3gb_price" value=""`, `name="storage-5gb_price" value=""`)

	res, body = s.savePrices(map[string]string{"storage-3gb_price": "20", "storage-5gb_price": "29.90"})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = s.get("/admin/prices?saved")
	expectAll(t, body, `name="storage-3gb_price" value="20"`, `name="storage-5gb_price" value="29.90"`)

	// An add-on mistake refuses the whole form and keeps what was typed; nothing is saved.
	res, body = s.savePrices(map[string]string{"lab-plan_price": "10", "storage-5gb_price": "abc"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "&#43;5 GB: Enter the price in RM")
	expectAll(t, body, `name="storage-5gb_price" value="abc"`)
	res, body = s.savePrices(map[string]string{"storage-3gb_price": "100000"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "&#43;3 GB: Enter the price in RM")
	_, body = s.get("/admin/prices")
	expectAll(t, body, `name="lab-plan_price" value="89"`, `name="storage-5gb_price" value="29.90"`)

	// The three products still refuse a price of 0 and an add-on is no product a Promo can be for.
	res, body = s.savePrices(map[string]string{"lab-plan_price": "0"})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Lab Plan: Enter a price more than 0")
	res, body = s.post("/admin/promos", promoForm("EXTRA", "storage-3gb", "10", "2020-01-01", "2099-01-01"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Choose a product.")
}
