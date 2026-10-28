package app_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// confirmFile uploads file to the Work Dashboard at path and confirms it.
func (s *site) confirmFile(path, name string, file []byte) {
	s.t.Helper()
	res, body := s.postFile(path+"/dataset", name, file)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	s.confirmWork(path, financeGuess)
}

// addFile uploads file to the Work Dashboard at path as an Add to data and confirms it, keeping the blocks.
func (s *site) addFile(path, name string, file []byte) {
	s.t.Helper()
	res, body := s.postFile(path+"/dataset", name, file, adding)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	s.confirmWork(path, nil)
}

// blocksOf is the Dashboard page at page (ending in /) without its data address: its blocks and filter bar.
func (s *site) blocksOf(page string) string {
	s.t.Helper()
	_, body := s.get(page)
	return strings.ReplaceAll(body, s.dataOf(page), "")
}

// perkara lists the Perkara and Part date of every row in the data the Dashboard page at page (ending in /) draws, in
// order.
func (s *site) perkara(page string) []string {
	s.t.Helper()
	var out []string
	for _, line := range strings.Split(s.rowsOf(page), "\n") {
		if i := strings.Index(line, `"Perkara": "`); i >= 0 {
			rest := line[i+len(`"Perkara": "`):]
			item := rest[:strings.Index(rest, `"`)]
			date := line[strings.Index(line, `"Part date": "`)+len(`"Part date": "`):]
			out = append(out, item+" "+date[:10])
		}
	}
	return out
}

func TestAddingToABuiltDashboardAddsAPartAndTheDashboardDrawsFromAllTheRows(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	link := ada.deploy(path)
	blocks := ada.blocksOf(path + "/view/")
	first := ada.dataOf(path + "/view/")

	ada.addFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	today := time.Now().UTC().Format(time.DateOnly)
	want := []string{"Beras " + today, "Jualan " + today, "Gula " + today, "Jualan " + today}
	for _, page := range []string{path + "/view/", link} {
		if got := ada.perkara(page); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s draws %v, want %v", page, got, want)
		}
	}
	if ada.blocksOf(path+"/view/") != blocks {
		t.Fatal("adding a Part changed the blocks")
	}
	if ada.dataOf(path+"/view/") == first {
		t.Fatal("adding a Part kept the data address")
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want one per Part", n)
	}
	_, body := ada.get(path)
	expectAll(t, body, "oktober.xlsx", "2 Parts", "4 rows")
}

func TestReplaceDataLeavesOnePartAndQueuesTheRemovedPartsFilesForDeletion(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	ada.addFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	ada.addFile(path, "november.xlsx", shopReport(t, "Garam", 70))
	if n := ada.datasetFiles(); n != 3 {
		t.Fatalf("%d stored files, want 3 Parts", n)
	}

	// With several Parts a Replace is not confirmed at once, as it would delete them all.
	res, body := ada.postFile(path+"/dataset", "disember.xlsx", shopReport(t, "Tepung", 60))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path {
		t.Fatalf("a Replace of 3 Parts redirects to %q, want its preview", loc)
	}
	if rows := ada.rowsOf(path + "/view/"); !strings.Contains(rows, "Garam") || strings.Contains(rows, "Tepung") {
		t.Fatalf("before confirming, data = %s", rows)
	}
	ada.confirmWork(path, nil)
	if got := ada.perkara(path + "/view/"); len(got) != 2 || !strings.HasPrefix(got[0], "Tepung ") {
		t.Fatalf("after Replace data the rows are %v, want the one new Part's", got)
	}
	var parts, queued int
	if err := ada.db.QueryRow(`SELECT count(*) FILTER (WHERE state = 'current'), (SELECT count(*) FROM stored_file_deletions) FROM dataset_parts`).Scan(&parts, &queued); err != nil {
		t.Fatal(err)
	}
	if n := ada.datasetFiles(); parts != 1 || n != 1 || queued != 0 {
		t.Fatalf("%d Parts, %d stored files, %d still queued, want 1, 1 and 0: the removed Parts' files deleted", parts, n, queued)
	}
	_, body = ada.get(path)
	expectAll(t, body, "disember.xlsx", "2 rows")
}

