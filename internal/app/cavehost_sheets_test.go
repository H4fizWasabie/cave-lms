package app_test

import (
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// threeSheets is a Spreadsheet of three sheets: Sales, Staff (with salaries) and Cost Q1.
func threeSheets(t *testing.T) []byte {
	return testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}, {"Ink", 5}}},
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali", 3000}}},
		testutil.Sheet{Name: "Cost Q1", Rows: [][]any{{"Item", "Cost"}, {"Pen", 1.5}}},
	)
}

// tickSheet ticks the sheet called sheet of the Deployment called name; untickSheet unticks it.
func (s *site) tickSheet(name, sheet string) {
	s.t.Helper()
	res, body := s.post("/cavehost/"+name+"/tick", url.Values{"sheet": {sheet}})
	expectRedirect(s.t, res, body, "/cavehost/"+name)
}

func (s *site) untickSheet(name, sheet string) {
	s.t.Helper()
	res, body := s.post("/cavehost/"+name+"/untick", url.Values{"sheet": {sheet}})
	expectRedirect(s.t, res, body, "/cavehost/"+name)
}

// sheetJSON is the status of the Deployment's sheets/<sheet name>.json, the sheet's name URL-encoded.
func (s *site) sheetJSON(link, sheet string) int {
	s.t.Helper()
	res, _ := s.get(link + "sheets/" + url.PathEscape(sheet) + ".json")
	return res.StatusCode
}

// xlsxFiles lists the fetched Spreadsheets the Disk store holds.
func xlsxFiles(s *site) []string {
	files, _ := filepath.Glob(filepath.Join(s.storageDir, "cavehost-xlsx-*"))
	return files
}

func TestASheetTickedAfterTheFirstConfirmFetchesTheSpreadsheetOnceUntilItIsConfirmedOrUnticked(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	if files := xlsxFiles(ada); len(files) != 0 {
		t.Fatalf("the store keeps the fetched Spreadsheet %v with nothing waiting to be confirmed", files)
	}
	asked := len(google.requests())

	// Every view of the preview, and the confirm itself, read the file kept after the first fetch.
	ada.tickSheet("jualan-ada", "Staff")
	for range 3 {
		res, body := ada.get("/cavehost/jualan-ada")
		expectStatus(t, res, body, http.StatusOK, "Check Staff before your page reads it")
	}
	if got := len(google.requests()) - asked; got != 1 {
		t.Fatalf("Google was asked %d times for three views of a ticked sheet, want 1", got)
	}
	if files := xlsxFiles(ada); len(files) != 1 {
		t.Fatalf("the store holds %v while a sheet waits, want the fetched Spreadsheet", files)
	}
	ada.confirmSheet("jualan-ada", nil)
	if got := len(google.requests()) - asked; got != 1 {
		t.Errorf("confirming asked Google again: %d requests since ticking, want 1", got)
	}
	if files := xlsxFiles(ada); len(files) != 0 {
		t.Errorf("the store holds %v once the sheet is confirmed, want the fetched Spreadsheet gone", files)
	}

	// Unticking a sheet that waits drops it too, and ticking again fetches afresh.
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.get("/cavehost/jualan-ada")
	ada.get("/cavehost/jualan-ada")
	if got := len(google.requests()) - asked; got != 2 {
		t.Errorf("Google was asked %d times after ticking a second sheet, want 2 in all", got)
	}
	ada.untickSheet("jualan-ada", "Cost Q1")
	if files := xlsxFiles(ada); len(files) != 0 {
		t.Errorf("the store holds %v after the waiting sheet was unticked, want the fetched Spreadsheet gone", files)
	}
}

