package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// confirmSheet confirms the Clean-up preview of the Deployment called name.
func (s *site) confirmSheet(name string, form url.Values) {
	s.t.Helper()
	res, body := s.post("/cavehost/"+name+"/confirm", form)
	expectRedirect(s.t, res, body, "/cavehost/"+name)
}

// putPageOnline pastes html as the page of the Deployment called name.
func (s *site) putPageOnline(name, html string) {
	s.t.Helper()
	res, body := s.post("/cavehost/"+name+"/page", url.Values{"html": {html}})
	expectRedirect(s.t, res, body, "/cavehost/"+name)
}

// caveHostLink is the link to share that the page of the Deployment called name shows.
func (s *site) caveHostLink(name string) string {
	s.t.Helper()
	res, body := s.get("/cavehost/" + name)
	expectStatus(s.t, res, body, http.StatusOK, "")
	m := deploymentLink.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("the Deployment's page shows no link: %s", body)
	}
	return m[1]
}

func TestAConfirmedSheetAndAPageGoOnlineAtTheDeploymentsLinkWithTheSheetsRowsAsDataJSON(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	ada.confirmSheet("jualan-ada", nil)

	link := ada.caveHostLink("jualan-ada")
	if want := pagesOrigin + "/ada-example-com/jualan-ada-"; !strings.HasPrefix(link, want) {
		t.Fatalf("the link is %q, want it to start %q: a Link key follows the name (ADR 0027)", link, want)
	}
	visitor := ada.second()
	if res, _ := visitor.get(link); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET the link before a page is put online = %d, want 404", res.StatusCode)
	}

	const page = `<!doctype html><title>Jualan</title><script>fetch("./data.json")</script>`
	ada.putPageOnline("jualan-ada", page)
	res, body = visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, page)
	if csp := res.Header.Get("Content-Security-Policy"); csp != pageCSP {
		t.Errorf("the page's Content-Security-Policy is %q, want the Lab's page CSP", csp)
	}
	if body != page {
		t.Errorf("the page is served as %q, want it as uploaded (no cleaning, ADR 0030)", body)
	}

	// data.json is the Lab Dataset's rows, each with its Part date.
	res, body = visitor.get(link + "data.json")
	expectStatus(t, res, body, http.StatusOK, "")
	if res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("data.json headers = %v, want CORS for the sandboxed page and JSON", res.Header)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("data.json is not JSON: %v\n%s", err, body)
	}
	day := partDate()
	want := []map[string]any{{"Item": "Pen", "Qty": 3.0, "Part date": day}, {"Item": "Ink", "Qty": 5.0, "Part date": day}, {"Item": "Pad", "Qty": 7.0, "Part date": day}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("data.json = %v, want %v", rows, want)
	}

	// The data and the page are in the local Disk store under a CaveHost prefix, not R2; the fetched Spreadsheet is not kept.
	files, _ := filepath.Glob(filepath.Join(ada.storageDir, "cavehost-*"))
	if len(files) != 2 {
		t.Errorf("the store holds %v, want the rows and the page under the cavehost- prefix", files)
	}
}
