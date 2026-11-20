package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// rowsAt is the rows of the JSON file at address.
func (s *site) rowsAt(address string) []map[string]any {
	s.t.Helper()
	res, body := s.get(address)
	expectStatus(s.t, res, body, http.StatusOK, "")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		s.t.Fatalf("%s is not JSON: %v\n%s", address, err, body)
	}
	return rows
}

// salesRows is the rows of salesSheet as the page reads them.
func salesRows() []map[string]any {
	day := partDate()
	return []map[string]any{{"Item": "Pen", "Qty": 3.0, "Part date": day}, {"Item": "Ink", "Qty": 5.0, "Part date": day}, {"Item": "Pad", "Qty": 7.0, "Part date": day}}
}

// salesWith is a Spreadsheet of the sheet Sales with these rows under its header.
func salesWith(t *testing.T, header []any, rows ...[]any) []byte {
	return testutil.Workbook(t, testutil.Sheet{Name: "Sales", Rows: append([][]any{header}, rows...)})
}

func TestASyncThatCannotReadASheetAsConfirmedFailsWithTheReasonAndKeepsItsLastRows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, want string
		file       func(t *testing.T) []byte
	}{
		{"a confirmed column is missing", "Sales: The sheet lacks columns it had when you confirmed it. Missing: Qty.", func(t *testing.T) []byte {
			return salesWith(t, []any{"Item", "Price"}, []any{"Pen", 2})
		}},
		{"a confirmed column is only renamed in case and spaces", "", func(t *testing.T) []byte {
			return salesWith(t, []any{"item", " QTY "}, []any{"Pen", 2})
		}},
		{"more than half of a column's cells would be emptied", "Sales: More than half of the cells in the column Qty (3 of 4) would be left empty", func(t *testing.T) []byte {
			return salesWith(t, []any{"Item", "Qty"}, []any{"Pen", "n/a"}, []any{"Ink", "none"}, []any{"Pad", "x"}, []any{"Cap", 9})
		}},
		{"the sheet is renamed", "Sales: The sheet Sales is not in the Spreadsheet any more", func(t *testing.T) []byte {
			return testutil.Workbook(t, testutil.Sheet{Name: "Jualan", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}}})
		}},
		{"the sheet is removed", "Sales: The sheet Sales is not in the Spreadsheet any more", func(t *testing.T) []byte {
			return testutil.Workbook(t, testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali", 3000}}})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ada, google := caveHostLearner(t, salesSheet(t))
			link := onlineSales(t, ada, "jualan-ada")

			google.swap(c.file(t))
			res, body := ada.syncNow("jualan-ada")
			expectRedirect(t, res, body, "/cavehost/jualan-ada")

			_, body = ada.get("/cavehost/jualan-ada")
			if c.want == "" {
				if !strings.Contains(body, "It worked") {
					t.Errorf("a column that differs only in case and spaces fails the Sync: %s", body)
				}
				return
			}
			for _, want := range []string{"It failed", c.want} {
				if !strings.Contains(body, want) {
					t.Errorf("the page after the failed Sync lacks %q: %s", want, body)
				}
			}
			if got := ada.second().rowsAt(link + "data.json"); !reflect.DeepEqual(got, salesRows()) {
				t.Errorf("data.json after the failed Sync = %v, want the last good rows %v", got, salesRows())
			}
		})
	}
}

func TestAColumnWithLessThanHalfOfItsCellsEmptiedStillSyncsAndListsThem(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")

	google.swap(salesWith(t, []any{"Item", "Qty"}, []any{"Pen", "n/a"}, []any{"Ink", 5}, []any{"Pad", 7}, []any{"Cap", 9}))
	ada.syncNow("jualan-ada")

	day := partDate()
	want := []map[string]any{{"Item": "Pen", "Qty": nil, "Part date": day}, {"Item": "Ink", "Qty": 5.0, "Part date": day},
		{"Item": "Pad", "Qty": 7.0, "Part date": day}, {"Item": "Cap", "Qty": 9.0, "Part date": day}}
	if got := ada.second().rowsAt(link + "data.json"); !reflect.DeepEqual(got, want) {
		t.Errorf("data.json = %v, want the Sync to have read the rows, one cell empty", got)
	}
	_, body := ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "It worked") {
		t.Errorf("the page does not say it worked: %s", body)
	}
}

func TestASpreadsheetMadePrivateOrDeletedFailsTheSyncAndKeepsTheLastRows(t *testing.T) {
	t.Parallel()
	for name, status := range map[string]int{"made private": http.StatusForbidden, "deleted": http.StatusNotFound} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ada, google := caveHostLearner(t, salesSheet(t))
			link := onlineSales(t, ada, "jualan-ada")

			google.answerWith(status)
			ada.syncNow("jualan-ada")

			_, body := ada.get("/cavehost/jualan-ada")
			for _, want := range []string{"It failed", "Cave cannot open this Spreadsheet"} {
				if !strings.Contains(body, want) {
					t.Errorf("the page lacks %q: %s", want, body)
				}
			}
			if got := ada.second().rowsAt(link + "data.json"); !reflect.DeepEqual(got, salesRows()) {
				t.Errorf("data.json = %v, want the last good rows", got)
			}
		})
	}
}