// A current Dataset from before Parts that was last changed by an Add to data still holds the whole table: only a
// pending Add is cut to its new rows. Production lost rows to this on 2026-10-27.
func TestACurrentDatasetFromBeforePartsLastChangedByAnAddKeepsEveryRow(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	link := ada.deploy(path)
	var key string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dataset_parts`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if err := app.MigrateDownTo(context.Background(), ada.db, 127); err != nil {
		t.Fatal(err)
	}
	whole := `{"columns": [{"name": "Tarikh", "type": "date"}, {"name": "Perkara", "type": "text"}, {"name": "Kategori", "type": "text"},
		{"name": "Jenis", "type": "text"}, {"name": "Jumlah", "type": "number"}],
		"rows": [["2026-10-05", "Beras", "Dapur", "Keluar", 120], ["2026-10-06", "Jualan", "Kedai", "Masuk", 900]]}`
	if err := os.WriteFile(filepath.Join(ada.storageDir, key), []byte(whole), 0o600); err != nil {
		t.Fatal(err)
	}
	// Confirmed as an Add of one row to the one it had: the Clean-up record keeps saying so after the confirm.
	if _, err := ada.db.Exec(`UPDATE datasets SET confirmed_at = '2026-03-09 10:00+00', row_count = 2,
		cleanup = jsonb_set(cleanup, '{added}', '{"had": 1}')`); err != nil {
		t.Fatal(err)
	}
	ada.restart()
	link = ada.linkAfterMigration(path)

	want := []string{"Beras 2026-03-09", "Jualan 2026-03-09"}
	for _, page := range []string{path + "/view/", link} {
		if got := ada.perkara(page); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s draws %v, want every row of the Dataset: %v", page, got, want)
		}
	}
}

// Datasets before Parts keep their earlier copies, read-only and counted, until the Dashboard's data next changes.
func TestADatasetFromBeforePartsBecomesOnePartAndItsEarlierCopiesGoOnTheNextConfirm(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	link := ada.deploy(path)
	var key string
	if err := ada.db.QueryRow(`SELECT storage_key FROM dataset_parts`).Scan(&key); err != nil {
		t.Fatal(err)
	}

	// Put the Dashboard back as it was before Parts: one current Dataset stored whole, confirmed on 9 March, and an
	// earlier one.
	if err := app.MigrateDownTo(context.Background(), ada.db, 127); err != nil {
		t.Fatal(err)
	}
	legacy := `{"columns": [{"name": "Tarikh", "type": "date"}, {"name": "Perkara", "type": "text"}, {"name": "Kategori", "type": "text"},
		{"name": "Jenis", "type": "text"}, {"name": "Jumlah", "type": "number"}],
		"rows": [["2026-10-05", "Beras", "Dapur", "Keluar", 120], ["2026-10-05", "Jualan", "Kedai", "Masuk", 900]]}`
	if err := os.WriteFile(filepath.Join(ada.storageDir, key), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	earlier := "dataset-earlier-before-parts"
	if err := os.WriteFile(filepath.Join(ada.storageDir, earlier), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ada.db.Exec(`UPDATE datasets SET confirmed_at = '2026-03-09 10:00+00'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ada.db.Exec(`UPDATE dashboards SET upload_settings = upload_settings - 'recipe'`); err != nil { // none before Parts
		t.Fatal(err)
	}
	if _, err := ada.db.Exec(`INSERT INTO datasets (dashboard_id, storage_key, file_name, saved_size, row_count, state, confirmed_at)
		SELECT dashboard_id, $1, 'agosto.xlsx', 1000, 2, 'earlier', '2026-02-01 10:00+00' FROM datasets`, earlier); err != nil {
		t.Fatal(err)
	}
	ada.restart()
	link = ada.linkAfterMigration(path)

	// The Dataset has the recipe its next Parts are read with: the sheet its Part was read from, the header not typed.
	var recipe string
	if err := ada.db.QueryRow(`SELECT upload_settings->'recipe'->>'sheet' || '/' || COALESCE(upload_settings->'recipe'->>'headerTyped', 'no')
		FROM dashboards`).Scan(&recipe); err != nil || recipe != "Sheet1/no" {
		t.Fatalf("recipe after the conversion = %q (%v), want Sheet1/no", recipe, err)
	}

	want := []string{"Beras 2026-03-09", "Jualan 2026-03-09"}
	for _, page := range []string{path + "/view/", link} {
		if got := ada.perkara(page); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s draws %v, want %v", page, got, want)
		}
	}
	var parts, earlierParts int
	if err := ada.db.QueryRow(`SELECT count(*) FILTER (WHERE state = 'current' AND position = 1), count(*) FILTER (WHERE state = 'earlier') FROM dataset_parts`).Scan(&parts, &earlierParts); err != nil {
		t.Fatal(err)
	}
	if parts != 1 || earlierParts != 1 {
		t.Fatalf("%d current Parts and %d earlier, want 1 and 1", parts, earlierParts)
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want the Part and the earlier Dataset", n)
	}
	_, body := ada.get(path)
	for _, gone := range []string{"Earlier Datasets", "Restore", "agosto.xlsx"} {
		if strings.Contains(body, gone) {
			t.Fatalf("the Data tab still shows %q:\n%s", gone, body)
		}
	}

	ada.addFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	if err := ada.db.QueryRow(`SELECT count(*) FILTER (WHERE state = 'current'), count(*) FILTER (WHERE state = 'earlier') FROM dataset_parts`).Scan(&parts, &earlierParts); err != nil {
		t.Fatal(err)
	}
	if parts != 2 || earlierParts != 0 {
		t.Fatalf("after the next confirm %d current Parts and %d earlier, want 2 and 0", parts, earlierParts)
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want both Parts and no earlier Dataset", n)
	}
}

