package app_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// createWork creates a Dashboard with template pre-chosen, as a Sample's Template is, and returns its path.
// The New Dashboard form makes a page Dashboard on General; its page is dropped, so it is an engine Dashboard until
// the engine goes (#376), and any other Template is set in the database.
func (s *site) createWork(template, name string) string {
	s.t.Helper()
	res, body := s.post("/lab/dashboards", url.Values{"name": {name}, "html": {myPage}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	loc := res.Header.Get("Location")
	m := dashboardPath.FindStringSubmatch(loc)
	if m == nil {
		s.t.Fatalf("created Work Dashboard redirects to %q", loc)
	}
	if _, err := s.db.Exec(`UPDATE dashboards SET template = $2 WHERE id = $1`, m[1], template); err != nil {
		s.t.Fatal(err)
	}
	s.dropPage(m[1])
	return loc
}

// shopReport is a Finance-like report as kept at work: two title rows and a blank row above the header.
func shopReport(t *testing.T, item string, amount float64) []byte {
	t.Helper()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, []string{"Kedai Runcit Ali"},
		[]any{"Laporan Oktober 2026"},
		[]any{},
		[]any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, item, "Dapur", "Keluar", amount},
		[]any{day, "Jualan", "Kedai", "Masuk", 900.0},
	)
}

// dashReport is shopReport with item's amount a dash, read as 0: a kind of change shopReport never needs, so
// its upload shows the preview.
func dashReport(t *testing.T, item string) []byte {
	t.Helper()
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	return testutil.XLSX(t, []string{"Kedai Runcit Ali"}, []any{}, []any{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"},
		[]any{day, item, "Dapur", "Keluar", "-"}, []any{day, "Jualan", "Kedai", "Masuk", 900.0})
}

// confirmWork confirms the pending Dataset of the Work Dashboard at path with Cave's guesses.
func (s *site) confirmWork(path string, choices url.Values) {
	s.t.Helper()
	res, body := s.post(path+"/dataset/confirm", choices)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
}

// rowsOf fetches the data the Dashboard page at page (ending in /) draws.
func (s *site) rowsOf(page string) string {
	s.t.Helper()
	res, body := s.get(page + s.dataOf(page))
	expectStatus(s.t, res, body, http.StatusOK, "")
	return body
}

var financeGuess = url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "category": {"Kategori"}, "type": {"Jenis"}, "description": {"Perkara"}}

// The Lab speaks of one kind of Dashboard (ADR 0015): no page names a kind, in either language.
func TestTheLabOffersOneNewDashboardAndNamesNoKind(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	ada.createWork("hr", "Staf")
	for _, lang := range []string{"en", "ms"} {
		ada.switchLanguage(lang, "/")
		res, body := ada.get("/lab")
		expectStatus(t, res, body, http.StatusOK, `href="/lab/new"`)
		for _, p := range []string{"/lab", "/lab/new", path, path + "/blocks", path + "/preview", path + "/hood", path + "/deploy"} {
			res, body := ada.get(p)
			expectStatus(t, res, body, http.StatusOK, `<html lang="`+lang+`">`)
			low := strings.ToLower(body)
			for _, gone := range []string{"kind=work", "practice dashboard", "work dashboard", "dashboard latihan", "dashboard kerja",
				"<li>practice</li>", "<li>work</li>", "<li>latihan</li>", "<li>kerja</li>"} {
				if strings.Contains(low, gone) {
					t.Errorf("%s in %s still names a kind of Dashboard (%s)", p, lang, gone)
				}
			}
		}
	}
}

