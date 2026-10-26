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
	expectStatus(t, res, body, http.StatusForbidden, "The Lab is locked")
	res, body = ada.post("/lab/dashboards", url.Values{"name": {"Staf"}, "html": {myPage}})
	expectStatus(t, res, body, http.StatusForbidden, "The Lab is locked")
}

// buildFromSample creates a Dashboard on a Template, uploads its Sample and returns the Dashboard's path and the
// upload's preview, which shows Cave's guesses for the blanks.
func buildFromSample(t *testing.T, s *site, key string) (path, preview string) {
	t.Helper()
	path = s.createWork(key, "Mine")
	res, sample := s.get("/lab/samples/" + key + ".xlsx")
	expectStatus(t, res, sample, http.StatusOK, "")
	res, body := s.postFile(path+"/dataset", key+".xlsx", []byte(sample))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, preview = s.get(path)
	expectStatus(t, res, preview, http.StatusOK, "")
	return path, preview
}

func TestLearnerBuildsInventoryAndHRFromTheirSamplesWithGuesses(t *testing.T) {
	_, ada := labLearner(t)
	for key, want := range map[string]struct {
		guesses []string
		confirm url.Values
		blocks  []string
	}{
		"inventory": {[]string{`<option value="Nama Item" selected>`, `<option value="Kuantiti" selected>`, `<option value="Paras Pesanan Semula" selected>`, `<option value="Harga Seunit" selected>`},
			url.Values{"item": {"Nama Item"}, "quantity": {"Kuantiti"}, "reorder": {"Paras Pesanan Semula"}, "category": {"Kategori"}, "price": {"Harga Seunit"}},
			[]string{"Total items", "Stock value", "Low stock", "Stock by category", "Running low"}},
		"hr": {[]string{`<option value="Nama" selected>`, `<option value="Jabatan" selected>`, `<option value="Tarikh Mula" selected>`, `<option value="Jantina" selected>`},
			url.Values{"name": {"Nama"}, "department": {"Jabatan"}, "joined": {"Tarikh Mula"}, "status": {"Status"}, "gender": {"Jantina"}},
			[]string{"Headcount", "Active", "Headcount by department", "Joins per month", "Status split"}},
	} {
		path, preview := buildFromSample(t, ada, key)
		for _, g := range want.guesses {
			if !strings.Contains(preview, g) {
				t.Errorf("%s upload preview lacks the guess %s", key, g)
			}
		}
		ada.confirmWork(path, want.confirm)
		res, body := ada.get(path + "/view/")
		for _, b := range want.blocks {
			expectStatus(t, res, body, http.StatusOK, `"`+b+`"`)
		}
	}
}

func TestSkippingUnitPriceHidesStockValueOnly(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := buildFromSample(t, ada, "inventory")
	ada.confirmWork(path, url.Values{"item": {"Nama Item"}, "quantity": {"Kuantiti"}, "reorder": {"Paras Pesanan Semula"}, "category": {"Kategori"}, "price": {""}})
	res, body := ada.get(path + "/view/")
	expectStatus(t, res, body, http.StatusOK, `"Low stock"`)
	if strings.Contains(body, `"Stock value"`) {
		t.Fatalf("Stock value shown without a Unit price column")
	}
}

func TestGeneralNeedsNoColumnChoices(t *testing.T) {
	_, ada := labLearner(t)
	path, preview := buildFromSample(t, ada, "general")
	if strings.Contains(preview, `class="field map-row"`) {
		t.Errorf("General's upload preview asks for columns:\n%s", preview)
	}
	ada.confirmWork(path, nil)
	res, body := ada.get(path + "/view/")
	for _, want := range []string{`"Jualan by Cawangan"`, `"Jualan by month"`, `"Latest rows"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
