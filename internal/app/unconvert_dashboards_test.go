package app_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// pageRows is how many page records the Dashboard at path has.
func (s *site) pageRows(path string) int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM dashboard_pages WHERE dashboard_id = $1`, idOf(path)).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// convertToPage gives the built Dashboard at path page as its current page, record and stored file, as the conversion
// to pages on 2026-10-27 did (#375); the Lab takes no page for a built Dashboard now.
func (s *site) convertToPage(path, page string) {
	s.t.Helper()
	key := "page-converted-" + idOf(path)
	if err := os.WriteFile(filepath.Join(s.storageDir, key), []byte(page), 0o600); err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO dashboard_pages (dashboard_id, storage_key, size, current) VALUES ($1, $2, $3, true)`,
		idOf(path), key, len(page)); err != nil {
		s.t.Fatal(err)
	}
}

// The Dashboards converted to pages on 2026-10-27 (#389) are built again after migration 00125: one with only its
// converted page loses it, its file goes from the store, and its link draws its Theme and blocks again. One whose
// owner uploaded a page of their own, and a pasted one, keep their pages.
func TestConvertedDashboardsWithOnlyTheirConvertedPageAreBuiltAgain(t *testing.T) {
	_, ada := labLearner(t)
	converted := ada.readyDashboard("Kewangan")
	link := ada.deploy(converted)
	theme, blocks := ada.drawn(link)
	data := ada.dataOf(converted + "/view/")
	ada.convertToPage(converted, versionOf(1))

	owned := ada.readyDashboard("Kedai")
	ada.convertToPage(owned, versionOf(1))
	ada.uploadPage(owned, versionOf(2))
	pasted := ada.createPage("Jualan", myPage)
	if _, page := ada.get(link); page != versionOf(1) {
		t.Fatalf("before the migration the link shows %.200q, want the converted page", page)
	}
	files := ada.storedFiles()

	if err := app.MigrateDownTo(context.Background(), ada.db, 124); err != nil {
		t.Fatal(err)
	}
	ada.restart()

	if gotTheme, gotBlocks := ada.drawn(link); gotTheme != theme || !slices.Equal(gotBlocks, blocks) {
		t.Fatalf("after the migration the link draws %q %q, want %q %q", gotTheme, gotBlocks, theme, blocks)
	}
	if got := ada.dataOf(converted + "/view/"); got != data {
		t.Fatalf("after the migration the Dashboard fetches %s, want %s", got, data)
	}
	for path, want := range map[string]int{converted: 0, owned: 2, pasted: 1} {
		if n := ada.pageRows(path); n != want {
			t.Errorf("%s has %d pages after the migration, want %d", path, n, want)
		}
	}
	if n := ada.storedFiles(); n != files-1 {
		t.Fatalf("%d stored files after the migration, want %d: the converted page gone", n, files-1)
	}
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, ada.storedKB())
}
