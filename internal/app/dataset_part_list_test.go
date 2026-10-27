package app_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

var partDeleteLink = regexp.MustCompile(`href="(/lab/dashboards/\d+/parts/\d+/delete)"`)

// partList is the Parts section of a Data tab: from the list to the end of the page.
func partList(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `class="ds-parts`)
	if i < 0 {
		t.Fatalf("the Data tab lists no Parts: %s", body)
	}
	return body[i:]
}

// twoParts is a deployed page Dashboard of Ada's holding september.xlsx and oktober.xlsx as its two Parts.
func twoParts(t *testing.T, ada *site) (path, link string) {
	t.Helper()
	path, link = pageWithData(t, ada)
	res, body := ada.postFile(path+"/dataset", "oktober.xlsx", shopColumns(t, "Gula", 80), adding)
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	return path, link
}

func TestTheDataTabListsEveryPartInOrderWithItsDateFileRowsSizeAndWhenAdded(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)

	list := partList(t, mustGet(t, ada, path+"/data"))
	first, second := strings.Index(list, "september.xlsx"), strings.Index(list, "oktober.xlsx")
	if first < 0 || second < first {
		t.Fatalf("the Parts are not listed in order, september.xlsx then oktober.xlsx: %s", list)
	}
	expectAll(t, list, "Part date "+partDate(), "2 rows", " KB", "added "+partDate())

	ada.switchLanguage("ms", path+"/data")
	expectAll(t, partList(t, mustGet(t, ada, path+"/data")), "Tarikh Bahagian "+partDate(), "2 baris", "ditambah "+partDate())
}

func mustGet(t *testing.T, s *site, path string) string {
	t.Helper()
	res, body := s.get(path)
	expectStatus(t, res, body, http.StatusOK, "")
	return body
}

func TestDeletingAPartAsksFirstThenRemovesItsRowsFreesItsRoomAndQueuesItsFile(t *testing.T) {
	_, ada := labLearner(t)
	path, link := twoParts(t, ada)
	visitor := ada.second()
	ada.forgetWorkbooks()
	before := ada.storedKB()
	del := partDeleteLink.FindAllStringSubmatch(mustGet(t, ada, path+"/data"), -1)
	if len(del) != 2 {
		t.Fatalf("%d Delete links, want one per Part", len(del))
	}

	res, body := ada.get(del[0][1])
	expectStatus(t, res, body, http.StatusOK, "september.xlsx")
	expectAll(t, body, "Delete Part")
	res, body = ada.post(del[0][1], nil) // not from the confirm screen
	expectRedirect(t, res, body, del[0][1])
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("an unconfirmed delete left %d stored files, want 2", n)
	}

	res, body = ada.post(del[0][1], confirming)
	expectRedirect(t, res, body, path+"/data")
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Beras") || !strings.Contains(body, "Gula") {
		t.Fatalf("after deleting the first Part data.json = %s", body)
	}
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files after the delete, want the one Part left", n)
	}
	var queued int
	if err := ada.db.QueryRow(`SELECT count(*) FROM stored_file_deletions`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("%d files still queued for deletion (%v), want the removed Part's deleted", queued, err)
	}
	if ada.storedKB() == before {
		t.Fatalf("the Storage allowance still shows %s", before)
	}
	_, body = ada.get(path + "/data")
	if strings.Contains(partList(t, body), "september.xlsx") || !strings.Contains(partList(t, body), "oktober.xlsx") {
		t.Fatalf("the Part list after the delete: %s", partList(t, body))
	}
	if res, _ = ada.post(del[0][1], confirming); res.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting a Part twice answered %d, want 404", res.StatusCode)
	}
}

func TestADeleteOnlyReachesThePartsOfTheLearnersOwnDashboard(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	other := ada.createPage("Lain", myPage)
	del := partDeleteLink.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	crossed := other + del[strings.Index(del, "/parts/"):]
	if res, _ := ada.post(crossed, confirming); res.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting another Dashboard's Part answered %d, want 404", res.StatusCode)
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want both Parts kept", n)
	}
}