func TestASpreadsheetOverTheSizeCapFailsTheSyncAndKeepsTheLastRows(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")

	google.swap(append([]byte("PK"), make([]byte, 50<<20)...)) // 50 MB and 2 bytes: one over the cap
	ada.syncNow("jualan-ada")

	_, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"It failed", "over 50 MB"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q: %s", want, body)
		}
	}
	if got := ada.second().rowsAt(link + "data.json"); !reflect.DeepEqual(got, salesRows()) {
		t.Errorf("data.json = %v, want the last good rows", got)
	}
}

func TestASheetOverTheRowLimitFailsTheSyncAndKeepsTheLastRows(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 50,001-row workbook")
	}
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")

	rows := make([][]any, 50001)
	for i := range rows {
		rows[i] = []any{"Pen", i}
	}
	google.swap(salesWith(t, []any{"Item", "Qty"}, rows...))
	ada.syncNow("jualan-ada")

	_, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"It failed", "Sales: This sheet has 50,001 rows. CaveHost reads up to 50,000 rows"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q: %s", want, body)
		}
	}
	if got := ada.second().rowsAt(link + "data.json"); !reflect.DeepEqual(got, salesRows()) {
		t.Errorf("data.json has %d rows, want the last good rows", len(got))
	}
}

func TestAFailingSheetKeepsItsRowsWhileTheOtherSheetsOfTheSpreadsheetStillSync(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.tickSheet("jualan-ada", "Cost Q1")
	ada.confirmSheet("jualan-ada", nil)
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()
	costBefore := visitor.rowsAt(link + "sheets/Cost%20Q1.json")

	// Sales loses its Qty column, while Cost Q1 gets a row.
	google.swap(testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Price"}, {"Pen", 2}}},
		testutil.Sheet{Name: "Staff", Rows: [][]any{{"Name", "Salary"}, {"Ali", 3000}}},
		testutil.Sheet{Name: "Cost Q1", Rows: [][]any{{"Item", "Cost"}, {"Pen", 1.5}, {"Ink", 4}}},
	))
	ada.syncNow("jualan-ada")

	day := partDate()
	if got, want := visitor.rowsAt(link+"data.json"), []map[string]any{{"Item": "Pen", "Qty": 3.0, "Part date": day}, {"Item": "Ink", "Qty": 5.0, "Part date": day}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the failing sheet's data.json = %v, want its last good rows", got)
	}
	got, want := visitor.rowsAt(link+"sheets/Cost%20Q1.json"), []map[string]any{{"Item": "Pen", "Cost": 1.5, "Part date": day}, {"Item": "Ink", "Cost": 4.0, "Part date": day}}
	if !reflect.DeepEqual(got, want) || reflect.DeepEqual(got, costBefore) {
		t.Errorf("the other sheet's rows = %v, want it Synced to %v", got, want)
	}
	_, body := ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Sales: The sheet lacks columns") || strings.Contains(body, "Cost Q1: ") {
		t.Errorf("the page does not name Sales alone as failing: %s", body)
	}
}

// syncFailureEmails is the emails to the address about a failing Sync of the Deployment called name, whatever their language.
func (s *site) syncFailureEmails(to, name string) []mailer.Message {
	var found []mailer.Message
	for _, m := range s.mail.Messages() {
		if m.To == to && (m.Subject == "CaveHost: the Sync of "+name+" failed" || m.Subject == "CaveHost: Penyegerakan "+name+" gagal") {
			found = append(found, m)
		}
	}
	return found
}

func TestTheLearnerGetsOneEmailWhenSyncsStartFailingAndNoMoreUntilASyncWorks(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	onlineSales(t, ada, "jualan-ada")
	ada.post("/language", url.Values{"lang": {"en"}, "next": {"/cavehost"}}) // the email follows the saved language, Malay until then
	emails := func() []mailer.Message { return ada.syncFailureEmails("ada@example.com", "jualan-ada") }
	if got := emails(); len(got) != 0 {
		t.Fatalf("%d failure emails before any Sync failed", len(got))
	}

	// The first failing Sync emails Ada, with the reason and where to look.
	broken := salesWith(t, []any{"Item", "Price"}, []any{"Pen", 2})
	google.swap(broken)
	ada.syncNow("jualan-ada")
	got := emails()
	if len(got) != 1 {
		t.Fatalf("%d failure emails after the first failing Sync, want 1", len(got))
	}
	for _, want := range []string{"Ada", "jualan-ada", "Missing: Qty", "/cavehost/jualan-ada", "last good rows"} {
		if !strings.Contains(got[0].Body, want) {
			t.Errorf("the failure email lacks %q: %s", want, got[0].Body)
		}
	}

	// Failing again, for the same reason or another, is the same failure: no more email.
	ada.syncDue()
	google.answerWith(http.StatusForbidden)
	ada.syncNow("jualan-ada")
	ada.syncDue()
	if n := len(emails()); n != 1 {
		t.Errorf("%d failure emails after more failing Syncs, want still 1", n)
	}

	// A Sync that works ends the failure without an email; failing after that is a new failure and emails again.
	google.answerWith(0)
	google.swap(salesSheet(t))
	ada.syncNow("jualan-ada")
	if n := len(emails()); n != 1 {
		t.Errorf("%d failure emails after a Sync that worked, want still 1", n)
	}
	google.swap(broken)
	ada.syncNow("jualan-ada")
	if n := len(emails()); n != 2 {
		t.Errorf("%d failure emails after a second run of failures, want 2", n)
	}
}

