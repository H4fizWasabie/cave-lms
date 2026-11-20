package app_test

import (
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var caveHostRestoreAction = regexp.MustCompile(`action="(/cavehost/[a-z-]+/pages/\d+/restore)"`)

// caveHostEarlierPages is the restore actions of the earlier pages on the Deployment's page, in the order listed.
func (s *site) caveHostEarlierPages(name string) []string {
	s.t.Helper()
	res, body := s.get("/cavehost/" + name)
	expectStatus(s.t, res, body, http.StatusOK, "")
	var out []string
	for _, m := range caveHostRestoreAction.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// caveHostPageFiles is the pages kept on the Disk store.
func (s *site) caveHostPageFiles() []string {
	files, _ := filepath.Glob(filepath.Join(s.storageDir, "cavehost-page-*"))
	return files
}

func TestANewPageReplacesTheLiveOneAtTheSameLinkAndAnEarlierOneCanBeRestored(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.putPageOnline("jualan-ada", versionOf(1))
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()

	res, body := ada.get("/cavehost/jualan-ada")
	expectAll(t, body, `action="/cavehost/jualan-ada/page"`, `type="file" name="file"`, "Replace your page")
	if strings.Contains(body, "Earlier pages") {
		t.Fatalf("a Deployment with one page lists earlier pages: %s", body)
	}

	ada.putPageOnline("jualan-ada", versionOf(2))
	if _, page := visitor.get(link); page != versionOf(2) {
		t.Fatalf("after a new page, the link shows %q", page)
	}
	if got := ada.caveHostLink("jualan-ada"); got != link {
		t.Fatalf("the link changed from %q to %q", link, got)
	}
	res, body = ada.postFile("/cavehost/jualan-ada/page", "v3.html", []byte(versionOf(3)))
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	if _, page := visitor.get(link); page != versionOf(3) {
		t.Fatalf("after a new page by file, the link shows %q", page)
	}

	res, body = ada.get("/cavehost/jualan-ada")
	expectAll(t, body, "Earlier pages", "Uploaded ")
	earlier := ada.caveHostEarlierPages("jualan-ada")
	if len(earlier) != 2 {
		t.Fatalf("earlier pages = %v, want versions 2 and 1", earlier)
	}
	// The earlier pages are kept on the Disk store, beside the live one.
	if files := ada.caveHostPageFiles(); len(files) != 3 {
		t.Fatalf("the store holds %v, want all three pages", files)
	}
	res, body = ada.post(earlier[1], nil) // version 1, the oldest
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	if _, page := visitor.get(link); page != versionOf(1) {
		t.Fatalf("after restoring version 1, the link shows %q", page)
	}
	if got := ada.caveHostEarlierPages("jualan-ada"); len(got) != 2 || slices.Contains(got, earlier[1]) {
		t.Fatalf("after restoring, earlier pages = %v, want versions 3 and 2", got)
	}

	res, body = ada.post(earlier[1], nil) // now live, so not an earlier page
	expectStatus(t, res, body, http.StatusNotFound, "")
}

func TestAFifthCaveHostPageDropsTheOldestFromTheStoreAndTheRecords(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	for n := 1; n <= 4; n++ {
		ada.putPageOnline("jualan-ada", versionOf(n))
	}
	earlier := ada.caveHostEarlierPages("jualan-ada")
	if len(earlier) != 3 {
		t.Fatalf("earlier pages = %v, want 3", earlier)
	}
	oldest := earlier[2]
	ada.putPageOnline("jualan-ada", versionOf(5))

	if got := ada.caveHostEarlierPages("jualan-ada"); len(got) != 3 || slices.Contains(got, oldest) {
		t.Fatalf("earlier pages = %v, want versions 4, 3 and 2 only", got)
	}
	files := ada.caveHostPageFiles()
	if len(files) != 4 {
		t.Fatalf("the store holds %v, want the live page and three earlier ones", files)
	}
	for _, f := range files {
		if strings.Contains(ada.storedPage(filepath.Base(f)), "Versi 1<") {
			t.Fatalf("version 1 is still in the store at %s", f)
		}
	}
	if res, body := ada.post(oldest, nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("restoring the dropped page = %d, want 404: %s", res.StatusCode, body)
	}
}

func TestOnlyTheOwnerCanRestoreACaveHostPageAndARefusedPageKeepsTheLiveOne(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.putPageOnline("jualan-ada", versionOf(1))
	ada.putPageOnline("jualan-ada", versionOf(2))
	restore := ada.caveHostEarlierPages("jualan-ada")[0]
	link := ada.caveHostLink("jualan-ada")

	other := ada.second()
	other.loginAsNewAdmin("Abah", "abah@example.com", "correct horse battery") // CaveHost is open to the Admin, who still does not own the page
	if res, body := other.post(restore, nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("another Learner restoring = %d, want 404: %s", res.StatusCode, body)
	}
	if res, body := ada.post(strings.Replace(restore, "/jualan-ada/", "/nope/", 1), nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("restoring on an unknown Deployment = %d, want 404: %s", res.StatusCode, body)
	}

	res, body := ada.post("/cavehost/jualan-ada/page", url.Values{"html": {"  "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Paste your page")
	if _, page := ada.second().get(link); page != versionOf(2) {
		t.Fatalf("a refused page changed the link to %q", page)
	}
}