func TestEverySheetIsListedByNameAndOnlyTheFirstStartsTicked(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")

	_, body = ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"Sales", "Staff", "Cost Q1"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not list the sheet %q: %s", want, body)
		}
	}
	// Only Sales has the Untick button; the other two can be ticked.
	if strings.Count(body, ">Untick<") != 1 || strings.Count(body, ">Tick<") != 2 {
		t.Errorf("the page does not show Sales ticked and the other two not: %s", body)
	}

	ada.confirmSheet("jualan-ada", nil)
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()
	if got := visitor.sheetJSON(link, "Sales"); got != http.StatusOK {
		t.Errorf("sheets/Sales.json = %d, want 200", got)
	}
	for _, sheet := range []string{"Staff", "Cost Q1"} {
		if got := visitor.sheetJSON(link, sheet); got != http.StatusNotFound {
			t.Errorf("the unticked sheet %q is served: %d, want 404", sheet, got)
		}
	}
}

func TestTickingASheetShowsItsPreviewToConfirmAndItIsServedAtItsOwnLinkWhileDataJSONStaysTheFirstTickedSheet(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()
	firstRows := visitor.dataRowsOf(link)
	want := []map[string]any{{"Item": "Pen", "Qty": 3.0, "Part date": partDate()}, {"Item": "Ink", "Qty": 5.0, "Part date": partDate()}}
	if !reflect.DeepEqual(firstRows, want) {
		t.Fatalf("data.json = %v, want the Sales rows %v", firstRows, want)
	}

	ada.tickSheet("jualan-ada", "Cost Q1")
	res, body := ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "Check Cost Q1 before your page reads it")
	expectStatus(t, res, body, http.StatusOK, "Cost")
	// Until it is confirmed nothing of it is online.
	if got := visitor.sheetJSON(link, "Cost Q1"); got != http.StatusNotFound {
		t.Errorf("an unconfirmed sheet is served: %d, want 404", got)
	}

	ada.confirmSheet("jualan-ada", nil)
	res, body = visitor.get(link + "sheets/Cost%20Q1.json")
	expectStatus(t, res, body, http.StatusOK, `"Cost": 1.5`)
	if res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("a sheet's JSON headers = %v, want CORS and JSON", res.Header)
	}
	if got := visitor.dataRowsOf(link); !reflect.DeepEqual(got, firstRows) {
		t.Errorf("data.json = %v after ticking a later sheet, want still the first sheet's %v", got, firstRows)
	}

	// Unticking takes its link offline at once; unticking the first sheet makes data.json the next ticked one.
	ada.untickSheet("jualan-ada", "Cost Q1")
	if got := visitor.sheetJSON(link, "Cost Q1"); got != http.StatusNotFound {
		t.Errorf("an unticked sheet is still served: %d, want 404", got)
	}
	ada.tickSheet("jualan-ada", "Staff")
	ada.confirmSheet("jualan-ada", nil)
	ada.untickSheet("jualan-ada", "Sales")
	if got := visitor.dataRowsOf(link); len(got) != 1 || got[0]["Name"] != "Ali" {
		t.Errorf("data.json after unticking the first sheet = %v, want the Staff rows", got)
	}
	// Ticking an unticked sheet again asks for its confirmation again.
	ada.tickSheet("jualan-ada", "Sales")
	if _, body := ada.get("/cavehost/jualan-ada"); !strings.Contains(body, "Check Sales before your page reads it") {
		t.Errorf("ticking a sheet again shows no preview: %s", body)
	}
}

func TestDataJSONStaysTheSheetConfirmedFirstWhateverIsConfirmedLaterUntilItIsUnticked(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil) // Sales
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.confirmSheet("jualan-ada", nil)
	ada.untickSheet("jualan-ada", "Sales")
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()
	if got := visitor.dataRowsOf(link); len(got) != 1 || got[0]["Cost"] != 1.5 {
		t.Fatalf("data.json with only Cost Q1 ticked = %v, want its rows", got)
	}

	// Sales is first in the list, but it is confirmed after Cost Q1, so the live page's data does not move.
	ada.tickSheet("jualan-ada", "Sales")
	ada.confirmSheet("jualan-ada", nil)
	if got := visitor.dataRowsOf(link); len(got) != 1 || got[0]["Cost"] != 1.5 {
		t.Errorf("data.json after confirming an earlier sheet later = %v, want still the Cost Q1 rows", got)
	}
	if got := visitor.sheetJSON(link, "Sales"); got != http.StatusOK {
		t.Errorf("sheets/Sales.json = %d, want 200", got)
	}

	// It changes when that sheet is unticked: to the one confirmed next.
	ada.untickSheet("jualan-ada", "Cost Q1")
	if got := visitor.dataRowsOf(link); len(got) != 2 || got[0]["Item"] != "Pen" {
		t.Errorf("data.json after unticking Cost Q1 = %v, want the Sales rows", got)
	}
}