func TestDeletingTheLastPartLeavesDataJSONAnEmptyArrayAndTheNextUploadSetsNewColumns(t *testing.T) {
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	visitor := ada.second()
	del := partDeleteLink.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	res, body := ada.post(del, confirming)
	expectRedirect(t, res, body, path+"/data")

	res, body = visitor.get(link + "data.json")
	expectStatus(t, res, body, http.StatusOK, "")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil || len(rows) != 0 {
		t.Fatalf("data.json = %q (%v), want an empty array", body, err)
	}
	body = mustGet(t, ada, path+"/data")
	if strings.Contains(body, `class="ds-parts`) || strings.Contains(body, `name="mode"`) {
		t.Fatalf("an empty Dataset still lists Parts or offers Replace and Add: %s", body)
	}

	// The next upload is accepted with any columns, and sets them.
	other := testutil.XLSX(t, []string{"Nama", "Markah"}, []any{"Ali", 80.0}, []any{"Siti", 90.0})
	res, body = ada.postFile(path+"/dataset", "markah.xlsx", other)
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	_, body = visitor.get(link + "data.json")
	expectAll(t, body, `"Nama": "Ali"`, `"Markah": 90`)
	if strings.Contains(body, "Perkara") {
		t.Fatalf("data.json still holds the deleted Part: %s", body)
	}
}

func TestReplaceDataWarnsHowManyPartsGoAndThatTheirWorkbooksStay(t *testing.T) {
	_, ada := labLearner(t)
	path, link := twoParts(t, ada)
	visitor := ada.second()
	for _, c := range []struct{ lang, want string }{
		{"en", "Confirming removes all 2 Parts of your Dataset. Their Workbooks stay in your Folder."},
		{"ms", "Pengesahan membuang semua 2 Bahagian Set data anda. Buku kerja mereka kekal dalam Folder anda."},
	} {
		ada.switchLanguage(c.lang, path+"/data")
		res, body := ada.postFile(path+"/dataset", "disember.xlsx", shopColumns(t, "Tepung", 60))
		expectRedirect(t, res, body, path+"/data")
		if _, body = ada.get(path + "/data"); !strings.Contains(body, c.want) {
			t.Fatalf("the Replace preview in %s lacks %q: %s", c.lang, c.want, body)
		}
	}
	if _, body := visitor.get(link + "data.json"); strings.Contains(body, "Tepung") {
		t.Fatalf("a Replace changed data.json before it was confirmed: %s", body)
	}

	// An Add does not delete anything, so it has no warning.
	ada.switchLanguage("en", path+"/data")
	ada.postFile(path+"/dataset", "tambah.xlsx", shopColumns(t, "Garam", 70), adding)
	if _, body := ada.get(path + "/data"); strings.Contains(body, "Confirming removes all") {
		t.Fatalf("an Add's preview warns of Parts going: %s", body)
	}

	// One Part is warned of too.
	ada.postFile(path+"/dataset", "disember.xlsx", shopColumns(t, "Tepung", 60))
	ada.confirmWork(path, nil)
	ada.postFile(path+"/dataset", "januari.xlsx", shopColumns(t, "Minyak", 50))
	if _, body := ada.get(path + "/data"); !strings.Contains(body, "Confirming removes the 1 Part of your Dataset. Its Workbook stays in your Folder.") {
		t.Fatalf("a Replace of one Part has no warning: %s", body)
	}

	// After the last Part is deleted there is nothing to warn about.
	ada.confirmWork(path, nil)
	del := partDeleteLink.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	ada.post(del, confirming)
	ada.postFile(path+"/dataset", "januari.xlsx", shopColumns(t, "Minyak", 50))
	if _, body := ada.get(path + "/data"); strings.Contains(body, "Confirming removes all") {
		t.Fatalf("an empty Dataset's first upload warns of Parts going: %s", body)
	}
}
