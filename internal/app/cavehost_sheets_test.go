package app_test

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

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

func TestThePageGivesTheChatbotASentenceListingDataJSONAndEveryTickedSheetsLink(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	if _, body := ada.get("/cavehost/jualan-ada"); strings.Contains(body, "./data.json") {
		t.Fatalf("the page gives the chatbot a sentence before any sheet is confirmed: %s", body)
	}
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.confirmSheet("jualan-ada", nil)

	_, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"./data.json", "./sheets/Sales.json", "./sheets/Cost%20Q1.json"} {
		if !strings.Contains(body, want) {
			t.Errorf("the chatbot sentence lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "./sheets/Staff.json") {
		t.Errorf("the chatbot sentence lists the unticked Staff sheet: %s", body)
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
	expectStatus(t, res, body, http.StatusOK, "Beritahu chatbot AI anda")
}