func TestEachSyncReadsEveryTickedSheetWithItsOwnRecipeAndANewSheetAppearsUnticked(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Kedai Runcit Ali"}, {"Item", "Qty"}, {"Pen", 3}}}, // its header is in row 2
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali", 3000}}},
	))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Staff")
	ada.confirmSheet("jualan-ada", nil)
	link := ada.caveHostLink("jualan-ada")

	google.swap(testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Kedai Runcit Ali"}, {"Item", "Qty"}, {"Pen", 4}, {"Cap", 9}}},
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali", 3100}, {"Siti", 2800}}},
		testutil.Sheet{Name: "Cost", Rows: [][]any{{"Item", "Cost"}, {"Pen", 1}}}, // added to the Spreadsheet later
	))
	res, body := ada.syncNow("jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")

	visitor := ada.second()
	day := partDate()
	wantSales := []map[string]any{{"Item": "Pen", "Qty": 4.0, "Part date": day}, {"Item": "Cap", "Qty": 9.0, "Part date": day}}
	if got := visitor.dataRowsOf(link); !reflect.DeepEqual(got, wantSales) {
		t.Errorf("data.json after Sync = %v, want %v", got, wantSales)
	}
	res, body = visitor.get(link + "sheets/Staff.json")
	expectStatus(t, res, body, http.StatusOK, `"Siti"`)
	if got := visitor.sheetJSON(link, "Cost"); got != http.StatusNotFound {
		t.Errorf("a sheet added later is served before it is ticked: %d, want 404", got)
	}
	// It is listed, unticked, with the others.
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Cost") || strings.Count(body, ">Untick<") != 2 || strings.Count(body, ">Tick<") != 1 {
		t.Errorf("the page after Sync does not list the new sheet unticked: %s", body)
	}
}

// chatbotPrompt is the text of the prompt field on a CaveHost Deployment's page, as the browser reads it; "" when the page has none.
func chatbotPrompt(body string) string {
	_, rest, found := strings.Cut(body, `<textarea id="chatbot"`)
	if !found {
		return ""
	}
	_, rest, _ = strings.Cut(rest, ">")
	rest, _, _ = strings.Cut(rest, "</textarea>")
	return html.UnescapeString(rest)
}

// promptWorkbook is a Spreadsheet whose rows have values that appear nowhere else, so a prompt that leaks one is caught.
func promptWorkbook(t *testing.T) []byte {
	day := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	return testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty", "Sold"}, {"Pensil-Rahsia", 7351, day}, {"Dakwat-Biru", 8842, day}}},
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali-Rahsia", 3000}}},
		testutil.Sheet{Name: "Cost Q1", Rows: [][]any{{"Item", "Cost"}, {"Pensil-Rahsia", 1.5}}},
	)
}

