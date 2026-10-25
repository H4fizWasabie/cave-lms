package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

var labKeys = []string{"finance", "inventory", "hr", "general"}

// expectDemoPreviews checks the Lab page previews every Template on its Sample, and that each preview is served.
func expectDemoPreviews(t *testing.T, s *site) {
	t.Helper()
	res, body := s.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "Templates, shown with their Samples")
	for _, key := range labKeys {
		expectStatus(t, res, body, http.StatusOK, `src="/lab/demo/`+key+`/"`)
		expectStatus(t, res, body, http.StatusOK, `href="/lab/demo/`+key+`/"`)
	}
	for key, row := range map[string]string{"finance": "Sewa pejabat", "inventory": "Kertas A4 (rim)", "hr": "Aiman", "general": "Nasi lemak"} {
		res, body := s.get("/lab/demo/" + key + "/")
		expectStatus(t, res, body, http.StatusOK, `<script src="charts.js"></script>`)
		res, body = s.get("/lab/demo/" + key + "/data.json")
		expectStatus(t, res, body, http.StatusOK, row)
		res, body = s.get("/lab/demo/" + key + "/charts.js")
		expectStatus(t, res, body, http.StatusOK, "drawDashboard")
	}
}

func TestVisitorSeesEveryTemplatePreviewedButCannotBuild(t *testing.T) {
	s := newSite(t)
	expectDemoPreviews(t, s)
	res, _ := s.get("/lab/new")
	if res.StatusCode == http.StatusOK {
		t.Fatalf("a Visitor opened New Dashboard")
	}
	res, _ = s.post("/lab/dashboards", url.Values{"template": {"inventory"}, "name": {"Stok"}})
	if loc := res.Header.Get("Location"); strings.HasPrefix(loc, "/lab/dashboards/") {
		t.Fatalf("a Visitor created a Dashboard: %s", loc)
	}
}

func TestLearnerWithoutAccessSeesEveryTemplatePreviewedButCannotBuild(t *testing.T) {
	_, ada, _, _ := labSetup(t)
	expectDemoPreviews(t, ada)
	res, body := ada.get("/lab/new")
	expectStatus(t, res, body, http.StatusForbidden, "The Lab is locked")
	res, body = ada.post("/lab/dashboards", url.Values{"template": {"hr"}, "name": {"Staf"}})
	expectStatus(t, res, body, http.StatusForbidden, "The Lab is locked")
}

func TestUnknownDemoIsNotFound(t *testing.T) {
	s := newSite(t)
	res, body := s.get("/lab/demo/payroll/")
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestTemplatePickerShowsBlanksAndSampleColumns(t *testing.T) {
	_, ada := labLearner(t)
	res, body := ada.get("/lab/new?t=inventory")
	for _, want := range []string{`href="/lab/samples/inventory.xlsx"`, "Reorder level", "Unit price", "<code>Nama Item</code>", "22 rows", `src="/lab/demo/hr/"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body = ada.get("/lab/new?t=general")
	expectStatus(t, res, body, http.StatusOK, "No blanks")
}

// buildFromSample creates a Dashboard from a Template, uploads its Sample and returns the Dashboard's path
// and its Columns page.
func buildFromSample(t *testing.T, s *site, key string) (path, columns string) {
	t.Helper()
	path = s.createDashboard(key, "Mine")
	res, sample := s.get("/lab/samples/" + key + ".xlsx")
	expectStatus(t, res, sample, http.StatusOK, "")
	res, body := s.postFile(path+"/dataset", key+".xlsx", []byte(sample))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, columns = s.get(path + "/columns")
	expectStatus(t, res, columns, http.StatusOK, "")
	return path, columns
}

func TestLearnerBuildsInventoryAndHRFromTheirSamplesWithGuesses(t *testing.T) {
	_, ada := labLearner(t)
	for key, want := range map[string]struct{ guesses, blocks []string }{
		"inventory": {[]string{`<option value="Nama Item" selected>`, `<option value="Kuantiti" selected>`, `<option value="Paras Pesanan Semula" selected>`, `<option value="Harga Seunit" selected>`},
			[]string{"Total items", "Stock value", "Low stock", "Stock by category", "Running low"}},
		"hr": {[]string{`<option value="Nama" selected>`, `<option value="Jabatan" selected>`, `<option value="Tarikh Mula" selected>`, `<option value="Jantina" selected>`},
			[]string{"Headcount", "Active", "Headcount by department", "Joins per month", "Status split"}},
	} {
		path, columns := buildFromSample(t, ada, key)
		for _, g := range want.guesses {
			if !strings.Contains(columns, g) {
				t.Errorf("%s Columns page lacks the guess %s", key, g)
			}
		}
		res, body := ada.get(path + "/view/")
		for _, b := range want.blocks {
			expectStatus(t, res, body, http.StatusOK, `"`+b+`"`)
		}
	}
}

func TestSkippingUnitPriceHidesStockValueOnly(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := buildFromSample(t, ada, "inventory")
	res, body := ada.post(path+"/columns", url.Values{"item": {"Nama Item"}, "quantity": {"Kuantiti"}, "reorder": {"Paras Pesanan Semula"}, "category": {"Kategori"}, "price": {""}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path + "/view/")
	expectStatus(t, res, body, http.StatusOK, `"Low stock"`)
	if strings.Contains(body, `"Stock value"`) {
		t.Fatalf("Stock value shown without a Unit price column")
	}
}

func TestGeneralNeedsNoColumnChoices(t *testing.T) {
	_, ada := labLearner(t)
	path, columns := buildFromSample(t, ada, "general")
	for _, want := range []string{"General has no blanks", "<code>Cawangan</code>", "number"} {
		if !strings.Contains(columns, want) {
			t.Errorf("General Columns page lacks %q:\n%s", want, columns)
		}
	}
	res, body := ada.get(path + "/view/")
	for _, want := range []string{`"Jualan by Cawangan"`, `"Jualan by month"`, `"Latest rows"`} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
