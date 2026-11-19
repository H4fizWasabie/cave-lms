package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Google's public sheet list (/spreadsheets/d/<id>/htmlview) carries every sheet's name and gid in the page's script, one
// items.push({...}) per sheet. The fake serves the same shape, with the escapes Google writes in a JavaScript string.

type gidSheet struct{ Name, Gid string }

var jsEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\x22`, `'`, `\x27`, `&`, `\x26`, `<`, `\x3c`, `>`, `\x3e`, `=`, `\x3d`, `/`, `\/`)

// sheetListPage is the Spreadsheet's sheet list page, as Google writes it, for the sheets in order.
func sheetListPage(id string, sheets ...gidSheet) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><title>Book</title></head><body><script>var gidMatch = /gid=(-?[0-9]+)/.exec(window.location.hash); var gid = gidMatch ? gidMatch[1] : null;var items = [];`)
	for _, s := range sheets {
		fmt.Fprintf(&b, `items.push({name: "%s", pageUrl: "https:\/\/docs.google.com\/spreadsheets\/d\/%s\/htmlview\/sheet?headers\x3dtrue&gid=%s", gid: "%s",initialSheet: ("%s" == gid)});`,
			jsEscaper.Replace(s.Name), id, s.Gid, s.Gid, s.Gid)
	}
	b.WriteString(`var sheetsViewport = document.getElementById('sheets-viewport');</script></body></html>`)
	return b.String()
}

const sheetListPath = "/spreadsheets/d/1AbC-dEf_123/htmlview"

func gidLink(gid string) string {
	return "https://docs.google.com/spreadsheets/d/1AbC-dEf_123/edit?usp=sharing#gid=" + gid
}

func threeGids() string {
	return sheetListPage("1AbC-dEf_123", gidSheet{"Sales", "0"}, gidSheet{"Staff", "1234567"}, gidSheet{"Cost Q1", "99"})
}

func TestALinkWithTheGidOfASheetStartsWithThatSheetTicked(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	google.page = threeGids()

	res, body := ada.linkSheet(gidLink("1234567"), "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	if got := google.requests(); len(got) != 2 || got[1] != sheetListPath {
		t.Fatalf("Google was asked for %q, want the export and then the sheet list", got)
	}

	// Only Staff is ticked, and it is the one whose Clean-up waits to be confirmed.
	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "Check Staff before your page reads it")
	if strings.Count(body, ">Untick<") != 1 || strings.Count(body, ">Tick<") != 2 {
		t.Errorf("the page does not show only Staff ticked: %s", body)
	}
	ada.confirmSheet("jualan-ada", nil)
	visitor := ada.second()
	link := ada.caveHostLink("jualan-ada")
	if got := visitor.sheetJSON(link, "Staff"); got != http.StatusOK {
		t.Errorf("sheets/Staff.json = %d, want 200", got)
	}
	if got := visitor.sheetJSON(link, "Sales"); got != http.StatusNotFound {
		t.Errorf("sheets/Sales.json = %d, want 404: the first sheet did not start ticked", got)
	}
}

func TestALinkWithoutAGidStartsWithTheFirstSheetTickedAndAsksGoogleForTheFileAlone(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	google.page = threeGids()

	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	if got := google.requests(); len(got) != 1 {
		t.Errorf("Google was asked for %q, want the export alone", got)
	}
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Check Sales before your page reads it") {
		t.Errorf("the first sheet did not start ticked: %s", body)
	}
}

func TestWhenCaveCannotMatchTheGidTheFirstSheetStartsTicked(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, gid, page string }{
		{"the gid is not in the list", "424242", threeGids()},
		{"the list has no sheets", "99", "<html><body>nothing here</body></html>"},
		{"the sheet list is not served", "99", ""},
		{"the name is not in the workbook", "555", sheetListPage("1AbC-dEf_123", gidSheet{"Sales", "0"}, gidSheet{"Renamed since", "555"})},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ada, google := caveHostLearner(t, threeSheets(t))
			google.page = c.page

			res, body := ada.linkSheet(gidLink(c.gid), "jualan-ada")
			expectRedirect(t, res, body, "/cavehost/jualan-ada")
			_, body = ada.get("/cavehost/jualan-ada")
			if !strings.Contains(body, "Check Sales before your page reads it") || strings.Count(body, ">Untick<") != 1 {
				t.Errorf("the first sheet did not start ticked: %s", body)
			}
		})
	}
}

func TestWhenGoogleFailsToAnswerTheSheetListTheFirstSheetStartsTicked(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, threeSheets(t))
	google.page = threeGids()
	google.listStatus = http.StatusInternalServerError

	res, body := ada.linkSheet(gidLink("1234567"), "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "Check Sales before your page reads it") {
		t.Errorf("the first sheet did not start ticked: %s", body)
	}
}

func TestASheetNameWithEscapedCharactersInTheListStillMatches(t *testing.T) {
	t.Parallel()
	name := `Ali's "Q&A" <1>`
	book := testutil.Workbook(t,
		testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 3}}},
		testutil.Sheet{Name: name, Rows: [][]any{{"Item", "Qty"}, {"Ink", 5}}},
	)
	ada, google := caveHostLearner(t, book)
	google.page = sheetListPage("1AbC-dEf_123", gidSheet{"Sales", "0"}, gidSheet{name, "77"})

	res, body := ada.linkSheet(gidLink("77"), "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	if strings.Count(body, ">Untick<") != 1 || !strings.Contains(body, "Ink") {
		t.Errorf("the sheet %q did not start ticked: %s", name, body)
	}
}