func TestThePageGivesTheChatbotAPromptWithEveryConfirmedSheetsFileColumnsAndTypesButNoRows(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, promptWorkbook(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	if _, body := ada.get("/cavehost/jualan-ada"); strings.Contains(body, "./data.json") {
		t.Fatalf("the page gives the chatbot a prompt before any sheet is confirmed: %s", body)
	}
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.confirmSheet("jualan-ada", nil)

	_, body := ada.get("/cavehost/jualan-ada")
	prompt := chatbotPrompt(body)
	for _, want := range []string{
		"Write one HTML file for a dashboard.", "must not contain any of my data", "loads its data when it opens",
		`./data.json has the same rows as my sheet "Sales"`,
		`./sheets/Sales.json: "Item" (text), "Qty" (number), "Sold" (date, YYYY-MM-DD)`,
		`./sheets/Cost%20Q1.json: "Item" (text), "Cost" (number)`,
		`"Part date"`,
		"cdn.jsdelivr.net", "cdnjs.cloudflare.com", "unpkg.com", "fonts.googleapis.com", "fonts.gstatic.com",
		"Chart.js", "Do not fetch anything else", "No data yet", "every hour", "under 5 MB", "No forms that send", "no logins",
		"What I want the dashboard to show: [for example,",
		"CAVELMS already serves these JSON files", "I only upload the HTML",
		"one code block in your answer, not as a file",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, prompt)
		}
	}
	for _, leak := range []string{"Pensil-Rahsia", "Dakwat-Biru", "7351", "8842", "2026-03-09", "Staff", "Ali-Rahsia"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("the prompt has %q, a row value or an unticked sheet:\n%s", leak, prompt)
		}
	}
	unescaped := html.UnescapeString(body)
	for _, want := range []string{"Prompt for your AI chatbot", `data-copy="chatbot"`, `src="/static/forms.js`,
		"You don't need to give your chatbot your Spreadsheet. This prompt is enough, and your rows stay in CAVELMS."} {
		if !strings.Contains(unescaped, want) {
			t.Errorf("the page lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "./sheets/Staff.json") {
		t.Errorf("the page lists the unticked Staff sheet: %s", body)
	}
}

func TestThePromptNamesTheSheetConfirmedFirstAsDataJSONWhateverIsConfirmedLater(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, promptWorkbook(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil) // Sales
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.confirmSheet("jualan-ada", nil)
	ada.untickSheet("jualan-ada", "Sales")
	ada.tickSheet("jualan-ada", "Sales")
	ada.confirmSheet("jualan-ada", nil) // Sales is first in the list but confirmed after Cost Q1

	_, body := ada.get("/cavehost/jualan-ada")
	if prompt := chatbotPrompt(body); !strings.Contains(prompt, `./data.json has the same rows as my sheet "Cost Q1"`) {
		t.Errorf("the prompt does not name Cost Q1 as data.json's sheet:\n%s", prompt)
	}
}

func TestThePromptReadsInMalay(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, promptWorkbook(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)

	_, body := ada.getAsking("/cavehost/jualan-ada", "ms")
	prompt := chatbotPrompt(body)
	for _, want := range []string{
		"Tulis satu fail HTML untuk papan pemuka.", "tidak boleh mengandungi sebarang data saya",
		`./data.json mempunyai baris yang sama dengan helaian saya "Sales"`,
		`./sheets/Sales.json: "Item" (teks), "Qty" (nombor), "Sold" (tarikh, YYYY-MM-DD)`,
		"Belum ada data", "cdn.jsdelivr.net", "Apa yang saya mahu papan pemuka tunjukkan: [contohnya,",
		"CAVELMS sudah menyediakan fail JSON ini", "satu blok kod dalam jawapan anda, bukan sebagai fail",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the Malay prompt lacks %q:\n%s", want, prompt)
		}
	}
	unescaped := html.UnescapeString(body)
	for _, want := range []string{"Gesaan untuk chatbot AI anda", "Anda tidak perlu memberi Hamparan anda kepada chatbot anda. Gesaan ini sudah cukup, dan baris anda kekal dalam CAVELMS."} {
		if !strings.Contains(unescaped, want) {
			t.Errorf("the Malay page lacks %q: %s", want, body)
		}
	}
	if strings.Contains(prompt, "Write one HTML file") || strings.Contains(body, "Prompt for your AI chatbot") {
		t.Errorf("the Malay page still has English text: %s", body)
	}
}

