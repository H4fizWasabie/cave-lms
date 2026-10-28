package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var pageRestoreAction = regexp.MustCompile(`action="(/lab/dashboards/\d+/pages/(\d+)/restore)"`)

// versionOf is a page saying which version it is.
func versionOf(n int) string {
	return fmt.Sprintf(`<!doctype html><title>Jualan</title><h1>Versi %d</h1>`, n)
}

// uploadPage pastes page as the new page of the Dashboard at path.
func (s *site) uploadPage(path, page string) {
	s.t.Helper()
	res, body := s.post(path+"/page", url.Values{"html": {page}})
	expectRedirect(s.t, res, body, path)
}

// earlierPages is the restore actions of the earlier pages on the Dashboard at path, in the order listed.
func (s *site) earlierPages(path string) []string {
	s.t.Helper()
	var out []string
	for _, m := range s.earlierPageMatches(path) {
		out = append(out, m[1])
	}
	return out
}

func (s *site) earlierPageMatches(path string) [][]string {
	s.t.Helper()
	res, body := s.get(path)
	expectStatus(s.t, res, body, http.StatusOK, "")
	return pageRestoreAction.FindAllStringSubmatch(body, -1)
}

func TestANewPageGoesLiveAtTheSameLinkAndAnEarlierOneCanBeRestored(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	link := ada.deployAs(path, "jualan-ada")
	visitor := ada.second()

	res, body := ada.get(path)
	expectAll(t, body, `action="`+path+`/page"`, `enctype="multipart/form-data"`, `type="file" name="file"`)
	if strings.Contains(body, "Earlier pages") {
		t.Fatalf("a Dashboard with one page lists earlier pages: %s", body)
	}

	ada.uploadPage(path, versionOf(2))
	if _, page := visitor.get(link); page != versionOf(2) {
		t.Fatalf("after a new page, the link shows %q", page)
	}
	res, body = ada.postFile(path+"/page", "v3.html", []byte(versionOf(3)))
	expectRedirect(t, res, body, path)
	if _, page := visitor.get(link); page != versionOf(3) {
		t.Fatalf("after a new page by file, the link shows %q", page)
	}

	res, body = ada.get(path)
	expectAll(t, body, "Earlier pages", "Uploaded ")
	earlier := ada.earlierPages(path)
	if len(earlier) != 2 {
		t.Fatalf("earlier pages = %v, want versions 2 and 1", earlier)
	}
	res, body = ada.post(earlier[1], nil) // version 1, the oldest
	expectRedirect(t, res, body, path)
	if _, page := visitor.get(link); page != versionOf(1) {
		t.Fatalf("after restoring version 1, the link shows %q", page)
	}
	if got := ada.earlierPages(path); len(got) != 2 || slices.Contains(got, earlier[1]) {
		t.Fatalf("after restoring, earlier pages = %v, want versions 3 and 2", got)
	}

	res, body = ada.post(earlier[1], nil) // now current, so not an earlier page
	expectStatus(t, res, body, http.StatusNotFound, "")
}

// storedPage is the page kept in the store under key.
func (s *site) storedPage(key string) string {
	s.t.Helper()
	b, err := os.ReadFile(filepath.Join(s.storageDir, key))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(b)
}

func TestAFifthPageDropsTheOldestFromTheStoreAndTheRecords(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	var oldest string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dashboard_pages`).Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	for n := 2; n <= 5; n++ {
		ada.uploadPage(path, versionOf(n))
	}
	var rows int
	if err := ada.db.QueryRow(`SELECT count(*) FROM dashboard_pages WHERE storage_key = $1`, oldest).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || ada.storedFiles() != 4 {
		t.Fatalf("after 5 pages, the oldest has %d records and the store has %d files, want 0 and 4", rows, ada.storedFiles())
	}
	earlier := ada.earlierPageMatches(path)
	if len(earlier) != 3 {
		t.Fatalf("earlier pages = %v, want 3", earlier)
	}
	for i, want := range []int{4, 3, 2} { // newest first
		var key string
		if err := ada.db.QueryRow(`SELECT storage_key FROM dashboard_pages WHERE id = $1`, earlier[i][2]).Scan(&key); err != nil {
			t.Fatal(err)
		}
		if got := ada.storedPage(key); got != versionOf(want) {
			t.Fatalf("earlier page %d is %q, want version %d", i, got, want)
		}
	}
	_, body := ada.get("/lab")
	expectAll(t, body, strings.TrimSuffix(ada.storedKB(), " of 1 GB"))
}

func TestANewPageIsCheckedLikeTheFirstAndARefusedOneKeepsTheLiveOne(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", strings.Repeat("a", 600<<10))
	for _, c := range []struct {
		post func() (*http.Response, string)
		want string
	}{
		{func() (*http.Response, string) { return ada.post(path+"/page", url.Values{"html": {" "}}) },
			"Paste your page&#39;s HTML or pick a .html file."},
		{func() (*http.Response, string) { return ada.postFile(path+"/page", "jualan.xlsx", []byte(myPage)) },
			"jualan.xlsx is not a .html file."},
		{func() (*http.Response, string) {
			return ada.postFile(path+"/page", "jualan.html", []byte(strings.Repeat("x", 6<<20)))
		}, "The page is over 5 MB."},
	} {
		res, body := c.post()
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		if n := ada.storedFiles(); n != 1 {
			t.Fatalf("refused %q, but the store has %d files", c.want, n)
		}
	}
	link := ada.deployAs(path, "jualan-ada")
	if _, page := ada.second().get(link); page != strings.Repeat("a", 600<<10) {
		t.Fatalf("a refused page replaced the live one")
	}
}

func TestOnlyTheOwnerOrTheAdminChangesAPage(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	ada.uploadPage(path, versionOf(2))
	restore := ada.earlierPages(path)[0]

	bea := admin.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	admin.assignPlan("bea@example.com", admin.onlyPlanID())
	res, body := bea.post(path+"/page", url.Values{"html": {versionOf(3)}})
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = bea.post(restore, nil)
	expectStatus(t, res, body, http.StatusNotFound, "")
	var current string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dashboard_pages WHERE current`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if got := ada.storedPage(current); got != versionOf(2) {
		t.Fatalf("after others tried, the current page is %q", got)
	}

	own := admin.createPage("Admin", versionOf(1))
	admin.uploadPage(own, versionOf(2))
	res, body = admin.post(admin.earlierPages(own)[0], nil)
	expectRedirect(t, res, body, own)
}
