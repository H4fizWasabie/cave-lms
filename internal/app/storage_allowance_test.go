package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// The Storage allowance's rules, one test each, through the screens a Learner uploads on. Rules that other test files
// own: Replace data and an Add (TestAReplaceCountsTheDatasetsPartsAsFreeButAnAddDoesNot), Replace one Part
// (TestAReplaceOfOnePartCountsItsOldPartAsFree...), a re-upload over a Pending Part
// (TestPendingWorkDatasetsCountAgainstTheStorageAllowance) and the Lab's "used" (storage_test.go, workbooks_test.go).

// partBytes is the saved size of every Part and Pending Part, as the Lab counts them.
func (s *site) partBytes() int64 {
	s.t.Helper()
	var n int64
	if err := s.db.QueryRow(`SELECT COALESCE(sum(saved_size), 0) FROM dataset_parts`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// usedBytes is everything the Storage allowance counts: Parts and Pending Parts, Workbooks and pages.
func (s *site) usedBytes() int64 {
	s.t.Helper()
	var n int64
	if err := s.db.QueryRow(`SELECT (SELECT COALESCE(sum(saved_size), 0) FROM dataset_parts) + (SELECT COALESCE(sum(size), 0) FROM workbooks) +
		(SELECT COALESCE(sum(size), 0) FROM dashboard_pages)`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// A Pending Part counts in every check, whichever Dashboard it is on: a page of this size, or a Workbook, fits beside
// the Learner's other files but not beside the Pending Part.
func TestAPendingPartOnAnotherDashboardLeavesNoRoomForAPageOrAWorkbook(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "2")
	page := ada.createPage("Jualan", myPage)
	res, body := ada.postFile(ada.createWork("Kedai")+"/dataset", "data.xlsx", bigFinance(t, 1)) // pending: about 1.7 MB used
	expectStatus(t, res, body, http.StatusSeeOther, "")
	room := 2<<20 - ada.usedBytes()
	if room <= 0 || room > 600<<10 {
		t.Fatalf("%d bytes of room left, want a little", room)
	}
	big := strings.Repeat("x", int(room)+1<<10) // the Pending Part is about 1.1 MB: without it, this fits

	res, body = ada.post(page+"/page", url.Values{"html": {big}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	res, body = ada.post("/lab/dashboards", url.Values{"name": {"Baru"}, "html": {big}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	res, body = ada.postFile("/workbooks", "besar.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
}

// A page counts its own size, and the live one stays when it is refused, on the new page Dashboard and the new page alike.
func TestAPageOverTheAllowanceIsRefusedAndKeepsTheLiveOne(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "1")
	live := strings.Repeat("a", 600<<10)
	path := ada.createPage("Jualan", live)

	res, body := ada.post(path+"/page", url.Values{"html": {strings.Repeat("b", 600<<10)}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	res, body = ada.post("/lab/dashboards", url.Values{"name": {"Baru"}, "html": {strings.Repeat("c", 600<<10)}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("refused, but the store has %d files", n)
	}
	var dashboards int
	if err := ada.db.QueryRow(`SELECT count(*) FROM dashboards`).Scan(&dashboards); err != nil || dashboards != 1 {
		t.Fatalf("%d Dashboards after a refused new page Dashboard, %v", dashboards, err)
	}
	if _, got := ada.second().get(ada.deployAs(path, "jualan-ada")); got != live {
		t.Fatalf("a refused page replaced the live one")
	}
}

// A new page pushes out the oldest of the earlier pages past the three kept, and counts that one as free.
func TestANewPageCountsTheEarlierPageItPushesOutAsFree(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "1")
	version := func(letter string, kb int) string { return strings.Repeat(letter, kb<<10) }
	path := ada.createPage("Jualan", version("a", 240))
	for _, letter := range []string{"b", "c", "d"} {
		ada.uploadPage(path, version(letter, 240)) // the current page and three earlier: 960 KB of 1 MB
	}
	res, body := ada.post(path+"/page", url.Values{"html": {version("e", 330)}}) // 960 + 330 - 240 is over
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	ada.uploadPage(path, version("f", 250)) // 960 + 250 - 240 fits only because the oldest goes
	if n := ada.storedFiles(); n != 4 {
		t.Fatalf("%d stored pages, want the new one and three earlier", n)
	}
}

// Two uploads at once, a page and an Excel file, cannot both slip under the allowance: they take turns.
func TestTwoUploadsAtOnceCannotBothSlipUnderTheAllowance(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "1")
	uploads := []func() (*http.Response, string){
		func() (*http.Response, string) {
			return ada.post("/lab/dashboards", url.Values{"name": {"Satu"}, "html": {strings.Repeat("p", 700<<10)}})
		},
		func() (*http.Response, string) { return ada.postFile("/workbooks", "satu.xlsx", bigFinance(t, 1)) },
	}
	codes := make([]int, len(uploads))
	var wg sync.WaitGroup
	for i, upload := range uploads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := upload()
			codes[i] = res.StatusCode
		}()
	}
	wg.Wait()
	if !(codes[0] == http.StatusSeeOther) != (codes[1] == http.StatusSeeOther) {
		t.Fatalf("a page and an Excel file at once answered %v, want one accepted and one refused", codes)
	}
}

// Every refusal is one message: the file's size, what is used and the allowance, in the Learner's language, and a line
// for the Pending Parts only when one counts.
func TestARefusalShowsTheSizesAndTheWaitingLineOnlyWhenAPendingPartCounts(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "2")
	file := bigFinance(t, 2)
	if _, err := ada.db.Exec(`INSERT INTO workbooks (learner_id, storage_key, name, size, sha256) VALUES ($1, 'workbook-filler', 'filler.xlsx', 1700000, '\x00')`, ada.adaID()); err != nil {
		t.Fatal(err)
	}
	en := "This " + storageText(int64(len(file))) + " file would take you over your Storage allowance: your Workbooks, Datasets and pages use " +
		storageText(ada.usedBytes()) + " of your 2 MB. Delete a Dashboard you no longer need, delete Workbooks in your Folder, or upload a smaller file."
	ms := "Fail " + storageText(int64(len(file))) + " ini akan melebihi had storan anda: Buku kerja, set data dan halaman anda menggunakan " +
		storageText(ada.usedBytes()) + " daripada 2 MB. Padam Dashboard yang tidak diperlukan lagi, padam Buku kerja dalam Folder anda, atau muat naik fail yang lebih kecil."

	res, body := ada.postFile("/workbooks", "besar.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, en)
	if strings.Contains(body, "waiting for you to confirm") {
		t.Fatalf("the message names Pending Parts though none counts: %s", body)
	}
	ada.switchLanguage("ms", "/")
	res, body = ada.postFile("/workbooks", "besar.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, ms)
	if strings.Contains(body, "menunggu pengesahan") {
		t.Fatalf("the Malay message names Pending Parts though none counts: %s", body)
	}

	// A Pending Part now counts: the message says how much of what is used it is.
	if _, err := ada.db.Exec(`DELETE FROM workbooks`); err != nil {
		t.Fatal(err)
	}
	ada.switchLanguage("en", "/")
	res, body = ada.postFile(ada.createWork("Kedai")+"/dataset", "data.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	pending := storageText(ada.partBytes())
	res, body = ada.postFile("/workbooks", "besar.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "your Workbooks, Datasets and pages use "+storageText(ada.usedBytes())+" of your 2 MB.")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, pending+" of that is in previews waiting for you to confirm.")
	ada.switchLanguage("ms", "/")
	res, body = ada.postFile("/workbooks", "besar.xlsx", file)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "menggunakan "+storageText(ada.usedBytes())+" daripada 2 MB.")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, pending+" daripadanya ialah pratonton yang menunggu pengesahan anda.")
}

func TestTheAdminsUploadsAreNeverRefusedForStorage(t *testing.T) {
	t.Parallel()
	admin, _, planID := storageSite(t, "0")
	admin.assignPlan("admin@example.com", planID)
	path := admin.createWork("Kewangan")
	admin.confirmFile(path, "mine.xlsx", admin.financeSample())
	page := admin.createPage("Jualan", myPage)
	admin.uploadPage(page, versionOf(2))
	res, body := admin.postFile("/workbooks", "lain.xlsx", bigFinance(t, 1))
	expectRedirect(t, res, body, "/my-folder")
	_, body = admin.get("/lab")
	if strings.Contains(body, "of 0 KB") {
		t.Fatalf("the Admin's Lab shows a Storage allowance: %s", body)
	}
}
