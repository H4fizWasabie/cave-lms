package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// twoRowSheet is a Spreadsheet whose first sheet has a title row and a two-row header, as the Lab's Clean-up tests have.
func twoRowSheet(t *testing.T) []byte {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	file := testutil.XLSX(t, []string{"Jualan Oktober 2026"},
		[]any{"Tarikh", "Jualan", nil, "Staf"},
		[]any{nil, "Tunai", "Kad"},
		[]any{day, 120, 80, "Ali"},
		[]any{day, 60, 0, "Siti"},
	)
	return testutil.Merge(t, file, "A2:A3", "B2:C2", "D2:D3")
}

// dataRowsOf is the rows of the Deployment's data.json.
func (s *site) dataRowsOf(link string) []map[string]any {
	s.t.Helper()
	res, body := s.get(link + "data.json")
	expectStatus(s.t, res, body, http.StatusOK, "")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		s.t.Fatalf("data.json is not JSON: %v\n%s", err, body)
	}
	return rows
}

func TestThePreviewListsCleanUpsChangeNotes(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, testutil.XLSX(t, []string{"Kedai Runcit Ali"}, []any{"Item", "Qty"}, []any{"Pen", 3}, []any{"Ink", nil}))
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")

	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "What Cave changed")
	expectStatus(t, res, body, http.StatusOK, "Skipped row 1 above the header in row 2.")
	expectStatus(t, res, body, http.StatusOK, "Item")
}

func TestTheHeaderRowAndTwoRowHeaderCanBePickedInThePreviewAndConfirmingKeepsThem(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, twoRowSheet(t))
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")

	// Read as it comes, the sheet gives a Problem, and the page offers the choices to read it again; nothing can be confirmed.
	res, body = ada.get("/cavehost/jualan-ada")
	expectStatus(t, res, body, http.StatusOK, "Column C has data but no name")
	expectStatus(t, res, body, http.StatusOK, `name="two-row"`)
	res, body = ada.post("/cavehost/jualan-ada/confirm", nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Column C has data but no name")

	// Reading again with the two-row header joins its names; the preview still waits for the Learner to confirm.
	res, body = ada.get("/cavehost/jualan-ada?two-row=on")
	expectStatus(t, res, body, http.StatusOK, "Joined header rows 2 and 3 into one name per column.")
	expectStatus(t, res, body, http.StatusOK, "Jualan Tunai")
	var linkKey string // the page shows the link only once the sheet is confirmed
	if err := ada.db.QueryRow(`SELECT link_key FROM deployments`).Scan(&linkKey); err != nil {
		t.Fatal(err)
	}
	if res, _ := ada.second().get(pagesOrigin + "/ada-example-com/jualan-ada-" + linkKey + "/data.json"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("data.json before confirming = %d, want 404", res.StatusCode)
	}

	// A header row typed by hand is read as chosen.
	res, body = ada.get("/cavehost/jualan-ada?header=2&two-row=on")
	expectStatus(t, res, body, http.StatusOK, "Read row 2 as the header, as you chose.")

	// Confirming with the choices sets the sheet's columns.
	ada.confirmSheet("jualan-ada", url.Values{"header": {"2"}, "two-row": {"on"}})
	link := ada.caveHostLink("jualan-ada")
	rows := ada.second().dataRowsOf(link)
	if len(rows) != 2 || rows[0]["Jualan Tunai"] != 120.0 || rows[1]["Staf"] != "Siti" {
		t.Fatalf("data.json = %v, want the two rows under the joined column names", rows)
	}
	// Confirmed, the preview is gone and a second confirm changes nothing.
	res, body = ada.get("/cavehost/jualan-ada?two-row=")
	expectStatus(t, res, body, http.StatusOK, "Jualan Tunai")
	if strings.Contains(body, "What Cave changed") {
		t.Fatalf("a confirmed sheet shows its preview again: %s", body)
	}
	ada.confirmSheet("jualan-ada", nil)
	if got := ada.second().dataRowsOf(link); len(got) != 2 || got[0]["Jualan Tunai"] != 120.0 {
		t.Fatalf("a second confirm changed data.json to %v", got)
	}
}