func TestAWorkDashboardUploadIsCleanedPreviewedAndConfirmed(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")

	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	for _, want := range []string{
		"Skipped rows 1 to 3 above the header in row 4.",
		"laporan.xlsx", "Beras", "Jualan",
		`<option value="Tarikh" selected>Tarikh</option>`, `<option value="Jumlah" selected>Jumlah</option>`,
		`action="` + path + `/dataset/confirm"`,
	} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	// Pending: nothing to preview or deploy yet.
	res, body = ada.get(path + "/view/")
	expectStatus(t, res, body, http.StatusNotFound, "")

	ada.switchLanguage("ms", path)
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Baris 1 hingga 3 di atas tajuk lajur di baris 4 dilangkau.")
	ada.switchLanguage("en", path)

	ada.confirmWork(path, financeGuess)
	page := ada.rowsOf(path + "/view/")
	_, index := ada.get(path + "/view/")
	for _, want := range []string{`"group": "Kategori"`, `"vals": [`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the Work Dashboard page lacks %s:\n%s", want, index)
		}
	}
	if !strings.Contains(page, `"Perkara": "Beras"`) {
		t.Fatalf("data = %s", page)
	}
	res, body = ada.get(path + "/hood")
	expectStatus(t, res, body, http.StatusOK, "data.json")
}

func TestUntilConfirmTheDashboardAndDeploymentKeepTheOldDataset(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "september.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, financeGuess)
	link := ada.deploy(path)
	viewer := ada.second()

	res, body = ada.postFile(path+"/dataset", "oktober.xlsx", dashReport(t, "Gula"))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	for _, page := range []string{ada.rowsOf(path + "/view/"), viewer.rowsOf(link)} {
		if !strings.Contains(page, "Beras") || strings.Contains(page, "Gula") {
			t.Fatalf("before confirming, data = %s", page)
		}
	}
	ada.confirmWork(path, nil) // the blocks are already the Dashboard's
	for _, page := range []string{ada.rowsOf(path + "/view/"), viewer.rowsOf(link)} {
		if !strings.Contains(page, "Gula") || strings.Contains(page, "Beras") {
			t.Fatalf("after confirming, data = %s", page)
		}
	}
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want the current Dataset and the earlier one", n)
	}
}