func TestTheDataTabOffersNoEarlierDatasetsOrRestoreAndPointsToTheWorkbooksInTheFolder(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	ada.confirmFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	for lang, want := range map[string]string{
		"en": "Cave keeps no earlier copies of your data. Every .xlsx you upload stays as a Workbook in your Folder",
		"ms": "Cave tidak menyimpan salinan data terdahulu. Setiap fail .xlsx yang anda muat naik kekal sebagai Buku kerja dalam Folder anda",
	} {
		ada.switchLanguage(lang, path)
		res, body := ada.get(path)
		expectStatus(t, res, body, http.StatusOK, want)
		expectAll(t, body, `href="/my-folder"`)
		for _, gone := range []string{"Earlier Datasets", "Set data terdahulu", "Restore", "Pulihkan", "/datasets/"} {
			if strings.Contains(body, gone) {
				t.Fatalf("the Data tab in %s still offers %q", lang, gone)
			}
		}
	}
	for _, p := range []string{"/datasets/1/restore", "/datasets/1/delete"} {
		if res, _ := ada.post(path+p, confirming); res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s = %d, want it gone", p, res.StatusCode)
		}
	}
}

// workSized is a Finance Dataset whose saved size is about kb KB: text that does not compress.
func workSized(t *testing.T, seed int64, kb int) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(seed))
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	var rows [][]any
	for i := 0; i < kb*1024/330; i++ { // a row saves as about 330 bytes
		b := make([]byte, 200)
		for j := range b {
			b[j] = letters[rnd.Intn(len(letters))]
		}
		rows = append(rows, []any{day, string(b), "Dapur", "Keluar", float64(i)})
	}
	return testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, rows...)
}