func TestACaveHostLinkFollowsTheUsernameRedirectAndNeedsItsLinkKey(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.putPageOnline("jualan-ada", myPage)
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()

	// A wrong Username redirects for good to the owner's, keeping the file asked for (ADR 0023).
	for _, file := range []string{"", "data.json"} {
		res, _ := visitor.get(strings.Replace(link, "/ada-example-com/", "/nobody/", 1) + file)
		if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != pagesPath(link)+file {
			t.Errorf("a wrong Username for %q = %d to %q, want 301 to %q", file, res.StatusCode, res.Header.Get("Location"), pagesPath(link)+file)
		}
	}
	// The new Username covers it too: Ada changes hers and the old link redirects.
	res, body := ada.post("/my-profile/username", url.Values{"username": {"ada-baru"}})
	if res.StatusCode >= 400 {
		t.Fatalf("changing the Username = %d: %s", res.StatusCode, body)
	}
	res, _ = visitor.get(link)
	if want := strings.Replace(pagesPath(link), "/ada-example-com/", "/ada-baru/", 1); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != want {
		t.Errorf("the old link after a new Username = %d to %q, want 301 to %q", res.StatusCode, res.Header.Get("Location"), want)
	}

	// The key is the secret: the bare name and a wrong key are not found, with no redirect that would hand it out.
	bare := pagesOrigin + "/ada-baru/jualan-ada/"
	wrong := pagesOrigin + "/ada-baru/jualan-ada-aaaaaaaaaa/"
	for _, p := range []string{bare, bare + "data.json", wrong, wrong + "data.json"} {
		if res, _ := visitor.get(p); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, res.StatusCode)
		}
	}
	// Nothing else of a Dashboard's is there.
	for _, file := range []string{"charts.js", "data/abc"} {
		if res, _ := visitor.get(pagesOrigin + "/ada-baru/" + strings.TrimPrefix(pagesPath(link), "/ada-example-com/") + file); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s of a CaveHost Deployment = %d, want 404", file, res.StatusCode)
		}
	}
}

func TestADeploymentWhoseCaveHostPlanEndedShowsTheInactivePageAndStaysOnItsLink(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")
	ada.confirmSheet("jualan-ada", nil)
	ada.putPageOnline("jualan-ada", myPage)
	link := ada.caveHostLink("jualan-ada")
	visitor := ada.second()

	if _, err := ada.db.Exec(`UPDATE cavehost_plan_periods SET starts_at = starts_at - interval '60 days', ends_at = ends_at - interval '60 days'`); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"", "data.json"} {
		res, body := visitor.get(link + file)
		expectStatus(t, res, body, http.StatusOK, "This CaveHost page is asleep")
		if strings.Contains(body, "Jualan bulan ini") || strings.Contains(body, `"Item"`) {
			t.Fatalf("an asleep Deployment shows its content: %s", body)
		}
	}
}

func TestAPageNeedsAConfirmedSheetAndMustBeAnHTMLPage(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	ada.linkSheet(sheetLink, "jualan-ada")

	// Before the sheet is confirmed there is nowhere for a page to go.
	res, body := ada.post("/cavehost/jualan-ada/page", url.Values{"html": {myPage}})
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	if _, body := ada.get("/cavehost/jualan-ada"); strings.Contains(body, `action="/cavehost/jualan-ada/page"`) {
		t.Fatalf("an unconfirmed sheet offers the page form: %s", body)
	}
	ada.confirmSheet("jualan-ada", nil)

	res, body = ada.post("/cavehost/jualan-ada/page", url.Values{"html": {"   "}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Paste your page")
	res, body = ada.postFile("/cavehost/jualan-ada/page", "page.txt", []byte(myPage))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "page.txt")
	res, body = ada.post("/cavehost/jualan-ada/page", url.Values{"html": {strings.Repeat("a", 5<<20+1)}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")

	// A picked .html file works as a pasted page does.
	res, body = ada.postFile("/cavehost/jualan-ada/page", "page.html", []byte(myPage))
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	link := ada.caveHostLink("jualan-ada")
	res, body = ada.second().get(link)
	expectStatus(t, res, body, http.StatusOK, "Jualan bulan ini")
}

func TestCaveHostScreensAndMessagesReadInMalay(t *testing.T) {
	t.Parallel()
	ada, _ := caveHostLearner(t, salesSheet(t))
	res, body := ada.getAsking("/cavehost", "ms")
	for _, want := range []string{"Pautkan Hamparan", "Sesiapa yang mempunyai pautan boleh melihat data ini", "Deployment CaveHost"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	post := func(path string, form url.Values) (*http.Response, string) {
		req := ada.newRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept-Language", "ms")
		return ada.do(req)
	}
	res, body = post("/cavehost", url.Values{"link": {"https://example.com"}, "name": {"jualan-ada"}, "right": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Itu bukan pautan Hamparan Google")
	res, body = post("/cavehost", url.Values{"link": {sheetLink}, "name": {"jualan-ada"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tandakan bahawa anda berhak")

	res, body = post("/cavehost", url.Values{"link": {sheetLink}, "name": {"jualan-ada"}, "right": {"yes"}})
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	res, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	for _, want := range []string{"Semak Sales sebelum halaman anda membacanya", "Cave tidak mengubah apa-apa", "Sahkan helaian ini"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	ada.confirmSheet("jualan-ada", nil)
	res, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	for _, want := range []string{"Helaian Sales sudah sedia", "Letakkan halaman dalam talian"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
	ada.putPageOnline("jualan-ada", versionOf(1))
	ada.putPageOnline("jualan-ada", versionOf(2))
	res, body = ada.getAsking("/cavehost/jualan-ada", "ms")
	for _, want := range []string{"Gantikan halaman anda", "Gantikan halaman", "Halaman terdahulu", "Pulihkan", "Dimuat naik"} {
		expectStatus(t, res, body, http.StatusOK, want)
	}
}