func TestTheFailureEmailIsInTheLearnersLanguage(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	onlineSales(t, ada, "jualan-ada")
	res, body := ada.post("/language", url.Values{"lang": {"ms"}, "next": {"/cavehost"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	google.swap(salesWith(t, []any{"Item", "Price"}, []any{"Pen", 2}))
	ada.syncNow("jualan-ada")
	got := ada.syncFailureEmails("ada@example.com", "jualan-ada")
	if len(got) != 1 || got[0].Subject != "CaveHost: Penyegerakan jualan-ada gagal" {
		t.Fatalf("failure emails = %v, want one in Malay", got)
	}
	if !strings.Contains(got[0].Body, "Hilang: Qty") {
		t.Errorf("the failure email does not give the reason in Malay: %s", got[0].Body)
	}
}

// resetColumns presses Reset columns on the sheet called sheet of the Deployment called name.
func (s *site) resetColumns(name, sheet string) {
	s.t.Helper()
	res, body := s.post("/cavehost/"+name+"/reset", url.Values{"sheet": {sheet}})
	expectRedirect(s.t, res, body, "/cavehost/"+name)
}

func TestResetColumnsKeepsTheLastRowsOnlineAndShowsANewPreviewToConfirmInPlaceOfSyncing(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")
	visitor := ada.second()

	// Ada restructures Sales on purpose: new column names. A Sync fails on it, as the columns it knew are gone.
	google.swap(salesWith(t, []any{"Product", "Units", "Price"}, []any{"Pen", 4, 2}, []any{"Cap", 9, 3}))
	ada.syncNow("jualan-ada")
	if _, body := ada.get("/cavehost/jualan-ada"); !strings.Contains(body, "It failed") || !strings.Contains(body, "Reset columns") {
		t.Fatalf("the page after the failed Sync does not offer Reset columns: %s", body)
	}

	ada.resetColumns("jualan-ada", "Sales")
	// The next Sync leaves the sheet alone: the rows online are still the last good ones, and the preview waits.
	ada.syncNow("jualan-ada")
	_, body := ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"Columns reset", "Check Sales before your page reads it", "Product", "Units", "Price", "Cap"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page after Reset columns lacks %q: %s", want, body)
		}
	}
	if got := visitor.rowsAt(link + "data.json"); !reflect.DeepEqual(got, salesRows()) {
		t.Errorf("data.json before the new preview is confirmed = %v, want the last good rows", got)
	}

	ada.confirmSheet("jualan-ada", nil)
	day := partDate()
	want := []map[string]any{{"Product": "Pen", "Units": 4.0, "Price": 2.0, "Part date": day}, {"Product": "Cap", "Units": 9.0, "Price": 3.0, "Part date": day}}
	if got := visitor.rowsAt(link + "data.json"); !reflect.DeepEqual(got, want) {
		t.Errorf("data.json after confirming = %v, want %v", got, want)
	}
	if _, body := ada.get("/cavehost/jualan-ada"); strings.Contains(body, "Check Sales before") {
		t.Errorf("the page still asks to confirm Sales: %s", body)
	}
	if files, _ := filepath.Glob(filepath.Join(ada.storageDir, "cavehost-*")); len(files) != 2 {
		t.Errorf("the store holds %v after confirming, want the page and one set of rows", files)
	}

	// The next Sync reads Sales with the new columns.
	google.swap(salesWith(t, []any{"Product", "Units", "Price"}, []any{"Pen", 5, 2}))
	ada.syncNow("jualan-ada")
	want = []map[string]any{{"Product": "Pen", "Units": 5.0, "Price": 2.0, "Part date": day}}
	if got := visitor.rowsAt(link + "data.json"); !reflect.DeepEqual(got, want) {
		t.Errorf("data.json after the next Sync = %v, want %v", got, want)
	}
	if _, body := ada.get("/cavehost/jualan-ada"); !strings.Contains(body, "It worked") {
		t.Errorf("the next Sync did not work: %s", body)
	}
}

func TestTheCaveHostPageGivesTheFailureReasonAndResetColumnsInMalay(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	onlineSales(t, ada, "jualan-ada")
	res, body := ada.post("/language", url.Values{"lang": {"ms"}, "next": {"/cavehost"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	google.swap(salesWith(t, []any{"Item", "Price"}, []any{"Pen", 2}))
	ada.syncNow("jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"Ia gagal", "Hilang: Qty", ">Set semula lajur<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page in Malay lacks %q: %s", want, body)
		}
	}
}
