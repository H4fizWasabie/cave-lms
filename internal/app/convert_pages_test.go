package app_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pageState is every page's row as text, with the Dashboards' and Deployments', to see that a restart changed nothing.
func (s *site) pageState() string {
	s.t.Helper()
	var state string
	err := s.db.QueryRow(`SELECT COALESCE((SELECT string_agg(concat_ws('|', id, dashboard_id, storage_key, size, uploaded_at, current), E'\n' ORDER BY id) FROM dashboard_pages), '')
		|| E'\n' || (SELECT string_agg(concat_ws('|', id, name, updated_at), E'\n' ORDER BY id) FROM dashboards)
		|| E'\n' || (SELECT string_agg(concat_ws('|', id, dashboard_id, name, taken_down_at), E'\n' ORDER BY id) FROM deployments)`).Scan(&state)
	if err != nil {
		s.t.Fatal(err)
	}
	return state
}

// Engine Dashboards with a Dataset become page Dashboards at the next start (#375): the same dashboard at the same
// link, its chart script inlined under the page policy and its rows read from data.json, stored and counted like an
// uploaded page even over the allowance. A taken-down one is converted and stays down; one without a Dataset gets
// no page. Starting again changes nothing.
func TestEngineDashboardsBecomePagesAtStartupAtTheSameLinkReadingDataJSON(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	down := ada.readyDashboard("Lama")
	downLink := ada.deploy(down)
	empty := ada.createWork("finance", "Kosong")
	visitor := ada.second()
	visitor.client.Jar = nil
	theme, blocks := visitor.drawn(link)
	_, data := visitor.get(link + visitor.dataOf(link))
	if _, err := ada.db.Exec(`UPDATE deployments SET taken_down_at = now(), takedown_reason = 'Salah' WHERE name = $1`,
		strings.Trim(strings.TrimPrefix(downLink, "/d/"), "/")); err != nil {
		t.Fatal(err)
	}
	// An allowance of nothing: the owner is over it, and still converted.
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 0`); err != nil {
		t.Fatal(err)
	}
	files := ada.storedFiles()

	ada.restart()

	res, body := visitor.get(link)
	expectStatus(t, res, body, http.StatusOK, `fetch("./data.json")`)
	if csp := res.Header.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' https://") {
		t.Fatalf("the converted page's CSP = %q, want the page policy", csp)
	}
	if strings.Contains(body, `src="charts.js"`) || !strings.Contains(body, "function drawDashboard") {
		t.Fatalf("the converted page does not inline its chart script: %.300s", body)
	}
	if gotTheme, gotBlocks := visitor.drawn(link); gotTheme != theme || !slices.Equal(gotBlocks, blocks) {
		t.Fatalf("after: the link draws theme %q, blocks\n%q\nwant %q,\n%q", gotTheme, gotBlocks, theme, blocks)
	}
	res, got := visitor.get(link + "data.json")
	expectStatus(t, res, got, http.StatusOK, "")
	if got != data {
		t.Fatalf("after: data.json is\n%.300s\nwant\n%.300s", got, data)
	}
	res, body = visitor.get(downLink)
	expectStatus(t, res, body, http.StatusOK, "This Dashboard is asleep")

	var pages, emptyPages int
	var counted int64
	if err := ada.db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE dashboard_id = $1), COALESCE(sum(size) FILTER (WHERE dashboard_id = $2), 0)
		FROM dashboard_pages WHERE current`, idOf(empty), idOf(path)).Scan(&pages, &emptyPages, &counted); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || emptyPages != 0 {
		t.Fatalf("after: %d current pages, %d on the Dashboard without a Dataset; want 2, 0", pages, emptyPages)
	}
	var key string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dashboard_pages WHERE dashboard_id = $1`, idOf(path)).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(ada.storageDir, key)); err != nil || fi.Size() != counted {
		t.Fatalf("the stored page is %v (%v), counted as %d bytes", fi, err, counted)
	}
	if n := ada.storedFiles(); n != files+2 {
		t.Fatalf("%d stored files after converting, want %d", n, files+2)
	}
	res, body = ada.get(empty)
	expectStatus(t, res, body, http.StatusOK, "Add a page")
	expectAll(t, body, `action="`+empty+`/page"`)

	state, files := ada.pageState(), ada.storedFiles()
	ada.restart()
	if again := ada.pageState(); again != state {
		t.Fatalf("a second start changed the pages:\n%s\nwas\n%s", again, state)
	}
	if n := ada.storedFiles(); n != files {
		t.Fatalf("%d stored files after a second start, want %d", n, files)
	}

	// New data reaches the live page.
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 1024`); err != nil {
		t.Fatal(err)
	}
	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", shopReport(t, "Gula", 80))
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	if _, body = visitor.get(link + "data.json"); !strings.Contains(body, `"Perkara": "Gula"`) {
		t.Fatalf("after confirming new data, data.json = %.300s", body)
	}

	// The one without a Dataset takes its first page as any upload.
	res, body = ada.post(empty+"/page", url.Values{"html": {myPage}})
	expectRedirect(t, res, body, empty)
	res, body = ada.get(empty)
	expectStatus(t, res, body, http.StatusOK, "Your page")
}

// idOf is the id at the end of a Dashboard's path.
func idOf(path string) string { return path[strings.LastIndex(path, "/")+1:] }