func TestAReUploadDuringAConfirmTakesItsTurnAndLeavesACurrentDataset(t *testing.T) {
	for _, uploadFirst := range []bool{false, true} {
		t.Run(fmt.Sprint("upload first ", uploadFirst), func(t *testing.T) {
			_, ada := labLearner(t)
			path := ada.workReady()
			res, body := ada.postFile(path+"/dataset", "oktober.xlsx", dashReport(t, "Gula"))
			expectStatus(t, res, body, http.StatusSeeOther, "")

			// Holding the Learner's lock queues the confirm and the re-upload behind it: uploads and confirms take
			// turns, so neither can slip in under the Storage allowance or leave the Dashboard with no current Dataset.
			ctx := context.Background()
			tx, err := ada.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, `SELECT 1 FROM learners FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			waitForLockWaiters := func(n int) {
				for waiting := 0; waiting < n; {
					if err := ada.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
						WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
						t.Fatal(err)
					}
				}
			}
			confirmed, uploaded := make(chan *http.Response), make(chan *http.Response)
			confirm := func() {
				res, _ := ada.post(path+"/dataset/confirm", nil)
				confirmed <- res
			}
			upload := func() {
				res, _ := ada.postFile(path+"/dataset", "november.xlsx", dashReport(t, "Garam"))
				uploaded <- res
			}
			if uploadFirst { // the confirm then finds the Dataset it read replaced
				confirm, upload = upload, confirm
			}
			go confirm()
			waitForLockWaiters(1)
			go upload()
			waitForLockWaiters(2)
			tx.Rollback()

			// Each lands back on the Dashboard: a confirm on its preview, or on the new upload's preview when the
			// re-upload replaced the Dataset it was confirming.
			for what, done := range map[string]chan *http.Response{"confirm": confirmed, "re-upload": uploaded} {
				if res := <-done; res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), path) {
					t.Fatalf("the %s = %d to %q, want back to the Dashboard", what, res.StatusCode, res.Header.Get("Location"))
				}
			}
			// The routine re-upload goes live on its own in its turn, so any of the three may be current; none is not.
			if page := ada.rowsOf(path + "/view/"); !strings.Contains(page, "Gula") && !strings.Contains(page, "Garam") && !strings.Contains(page, "Beras") {
				t.Fatalf("the current Dataset = %s", page)
			}
		})
	}
}

// months is a workbook of one Finance-like sheet per name, with a notes sheet of other columns when notes.
func months(t *testing.T, notes bool, names ...string) []byte {
	t.Helper()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	var sheets []testutil.Sheet
	for _, name := range names {
		sheets = append(sheets, testutil.Sheet{Name: name, Rows: [][]any{
			{"Tarikh", "Perkara", "Jumlah"}, {day, "Beras " + name, 12.0},
		}})
	}
	if notes {
		sheets = append(sheets, testutil.Sheet{Name: "Nota", Rows: [][]any{{"Kumpulan", "Perkara"}, {"Dapur", "Beras"}}})
	}
	return testutil.Workbook(t, sheets...)
}

func TestAWorkUploadCombinesSheetsAndRemembersThePickedSheet(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	upload := func(file []byte, want string, fields ...url.Values) string {
		t.Helper()
		res, body := ada.postFile(path+"/dataset", "buku.xlsx", file, fields...)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusOK, "Sheets in this Dataset: "+want+".")
		return body
	}

	body := upload(months(t, true, "Jan", "Feb"), "Jan, Feb")
	for _, want := range []string{
		`name="sheet" value="Jan" checked`, `name="combine" value="Feb" checked`, `name="shown" value="Jan"`,
		`name="sheet" value="Nota"`, "Its columns differ from the picked sheet", "1 row",
		"Combined 2 sheets into one Dataset",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("the preview lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `name="combine" value="Nota"`) {
		t.Fatal("a sheet of other columns can be ticked")
	}

	// Read again with Feb unticked: Jan alone, remembered once confirmed.
	upload(months(t, true, "Jan", "Feb"), "Jan", url.Values{"sheet": {"Jan"}, "shown": {"Jan"}, "tickable": {"1"}})
	ada.confirmWork(path, url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}})
	ada.uploadLive(path, months(t, false, "Jan", "Feb", "Mac"), "Beras Jan", "Beras Feb")
	// Combining again: next time a new sheet with the same columns joins by itself.
	upload(months(t, false, "Jan", "Feb", "Mac"), "Jan, Feb", url.Values{"sheet": {"Jan"}, "shown": {"Jan"}, "tickable": {"1"}, "combine": {"Feb"}})
	ada.confirmWork(path, nil)
	ada.uploadLive(path, months(t, false, "Jan", "Feb", "Mac"), "Beras Mac", "")
	// The remembered sheet is gone: the first readable one is picked.
	ada.uploadLive(path, months(t, false, "Apr", "Mei"), "Beras Mei", "")
}

// uploadLive uploads a routine file to the Work Dashboard at path, which goes live with no preview, and checks
// its data has want and, unless empty, lacks not.
func (s *site) uploadLive(path string, file []byte, want, not string) {
	s.t.Helper()
	res, body := s.postFile(path+"/dataset", "buku.xlsx", file)
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		s.t.Fatalf("a routine upload redirects to %q", loc)
	}
	if page := s.rowsOf(path + "/view/"); !strings.Contains(page, want) || (not != "" && strings.Contains(page, not)) {
		s.t.Fatalf("data = %s, want %s and not %q", page, want, not)
	}
}

func TestAFailedOrAbandonedUploadLeavesTheRememberedSettings(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	janAlone := url.Values{"sheet": {"Jan"}, "shown": {"Jan"}, "tickable": {"1"}}
	both := func() {
		t.Helper()
		res, body := ada.postFile(path+"/dataset", "buku.xlsx", months(t, false, "Jan", "Feb"))
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusOK, "Sheets in this Dataset: Jan, Feb.")
	}

	res, body := ada.postFile(path+"/dataset", "buku.xlsx", []byte("Tarikh,Jumlah\n"), janAlone)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	both()
	res, body = ada.postFile(path+"/dataset", "buku.xlsx", months(t, false, "Jan", "Feb"), janAlone)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	both() // the Jan-alone preview is abandoned
}

func TestThePreviewAsksTheMonthOfADaysAcrossGrid(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	header := []any{"Staff"}
	for d := 1; d <= 31; d++ {
		header = append(header, d)
	}
	ali := []any{"Ali", "P", "M"}
	file := testutil.Workbook(t, testutil.Sheet{Name: "Jadual", Rows: [][]any{{"Jadual Tugas Oktober 2026"}, header, ali}})
	upload := func(want string, fields ...url.Values) string {
		t.Helper()
		res, body := ada.postFile(path+"/dataset", "jadual.xlsx", file, fields...)
		expectStatus(t, res, body, http.StatusSeeOther, "")
		res, body = ada.get(path)
		expectStatus(t, res, body, http.StatusOK, want)
		return body
	}

	body := upload("Turned the days across the columns into 2 rows, one per staff per day, dated 10/2026.")
	for _, want := range []string{`name="month" value="10"`, `name="year" value="2026"`, "2026-10-02"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the preview lacks %s:\n%s", want, body)
		}
	}
	body = upload("dated 1/2027.", url.Values{"month": {"1"}, "year": {"2027"}})
	if !strings.Contains(body, "2027-01-02") {
		t.Fatalf("the Learner's month is not used:\n%s", body)
	}
	upload(fmt.Sprintf("dated 1/%d.", time.Now().Year()), url.Values{"month": {"1"}, "year": {"99999"}})
}

func TestAWorkDashboardOnGeneralNeedsNoChoices(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	res, body := ada.postFile(path+"/dataset", "senarai.xlsx", testutil.XLSX(t, []string{"Hari", "Nota"}, []any{day, "Buka"}))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.post(path+"/dataset/confirm", url.Values{"template": {"general"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "General Template")
	_, index := ada.get(path + "/view/")
	if !strings.Contains(index, `"cols": [`) {
		t.Fatalf("General built no table:\n%s", index)
	}
}

func TestAnUnreadableWorkUploadGivesOneMessage(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", []byte("Tarikh,Jumlah\n05/11/2026,RM5\n"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "We could not read laporan.xlsx")
	if n := strings.Count(body, "CSV file stores everything"); n != 1 {
		t.Fatalf("the message shows %d times: %s", n, body)
	}
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after an unreadable upload", n)
	}
}

func TestTheLearnerCanUploadAgainNamingATwoRowHeader(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.Merge(t, testutil.XLSX(t, []string{"Jualan Oktober 2026", "Kedai Ali"},
		[]any{"Tarikh", "Jualan", nil, "Staf"},
		[]any{nil, "Tunai", "Kad"},
		[]any{day, 120, 80, "Ali"},
	), "B2:C2")
	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, `name="header"`)
	expectStatus(t, res, body, http.StatusOK, `name="two-row"`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("header", "2")
	mw.WriteField("two-row", "on")
	fw, _ := mw.CreateFormFile("file", "jualan.xlsx")
	fw.Write(file)
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, ada.server.URL+path+"/dataset", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, body = ada.do(req)
	expectStatus(t, res, body, http.StatusSeeOther, "")

	res, body = ada.get(path)
	for _, want := range []string{
		"Read row 2 as the header, as you chose.", "Joined header rows 2 and 3 into one name per column.",
		"Jualan Tunai", "Jualan Kad",
		`name="two-row" value="on" checked`, // remembered for the next upload; the header row is not
	} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}

func TestTheStorageAllowanceCountsConfirmedWorkDatasetsBySavedSize(t *testing.T) {
	_, ada, _ := storageSite(t, "1024")
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 GB") // pending does not count

	ada.confirmWork(path, financeGuess)
	entries, err := os.ReadDir(ada.storageDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stored files %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, fmt.Sprintf("%d KB of 1 GB", (info.Size()+1023)>>10))
}

func TestAConfirmOverTheStorageAllowanceIsRefused(t *testing.T) {
	_, ada, _ := storageSite(t, "2")
	first := ada.createWork("finance", "Satu")
	ada.confirmFile(first, "one.xlsx", bigFinance(t, 1))
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "two.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 1`); err != nil { // the upload fitted; the allowance shrinks
		t.Fatal(err)
	}
	res, body = ada.post(path+"/dataset/confirm", financeGuess)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
}

func TestPendingWorkDatasetsCountAgainstTheStorageAllowance(t *testing.T) {
	_, ada, _ := storageSite(t, "1") // 1 MB, and each upload saves as about 600 KB
	first := ada.createWork("finance", "Satu")
	res, body := ada.postFile(first+"/dataset", "one.xlsx", bigFinance(t, 1))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	path := ada.createWork("finance", "Kedai")
	res, body = ada.postFile(path+"/dataset", "two.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want only the first pending Dataset", n)
	}
	// A re-upload replaces the Dashboard's own pending Dataset, so that one does not count.
	res, body = ada.postFile(first+"/dataset", "again.xlsx", bigFinance(t, 3))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get("/lab")
	expectStatus(t, res, body, http.StatusOK, "0 KB of 1 MB") // the Storage figure still counts confirmed Datasets only
}

func TestUnconfirmedWorkDatasetsArePruned(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	admin.prune()
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("a fresh pending Dataset was pruned: %d stored files", n)
	}
	if _, err := admin.db.ExecContext(context.Background(), `UPDATE datasets SET uploaded_at = uploaded_at - interval '2 days'`); err != nil {
		t.Fatal(err)
	}
	admin.prune()
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after pruning an old pending Dataset", n)
	}
	res, body = ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "Upload your Dataset")
}