// A Replace frees the Parts it deletes, so it fits where an Add of the same size does not.
func TestAReplaceCountsTheDatasetsPartsAsFreeButAnAddDoesNot(t *testing.T) {
	t.Parallel()
	// 2048 KB. Each upload also keeps its Workbook, about 3/4 of the Part's size; they are forgotten here, but the upload
	// being checked has its own.
	_, ada, _ := storageSite(t, "2")
	other := ada.createWork("Lain")
	ada.confirmFile(other, "lain.xlsx", workSized(t, 1, 400))
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "1.xlsx", workSized(t, 2, 150))
	ada.addFile(path, "2.xlsx", workSized(t, 3, 150))
	ada.forgetWorkbooks() // 700 KB used

	res, body := ada.postFile(path+"/dataset", "3.xlsx", workSized(t, 4, 850)) // fits once the 2 Parts go
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, nil)
	if got := ada.perkara(path + "/view/"); len(got) == 0 {
		t.Fatal("the Replace left no rows")
	}
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want this Dashboard's one Part and the other Dashboard's", n)
	}
	ada.forgetWorkbooks() // 400 + 850 KB used

	res, body = ada.postFile(path+"/dataset", "4.xlsx", workSized(t, 5, 800), adding) // 1250 + 800 + its Workbook, nothing freed
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	res, body = ada.postFile(path+"/dataset", "4.xlsx", workSized(t, 5, 800)) // 1250 + 800 - 850 + its Workbook
	expectStatus(t, res, body, http.StatusSeeOther, "")
}

// A Dataset from before Parts still counts in the total, but the confirm that deletes it counts it free.
func TestEarlierDatasetsFromBeforePartsCountInStorageAndCountFreeOnTheConfirmThatDeletesThem(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "2")
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "1.xlsx", workSized(t, 1, 150))
	ada.forgetWorkbooks()
	if _, err := ada.db.Exec(`INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, part_date, date_source, legacy)
		SELECT dashboard_id, 'dataset-earlier-before-parts', 'old.xlsx', 1700 * 1024, 1, 'earlier', current_date, 'before-parts', true FROM dataset_parts`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ada.storageDir, "dataset-earlier-before-parts"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, body := ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "1.8 MB of 2 MB") // the Part and the earlier Dataset: 150 + 1700 KB

	ada.addFile(path, "2.xlsx", workSized(t, 2, 150)) // 1850 + 150 KB would be over, were the earlier Dataset not deleted
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want the 2 Parts and no earlier Dataset", n)
	}
}

func TestPartsGoNinetyDaysAfterTheLastPeriodButTheAdminsStay(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", shopReport(t, "Beras", 120))
	ada.addFile(path, "oktober.xlsx", shopReport(t, "Gula", 80))
	demo := admin.createWork("Demo")
	admin.confirmFile(demo, "1.xlsx", shopReport(t, "Beras", 10))
	admin.addFile(demo, "2.xlsx", shopReport(t, "Gula", 10))
	if n := ada.datasetFiles(); n != 4 {
		t.Fatalf("%d stored files, want 4", n)
	}

	admin.ageAllPeriods(30 + 91)
	admin.prune()
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("after 91 days %d stored files, want the Admin's 2", n)
	}
	if line := admin.lastLogLine("Parts"); line != "Daily prune deleted 2 Parts" {
		t.Fatalf("System log line = %q", line)
	}
}

// storedKB is the Storage allowance's KB the files in the store take, as /lab shows it.
func (s *site) storedKB() string {
	s.t.Helper()
	return storageText(s.storedBytes()) + " of 1 GB"
}

// storedBytes is the size of everything in the store: the Parts and Workbooks the Lab counts, in these tests.
func (s *site) storedBytes() int64 {
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
	return n
}

// storageText is n bytes as the Lab says it: whole KB, or MB with one decimal.
func storageText(n int64) string {
	if n >= 1<<20 {
		return strconv.FormatFloat(math.Round(float64(n)/(1<<20)*10)/10, 'f', -1, 64) + " MB"
	}
	return fmt.Sprintf("%d KB", (n+1023)>>10)
}
