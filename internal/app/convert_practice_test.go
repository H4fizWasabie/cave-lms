package app_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var dashboardSettings = regexp.MustCompile(`const dashboard = (\{[\s\S]*?\});\n`)

// drawn is what a Dashboard page draws: its Theme and its blocks' kinds and labels, in order.
func (s *site) drawn(page string) (theme string, blocks []string) {
	s.t.Helper()
	res, body := s.get(page)
	expectStatus(s.t, res, body, http.StatusOK, "")
	m := dashboardSettings.FindStringSubmatch(body)
	if m == nil {
		s.t.Fatalf("%s holds no dashboard settings: %s", page, body)
	}
	var settings struct {
		Theme  string `json:"theme"`
		Blocks []struct{ Kind, Label string }
	}
	if err := json.Unmarshal([]byte(m[1]), &settings); err != nil {
		s.t.Fatal(err)
	}
	for _, b := range settings.Blocks {
		blocks = append(blocks, b.Kind+": "+b.Label)
	}
	return settings.Theme, blocks
}

// dashboardState is every Dashboard's and Dataset's row as text, to see that a restart changed nothing.
func (s *site) dashboardState() string {
	s.t.Helper()
	var state string
	err := s.db.QueryRow(`SELECT (SELECT string_agg(concat_ws('|', id, kind, theme, config, dataset_key, dataset_size), E'\n' ORDER BY id) FROM dashboards)
		|| E'\n' || COALESCE((SELECT string_agg(concat_ws('|', id, dashboard_id, storage_key, file_name, saved_size, row_count, state), E'\n' ORDER BY id) FROM datasets), '')`).Scan(&state)
	if err != nil {
		s.t.Fatal(err)
	}
	return state
}

// asPractice turns the Dashboard at path back into a Practice Dashboard as one was stored before ADR 0015: its
// Dataset on its own row, out of datasets, its kind practice and no blocks of its own. No route makes one any more.
func (s *site) asPractice(path string) {
	s.t.Helper()
	id := path[strings.LastIndex(path, "/")+1:]
	if _, err := s.db.Exec(`WITH cur AS (DELETE FROM datasets WHERE dashboard_id = $1 RETURNING storage_key, file_name, saved_size)
		UPDATE dashboards d SET kind = 'practice', config = NULL, dataset_key = cur.storage_key, dataset_file = cur.file_name, dataset_size = cur.saved_size
		FROM cur WHERE d.id = $1`, id); err != nil {
		s.t.Fatal(err)
	}
	// Its Dataset never was in datasets, so nothing queued it for deletion.
	if _, err := s.db.Exec(`DELETE FROM stored_file_deletions WHERE storage_key = (SELECT dataset_key FROM dashboards WHERE id = $1)`, id); err != nil {
		s.t.Fatal(err)
	}
}

// Practice Dashboards made before ADR 0015 become Dashboards at the next start: blocks built by the General rules,
// as their Templates are gone (ADR 0019), their Dataset as the current one, their Theme, and their Deployment at
// the same link drawing the same data. Starting again changes nothing.
func TestPracticeDashboardsBecomeDashboardsAtStartupKeepingBlocksDatasetThemeAndDeployment(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	empty := ada.createWork("Kosong")
	general := ada.createWork("Umum")
	ada.confirmFile(general, "finance-sample.xlsx", ada.financeSample())
	_, generalBlocks := ada.drawn(general + "/view/")
	if _, err := ada.db.Exec(`UPDATE dashboards SET theme = 'midnight' WHERE id = $1`, path[strings.LastIndex(path, "/")+1:]); err != nil {
		t.Fatal(err)
	}
	link := ada.deploy(path)
	visitor := ada.second()
	visitor.client.Jar = nil
	theme, blocks := visitor.drawn(link)
	if theme != "midnight" || len(blocks) == 0 {
		t.Fatalf("before: the Deployment draws theme %q, blocks %q", theme, blocks)
	}
	dataAt := visitor.dataOf(link)
	_, data := visitor.get(link + dataAt)
	for _, p := range []string{path, empty, general} {
		ada.asPractice(p)
	}
	if _, err := ada.db.Exec(`UPDATE dashboards SET kind = 'practice' WHERE id = $1`, empty[strings.LastIndex(empty, "/")+1:]); err != nil {
		t.Fatal(err)
	}
	files := ada.storedFiles()

	ada.restart()

	var practice, dashboards, deployments int
	if err := ada.db.QueryRow(`SELECT count(*) FILTER (WHERE kind = 'practice'), count(*), (SELECT count(*) FROM deployments) FROM dashboards`).
		Scan(&practice, &dashboards, &deployments); err != nil {
		t.Fatal(err)
	}
	if practice != 0 || dashboards != 3 || deployments != 1 {
		t.Fatalf("after: %d Practice of %d Dashboards, %d Deployments; want 0 of 3, 1", practice, dashboards, deployments)
	}
	gotTheme, gotBlocks := visitor.drawn(link)
	if gotTheme != "midnight" || !slices.Equal(gotBlocks, blocks) {
		t.Fatalf("after: the Deployment draws theme %q, blocks\n%q\nwant midnight,\n%q", gotTheme, gotBlocks, blocks)
	}
	res, got := visitor.get(link + visitor.dataOf(link))
	expectStatus(t, res, got, http.StatusOK, "")
	if got != data {
		t.Fatalf("after: the Deployment's data is\n%.300s\nwant\n%.300s", got, data)
	}
	if n := ada.storedFiles(); n != files {
		t.Fatalf("%d stored files after converting, want the same %d Datasets", n, files)
	}
	// The blocks are the Dashboard's own now, and its Dataset is current: nothing to re-upload.
	if _, got := ada.drawn(general + "/view/"); len(generalBlocks) == 0 || !slices.Equal(got, generalBlocks) {
		t.Fatalf("after: General draws\n%q\nwant\n%q", got, generalBlocks)
	}
	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "finance-sample.xlsx")
	if theme, _ := ada.drawn(path + "/view/"); theme != "midnight" {
		t.Fatalf("after: the Dashboard draws theme %q, want midnight", theme)
	}
	res, body = ada.get(empty)
	expectStatus(t, res, body, http.StatusOK, "")

	state := ada.dashboardState()
	ada.restart()
	if again := ada.dashboardState(); again != state {
		t.Fatalf("a second start changed the Dashboards:\n%s\nwas\n%s", again, state)
	}
}
