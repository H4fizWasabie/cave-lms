package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

func TestACaveHostDataAddressThatAnswersNotFoundIsStillReadableByASandboxedPage(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	link := onlineSales(t, ada, "jualan-ada")
	visitor := ada.second()

	for _, file := range []string{"sheets/Staff.json", "sheets/Nothing.json"} { // unticked, and not in the Spreadsheet
		res, _ := visitor.get(link + file)
		if res.StatusCode != http.StatusNotFound || res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s = %d with Access-Control-Allow-Origin %q, want 404 with *", file, res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
		}
	}
	ada.untickSheet("jualan-ada", "Sales") // no ticked sheet is left to answer data.json
	res, _ := visitor.get(link + "data.json")
	if res.StatusCode != http.StatusNotFound || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("data.json = %d with Access-Control-Allow-Origin %q, want 404 with *", res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestATakenDownCaveHostDeploymentIsAPageNotADashboard(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin, ada := labLearner(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.assignCaveHost("ada@example.com", 30)
	link := onlineSales(t, ada, "jualan-ada")

	_, list := admin.get("/admin/deployments")
	res, body := admin.post(takedownOf(t, list, "jualan-ada"), url.Values{"reason": {"A complaint."}})
	expectRedirect(t, res, body, "/admin/deployments")

	res, page := ada.second().get(link)
	expectStatus(t, res, page, http.StatusOK, "This CaveHost page is asleep")
	if strings.Contains(page, "Dashboard") || !strings.Contains(page, "Cave took it offline early") {
		t.Errorf("the taken-down CaveHost page calls itself a Dashboard, or lost its reason: %s", page)
	}
	res, page = ada.second().getAsking(link, "ms")
	expectStatus(t, res, page, http.StatusOK, "Halaman CaveHost ini sedang tidur")
	if strings.Contains(page, "Dashboard") {
		t.Errorf("the taken-down CaveHost page calls itself a Dashboard in Malay: %s", page)
	}
}

func TestASheetWhoseLastSyncFailedIsNotSaidToBeReady(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	onlineSales(t, ada, "jualan-ada")
	ada.tickSheet("jualan-ada", "Staff")
	ada.confirmSheet("jualan-ada", nil)
	_, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"Sheet Sales is ready", "Sheet Staff is ready"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the page lacks %q before any failure: %s", want, body)
		}
	}

	// Staff loses a column it had when confirmed; Sales still reads.
	google.swap(testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}, {"Ink", 5}}},
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Pay"}, {"Ali", 3000}}},
		testutil.Sheet{Name: "Cost Q1", Rows: [][]any{{"Item", "Cost"}, {"Pen", 1.5}}},
	))
	ada.syncNow("jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Sheet Sales is ready") || strings.Contains(body, "Sheet Staff is ready") {
		t.Errorf("after Staff alone failed, the page does not say only Sales is ready: %s", body)
	}
	if !strings.Contains(body, "Last Sync of Staff failed") {
		t.Errorf("the page does not say the last Sync of Staff failed: %s", body)
	}
	if strings.Contains(body, "Last Sync of Sales failed") {
		t.Errorf("the page says the last Sync of Sales failed: %s", body)
	}
	_, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	if !strings.Contains(body, "Penyegerakan terakhir Staff gagal") {
		t.Errorf("the Malay page does not say the last Sync of Staff failed: %s", body)
	}

	// A Spreadsheet that does not open fails every sheet it Syncs.
	google.answerWith(http.StatusForbidden)
	ada.syncNow("jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	if strings.Contains(body, "is ready") || !strings.Contains(body, "Last Sync of Sales failed") || !strings.Contains(body, "Last Sync of Staff failed") {
		t.Errorf("after the whole Spreadsheet failed, the page does not say so of each sheet: %s", body)
	}

	// A Sync that works clears it.
	google.answerWith(0)
	google.swap(threeSheets(t))
	ada.syncNow("jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Sheet Staff is ready") || strings.Contains(body, "Sync of Staff failed") {
		t.Errorf("a Sync that worked leaves the failure on the sheet: %s", body)
	}
}

func TestCaveHostWordsItsOwnFormsNotTheLabs(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	for lang, want := range map[string]string{"en": "ticks the sheet in your link, or the first one", "ms": "menandakan helaian dalam pautan anda, atau yang pertama"} {
		res, body := ada.getAsking("/cavehost", lang)
		expectStatus(t, res, body, http.StatusOK, want)
		if strings.Contains(body, "first one ticked") || strings.Contains(body, "helaian pertama ditandakan") {
			t.Errorf("the link form still says it starts with the first sheet ticked: %s", body)
		}
	}

	ada.linkSheet(sheetLink, "jualan-ada")
	for lang, want := range map[string]string{"en": "type its row number and press Read again", "ms": "taip nombor barisnya dan tekan Baca semula"} {
		res, body := ada.getAsking("/cavehost/jualan-ada", lang)
		expectStatus(t, res, body, http.StatusOK, want)
		if strings.Contains(body, "upload the file again") || strings.Contains(body, "muat naik fail sekali lagi") {
			t.Errorf("the Clean-up preview tells a CaveHost Learner to upload a file: %s", body)
		}
	}

	ada.confirmSheet("jualan-ada", nil)
	for _, lang := range []string{"en", "ms"} {
		res, body := ada.getAsking("/cavehost/jualan-ada", lang)
		expectStatus(t, res, body, http.StatusOK, "5 MB")
		if strings.Contains(body, "Storage allowance") || strings.Contains(body, "had storan") {
			t.Errorf("a CaveHost page counts against a Storage allowance it does not have (%s): %s", lang, body)
		}
	}
}

func TestThePricingSubheadingNamesCaveHostWithTheProPlan(t *testing.T) {
	t.Parallel()
	visitor := promoSite(t).second()
	for lang, want := range map[string]string{
		"en": "CaveText Pro, the Lab Plan, or both plus CaveHost in the Pro Plan.",
		"ms": "CaveText Pro, Pelan Lab, atau kedua-duanya serta CaveHost dalam Pelan Pro.",
	} {
		res, body := visitor.getAsking("/pricing", lang)
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body := visitor.get(caveTextOrigin + "/pricing")
	expectStatus(t, res, body, http.StatusOK, "CaveText Pro, or CaveText Pro with the Lab and CaveHost in the Pro Plan.")
}