func TestDeletingAWorkDashboardDeletesItsDatasets(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("finance", "Kedai")
	res, body := ada.postFile(path+"/dataset", "a.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, financeGuess)
	res, body = ada.postFile(path+"/dataset", "b.xlsx", shopReport(t, "Gula", 80))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 2 {
		t.Fatalf("%d stored files, want current and pending", n)
	}
	res, body = ada.post(path+"/delete", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.storedFiles(); n != 0 {
		t.Fatalf("%d stored files after deleting the Dashboard", n)
	}
}

func TestTheSalesAndRosterTemplatesOpenAsTheirSamplesShow(t *testing.T) {
	_, ada := labLearner(t)
	_, demo := ada.get("/lab/demo/roster/")
	if !strings.Contains(demo, `"by": "day"`) {
		t.Fatalf("the Roster demo is not drawn by day:\n%s", demo)
	}

	path := ada.createWork("sales", "Kafe")
	_, sample := ada.get("/lab/samples/sales.xlsx")
	res, body := ada.postFile(path+"/dataset", "jualan.xlsx", []byte(sample))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	ada.confirmWork(path, url.Values{"date": {"Tarikh"}, "amount": {"Jumlah"}, "item": {"Produk"}, "category": {"Kategori"},
		"staff": {"Staf"}, "payment": {"Kaedah bayaran"}, "quantity": {"Kuantiti"}})
	_, index := ada.get(path + "/view/")
	for _, want := range []string{"What sells, and when?", `"label": "Revenue by payment method"`, `"group": "Kaedah bayaran"`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the Sales Dashboard lacks %s:\n%s", want, index)
		}
	}
}

// stockFile is a stock list General reads by column roles: a unique SKU, a category, a unit price and a quantity.
func stockFile(t *testing.T) []byte {
	t.Helper()
	var rows [][]any
	for i := range 12 {
		rows = append(rows, []any{fmt.Sprintf("SKU-%02d", i), []string{"Makanan", "Minuman", "Snek"}[i%3], float64(2 + i%4), float64(10 + i)})
	}
	return testutil.XLSX(t, []string{"SKU", "Kategori", "Unit Price", "Kuantiti"}, rows...)
}

func TestThePreviewSaysWhichColumnsGeneralLeavesOutOfChartsOrAverages(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.createWork("general", "Stor")
	res, body := ada.postFile(path+"/dataset", "stok.xlsx", stockFile(t))
	expectStatus(t, res, body, http.StatusSeeOther, "")
	res, body = ada.get(path)
	for _, want := range []string{"How Cave reads your columns", "SKU: an ID or code, so it&#39;s only in the table.",
		"Unit Price: a price, rate, balance or level, so it&#39;s averaged, not added up."} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body = ada.getAsking(path, "ms")
	for _, want := range []string{"Cara Cave membaca lajur anda", "SKU: ID atau kod, jadi ia hanya dalam jadual.",
		"Unit Price: harga, kadar, baki atau paras, jadi puratanya diambil, bukan dijumlahkan."} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
