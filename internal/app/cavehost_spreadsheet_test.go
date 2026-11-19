package app_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// fakeGoogle stands in for Google's Sheets export (spec #536, seam 1): it serves one xlsx file for any Spreadsheet ID, or
// the answer a test sets, and remembers every address asked for.
type fakeGoogle struct {
	server     *httptest.Server
	mu         sync.Mutex
	file       []byte
	status     int           // the status to answer with; 0 for 200
	to         string        // an address to redirect to instead
	hold       chan struct{} // when set, an answer waits until it is closed
	page       string        // the sheet list page, as htmlview serves it; a 404 when empty
	listStatus int           // the status the sheet list answers with instead of the page; 0 for the page
	asked      []string
}

func newFakeGoogle(t *testing.T, file []byte) *fakeGoogle {
	g := &fakeGoogle{file: file}
	g.server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGoogle) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.asked = append(g.asked, r.URL.RequestURI())
	file, status, to, hold, page, listStatus := g.file, g.status, g.to, g.hold, g.page, g.listStatus
	g.mu.Unlock()
	if hold != nil {
		<-hold
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/htmlview") && listStatus != 0:
		w.WriteHeader(listStatus)
	case strings.HasSuffix(r.URL.Path, "/htmlview") && page == "":
		http.NotFound(w, r)
	case strings.HasSuffix(r.URL.Path, "/htmlview"):
		_, _ = w.Write([]byte(page))
	case to != "" && strings.HasPrefix(r.URL.Path, "/spreadsheets/"):
		http.Redirect(w, r, to, http.StatusSeeOther)
	case status != 0:
		w.WriteHeader(status)
	default:
		_, _ = w.Write(file)
	}
}

// swap changes the file the Spreadsheet serves from now on, as an edit does.
func (g *fakeGoogle) swap(file []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.file = file
}

// requests is every address the fake was asked for, as path and query.
func (g *fakeGoogle) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.asked...)
}

// sheetLink has no gid, so linking it asks Google for the export alone; the gid of a sheet is in gidLink.
const sheetLink = "https://docs.google.com/spreadsheets/d/1AbC-dEf_123/edit"

// caveHostLearner is Ada, with a CaveHost Plan and no Lab Plan, on a site whose Google is the fake serving file.
func caveHostLearner(t *testing.T, file []byte) (ada *site, google *fakeGoogle) {
	t.Helper()
	google = newFakeGoogle(t, file)
	admin := newSite(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	res, body := admin.assignCaveHost("ada@example.com", 30)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	return ada, google
}

// salesSheet is a Spreadsheet of one sheet, Sales, of three rows.
func salesSheet(t *testing.T) []byte {
	return testutil.Workbook(t, testutil.Sheet{Name: "Sales", Rows: [][]any{
		{"Item", "Qty"}, {"Pen", 3}, {"Ink", 5}, {"Pad", 7},
	}})
}

// linkSheet posts the link form for the Spreadsheet at link, ticking that its data is shown to anyone with the Deployment's link.
func (s *site) linkSheet(link, name string) (*http.Response, string) {
	return s.post("/cavehost", url.Values{"link": {link}, "name": {name}, "right": {"yes"}})
}

func TestLinkingASpreadsheetShowsTheCleanUpPreviewOfItsFirstSheet(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))

	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/cavehost/jualan-ada" {
		t.Fatalf("linking went to %q, want the Deployment's page", loc)
	}
	// Cave built the export address itself, from the Spreadsheet's ID, and never asked for the pasted address.
	if got := google.requests(); len(got) != 1 || got[0] != "/spreadsheets/d/1AbC-dEf_123/export?format=xlsx" {
		t.Fatalf("Google was asked for %q, want the xlsx export of the ID", got)
	}

	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "Sales")
	for _, want := range []string{"Item", "Qty", "Pen", "Ink", "Pad"} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview lacks %q: %s", want, body)
		}
	}
}
