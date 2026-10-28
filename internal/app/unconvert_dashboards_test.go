package app_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

// linkAfterMigration is the link of the Deployment of the Dashboard at path after a migration down past 131 and up again,
// which gave its Learner a new random Username and so a new link.
func (s *site) linkAfterMigration(path string) string {
	s.t.Helper()
	_, body := s.get(path + "/deploy")
	m := regexp.MustCompile(`/d/learner-[a-z0-9]{6}/dash-\d+/`).FindString(body)
	if m == "" {
		s.t.Fatalf("after the migration the Deployment's link has no random Username: %s", body)
	}
	return m
}

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
	t.Parallel()
	_, ada := labLearner(t)
	converted := ada.readyDashboard("Kewangan")
	link := ada.deploy(converted)
	theme, blocks := ada.drawn(link)
	data := ada.dataOf(converted + "/view/")
	page := versionOf(1) + strings.Repeat(" ", 200<<10-len(versionOf(1))) // 200 KB exactly, so the total drops by 200
	ada.convertToPage(converted, page)

	owned := ada.readyDashboard("Kedai")
	ada.convertToPage(owned, versionOf(1))
	ada.uploadPage(owned, versionOf(2))
	pasted := ada.createPage("Jualan", myPage)
	if _, got := ada.get(link); got != page {
		t.Fatalf("before the migration the link shows %.200q, want the converted page", got)
	}
	// Migrating down past 127 drops the Workbooks table, and with it their share of the total; the restart deletes
	// their files from the store, as the migration's trigger queued them.
	ada.forgetWorkbooks()
	ada.restart()
	files := ada.storedFiles()
	_, body := ada.get("/lab")
	total := storageTotal.FindStringSubmatch(body)
	if total == nil {
		t.Fatalf("/lab shows no storage total in KB:\n%s", body)
	}
	before, _ := strconv.Atoi(total[1])

	if err := app.MigrateDownTo(context.Background(), ada.db, 124); err != nil {
		t.Fatal(err)
	}
	ada.restart()
	link = ada.linkAfterMigration(converted)

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
	expectStatus(t, res, body, http.StatusOK, fmt.Sprintf("%d KB of 1 GB", before-200))
}

// storageTotal is the Storage total /lab shows, in KB.
var storageTotal = regexp.MustCompile(`(\d+) KB of 1 GB`)