func TestTheSheetListSaysWhatResetColumnsAndUntickingDoAndSyncReadsInMalay(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)

	res, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"You get a new preview at once to confirm", "your page keeps the last good rows and Sync skips this sheet",
		"Unticking takes this sheet offline at once and drops its Clean-up choices"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	res, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	for _, want := range []string{">Set semula lajur<", ">Segerakkan sekarang<", ">Penyegerakan<", "Penyegerakan melangkau helaian ini", "membuang pilihan Pembersihannya"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	if strings.Contains(body, "Reset columns") || strings.Contains(body, "Sync now") {
		t.Errorf("the Malay page still has English controls: %s", body)
	}
}

func TestSeveralSheetsReadInMalay(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Staff")

	res, body := ada.getAsking("/cavehost/jualan-ada", "ms")
	for _, want := range []string{"Helaian", ">Tandakan<", ">Nyahtanda<", "Semak Staff sebelum halaman anda membacanya"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	ada.confirmSheet("jualan-ada", nil)
	res, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	expectStatus(t, res, body, http.StatusOK, "./sheets/Staff.json")
	expectStatus(t, res, body, http.StatusOK, "Gesaan untuk chatbot AI anda")
}

func TestATickedSheetCleanUpCannotReadShowsWhyNotAskForAHeaderRow(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}}},
		testutil.Sheet{Name: "Notes", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}, {"Ink", 5, "reorder"}}},
	))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Notes")
	_, body := ada.get("/cavehost/jualan-ada")
	body = html.UnescapeString(body)
	if !strings.Contains(body, "Sheet Notes: Column C has data but no name in the header, row 1.") {
		t.Errorf("page = %s, want the sheet's own reason", body)
	}
	if strings.Contains(body, "Pick its header row below") {
		t.Errorf("page asks for a header row that cannot help: %s", body)
	}
}

func TestReadAgainFetchesTheSpreadsheetAfreshAndAPlainViewKeepsTheLastFetchedOne(t *testing.T) {
	t.Parallel()
	broken := testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}}},
		testutil.Sheet{Name: "Notes", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}, {"Ink", 5, "reorder"}}},
	)
	fixed := testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}}},
		testutil.Sheet{Name: "Notes", Rows: [][]any{{"Item", "Qty", "Note"}, {"Pen", 3, "ok"}, {"Ink", 5, "reorder"}}},
	)
	ada, google := caveHostLearner(t, broken)
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Notes")
	_, body := ada.get("/cavehost/jualan-ada")
	if !strings.Contains(html.UnescapeString(body), "Column C has data but no name") {
		t.Fatalf("page = %s, want the unreadable sheet's reason", body)
	}
	old := xlsxFiles(ada)
	asked := len(google.requests())

	// The Learner fixes the Spreadsheet: a plain view still reads the file kept, Read again reads the new one.
	google.swap(fixed)
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(html.UnescapeString(body), "Column C has data but no name") || len(google.requests()) != asked {
		t.Errorf("a plain view fetched again or changed: %d requests since, page = %s", len(google.requests())-asked, body)
	}
	res, body := ada.get("/cavehost/jualan-ada?header=")
	expectStatus(t, res, body, http.StatusOK, "Check Notes before your page reads it")
	if got := len(google.requests()) - asked; got != 1 {
		t.Errorf("Read again asked Google %d times, want 1", got)
	}
	if files := xlsxFiles(ada); len(files) != 1 || files[0] == old[0] {
		t.Errorf("the store holds %v after Read again, want the new file alone instead of %v", files, old)
	}

	// Confirming reads the file kept by Read again, without asking Google.
	ada.confirmSheet("jualan-ada", nil)
	if got := len(google.requests()) - asked; got != 1 {
		t.Errorf("confirming asked Google again: %d requests since the fix, want 1", got)
	}
	res, body = ada.second().get(ada.caveHostLink("jualan-ada") + "sheets/Notes.json")
	expectStatus(t, res, body, http.StatusOK, `"Note": "reorder"`)
}
