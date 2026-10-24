package app_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// confirmFile uploads file to the Work Dashboard at path and confirms it.
func (s *site) confirmFile(path, name string, file []byte) {
	s.t.Helper()
	res, body := s.postFile(path+"/dataset", name, file)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	s.confirmWork(path, financeGuess)
}

var restoreAction = regexp.MustCompile(`action="/lab/dashboards/\d+/datasets/(\d+)/restore"`)

// earlier lists the earlier Datasets on the Dataset tab of the Work Dashboard at path, by file name: each one's
// path, which /restore and /delete follow.
func (s *site) earlier(path string) map[string]string {
	s.t.Helper()
	res, body := s.get(path)
	expectStatus(s.t, res, body, http.StatusOK, "")
	out := map[string]string{}
	for _, m := range restoreAction.FindAllStringSubmatchIndex(body, -1) {
		li := body[:m[0]]
		li = li[strings.LastIndex(li, "<li>"):]
		name := li[strings.Index(li, "<strong>")+len("<strong>") : strings.Index(li, "</strong>")]
		out[name] = path + "/datasets/" + body[m[2]:m[3]]
	}
	return out
}

func TestAFifthConfirmDropsTheOldestEarlierDataset(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	for i, item := range []string{"Beras", "Gula", "Garam", "Minyak", "Tepung"} {
		ada.confirmFile(path, fmt.Sprintf("%d.xlsx", i+1), shopReport(t, item, 10))
	}
	got := ada.earlier(path)
	if len(got) != 3 || got["2.xlsx"] == "" || got["3.xlsx"] == "" || got["4.xlsx"] == "" {
		t.Fatalf("earlier Datasets = %v, want 2, 3 and 4", got)
	}
	res, body := ada.get(path)
	for _, want := range []string{"5.xlsx", "Earlier Datasets", "2 rows"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if n := ada.storedFiles(); n != 4 {
		t.Fatalf("%d stored files, want the current and 3 earlier", n)
	}
}

// blocksOf is the Dashboard page at page (ending in /) without its data address: its blocks and filter bar.
func (s *site) blocksOf(page string) string {
	s.t.Helper()
	_, body := s.get(page)
	return strings.ReplaceAll(body, s.dataOf(page), "")
}

func TestRestoreSwapsTheCurrentAndAnEarlierDatasetAndKeepsTheBlocks(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	link := ada.deploy(path)
	viewer := ada.second()
	ada.confirmFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	blocks := ada.blocksOf(path + "/view/")

	september := ada.earlier(path)["september.xlsx"]
	res, body := ada.post(september+"/restore", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for _, page := range []string{ada.rowsOf(path + "/view/"), viewer.rowsOf(link)} {
		if !strings.Contains(page, "Beras") || strings.Contains(page, "Gula") {
			t.Fatalf("after restoring, data = %s", page)
		}
	}
	if got := ada.earlier(path); len(got) != 1 || got["oktober.xlsx"] == "" {
		t.Fatalf("earlier Datasets = %v, want oktober.xlsx", got)
	}
	if ada.blocksOf(path+"/view/") != blocks {
		t.Fatal("restoring changed the blocks")
	}
	res, body = ada.post(september+"/restore", nil) // now current, not earlier
	expectStatus(t, res, body, http.StatusNotFound, "")
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want 2", n)
	}
}

// workSized is a Finance Dataset whose saved size is about kb KB: text that does not compress.
func workSized(t *testing.T, seed int64, kb int) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(seed))
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	var rows [][]any
	for i := 0; i < kb*1024/244; i++ { // a row saves as about 244 bytes
		b := make([]byte, 200)
		for j := range b {
			b[j] = letters[rnd.Intn(len(letters))]
		}
		rows = append(rows, []any{day, string(b), "Dapur", "Keluar", float64(i)})
	}
	return testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, rows...)
}

func TestAnUploadOverTheAllowanceDropsThisDashboardsOldestEarlierDatasets(t *testing.T) {
	_, ada, _ := storageSite(t, "1") // 1024 KB
	other := ada.createWork("finance", "Lain")
	ada.confirmFile(other, "lain.xlsx", workSized(t, 1, 400))
	path := ada.createWork("finance", "Kedai")
	ada.confirmFile(path, "1.xlsx", workSized(t, 2, 150))
	ada.confirmFile(path, "2.xlsx", workSized(t, 3, 150)) // 700 KB used

	res, body := ada.postFile(path+"/dataset", "3.xlsx", workSized(t, 4, 550)) // fits once 1 and 2 go
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Confirming deletes these earlier Datasets")
	drops := body[strings.Index(body, "Confirming deletes"):]
	drops = drops[:strings.Index(drops, "</ul>")]
	if !strings.Contains(drops, "1.xlsx") || !strings.Contains(drops, "2.xlsx") {
		t.Fatalf("the preview lists %s", drops)
	}
	ada.confirmWork(path, nil)
	if got := ada.earlier(path); len(got) != 0 {
		t.Fatalf("earlier Datasets = %v, want none", got)
	}
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want this Dashboard's current one and the other Dashboard's", n)
	}

	res, body = ada.postFile(path+"/dataset", "4.xlsx", workSized(t, 5, 700)) // 400 + 700 even with 3 gone
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
	if n := ada.storedFiles(); n != 3 {
		t.Fatalf("%d stored files, want both current ones and the pending one", n)
	}
}

func TestEarlierDatasetsGoNinetyDaysAfterTheLastPeriodButTheAdminsStay(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	ada.confirmFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	demo := admin.createWork("finance", "Demo")
	for i := range 5 {
		admin.confirmFile(demo, fmt.Sprintf("%d.xlsx", i+1), shopReport(t, "Beras", 10))
	}
	if got := admin.earlier(demo); len(got) != 3 {
		t.Fatalf("the Admin keeps %v, want 3 earlier", got)
	}
	if n := ada.storedFiles(); n != 6 {
		t.Fatalf("%d stored files, want 6", n)
	}

	admin.ageAllPeriods(30 + 91)
	admin.prune()
	if n := ada.storedFiles(); n != 4 {
		t.Fatalf("after 91 days %d stored files, want the Admin's 4", n)
	}
}

// storedKB is the Storage allowance's KB the files in the store take, as /lab shows it.
func (s *site) storedKB() string {
	s.t.Helper()
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		s.t.Fatal(err)
	}
	var n int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			s.t.Fatal(err)
		}
		n += info.Size()
	}
	return fmt.Sprintf("%d KB of 1 GB", (n+1023)>>10)
}

func TestDeletingAnEarlierDatasetFreesItsStorage(t *testing.T) {
	_, ada, _ := storageSite(t, "1024")
	path := ada.createWork("finance", "Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	ada.confirmFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, ada.storedKB())

	res, body = ada.post(ada.earlier(path)["september.xlsx"]+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want the current one", n)
	}
	if got := ada.earlier(path); len(got) != 0 {
		t.Fatalf("earlier Datasets = %v after deleting", got)
	}
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, ada.storedKB())
	if !strings.Contains(ada.rowsOf(path+"/view/"), "Gula") {
		t.Fatal("deleting an earlier Dataset changed the current one")
	}
}
