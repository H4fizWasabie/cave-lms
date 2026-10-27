package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// juneColumns are the Dataset's columns: the kehadiran export of June and July.
var juneColumns = []string{"PPD", "KOD_INSTITUSI", "NAMA_PENUH_INSTITUSI", "KOD_THN_TING", "NAMA_KELAS", "TAHUN", "BULAN", "JUMPATUTHADIR", "JUMHADIR", "JUMTHADIR", "PERATUS"}

// augustColumns are the same report in August: seven of June's columns renamed (two of them only by spaces for
// underscores) and four more columns.
var augustColumns = []string{"JPN", "PPD", "KOD INSTITUSI", "NAMA INSTITUSI", "PERINGKAT", "JENIS SEKOLAH", "KOD TAHUN TINGKATAN", "NAMA KELAS",
	"TAHUN", "BULAN", "TARIKH BULAN", "JUMLAH PATUT HADIR", "JUMLAH HADIR", "JUMLAH TIDAK HADIR", "PERATUS"}

// juneReport is the kehadiran export of June, one class of school (code, name) per row.
func juneReport(t *testing.T, schools ...[2]string) []byte {
	t.Helper()
	var rows [][]any
	for _, s := range schools {
		rows = append(rows, []any{"Batu Pahat", s[0], s[1], "T1", "1 Amanah", 2026.0, 6.0, 100.0, 90.0, 10.0, 90.0})
	}
	return testutil.XLSX(t, juneColumns, rows...)
}

// augustReport is the kehadiran export of August with its renamed headers, one class of school (code, name) per row.
func augustReport(t *testing.T, schools ...[2]string) []byte {
	t.Helper()
	var rows [][]any
	for _, s := range schools {
		rows = append(rows, []any{"JOHOR", "Batu Pahat", s[0], s[1], "RENDAH", "SK", "T1", "1 Amanah", 2026.0, 8.0, "31/08/2026", 120.0, 108.0, 12.0, 90.0})
	}
	return testutil.XLSX(t, augustColumns, rows...)
}

// kehadiranPage is a deployed page Dashboard of Ada's whose Dataset is June's report, confirmed: its path and link.
func kehadiranPage(t *testing.T, ada *site) (path, link string) {
	t.Helper()
	path = ada.createPage("Kehadiran", myPage)
	link = ada.deployAs(path, "kehadiran-ada")
	res, body := ada.postFile(path+"/dataset", "kehadiran_062026.xlsx", juneReport(t, [2]string{"JBA1001", "SK Maju"}))
	expectRedirect(t, res, body, path+"/data")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	return path, link
}

// dataRows are data.json's rows.
func dataRows(t *testing.T, body string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("data.json is not one array: %v\n%s", err, body)
	}
	return rows
}

var pickedWorkbook = regexp.MustCompile(`name="workbook" value="(\d+)"`)

// An Add whose columns are renamed is refused as before, naming only the columns neither matched by name nor mapped,
// and offers a dropdown for each of them of the file's columns the Dataset does not use.
func TestAnAddWithRenamedColumnsIsRefusedOfferingADropdownPerMissingColumnOfTheFilesUnusedColumns(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)

	res, body := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA1001", "SK Maju"}), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "Missing: NAMA_PENUH_INSTITUSI, KOD_THN_TING, JUMPATUTHADIR, JUMHADIR, JUMTHADIR.",
		`<form class="form map-columns" method="post" action="`+path+`/dataset"`,
		`name="mode" value="add"`, `name="map:NAMA_PENUH_INSTITUSI"`, `name="map:JUMTHADIR"`)
	if pickedWorkbook.FindStringSubmatch(body) == nil {
		t.Fatalf("the mapping form does not carry the Workbook the file was kept as: %s", body)
	}
	// KOD INSTITUSI and NAMA KELAS match by name, so they are neither asked for nor offered.
	for _, absent := range []string{`name="map:KOD_INSTITUSI"`, `name="map:NAMA_KELAS"`, `<option value="KOD INSTITUSI"`, `<option value="NAMA KELAS"`, `<option value="PPD"`} {
		if strings.Contains(body, absent) {
			t.Fatalf("the page has %s: %s", absent, body)
		}
	}
	expectAll(t, body, `<option value="JPN"`, `<option value="NAMA INSTITUSI"`, `<option value="JUMLAH TIDAK HADIR"`, `<option value="TARIKH BULAN"`)
}

// postMapping posts the mapping form of a refused Add: the Workbook it carries, as the Learner would after picking.
func postMapping(t *testing.T, ada *site, action, body string, picks map[string]string, fields url.Values) (*http.Response, string) {
	t.Helper()
	m := pickedWorkbook.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no Workbook in the mapping form: %s", body)
	}
	form := url.Values{"workbook": {m[1]}}
	for k, v := range fields {
		form[k] = v
	}
	for column, file := range picks {
		form.Set("map:"+column, file)
	}
	return ada.post(action, form)
}

var augustPicks = map[string]string{
	"NAMA_PENUH_INSTITUSI": "NAMA INSTITUSI", "KOD_THN_TING": "KOD TAHUN TINGKATAN",
	"JUMPATUTHADIR": "JUMLAH PATUT HADIR", "JUMHADIR": "JUMLAH HADIR", "JUMTHADIR": "JUMLAH TIDAK HADIR",
}

// The Learner maps the five columns and the Add goes on from the same Workbook: the preview shows the rows under the
// Dataset's names with a note for what was matched, mapped and dropped, and confirming adds them.
func TestMappingTheMissingColumnsAddsThePartUnderTheDatasetsColumnNames(t *testing.T) {
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)

	res, body := postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Adding 1 row to your 1: 2 rows in all.",
		"Columns matched to your data&#39;s by name (2): KOD INSTITUSI → KOD_INSTITUSI, NAMA KELAS → NAMA_KELAS.",
		"Columns mapped to your data&#39;s (5): NAMA INSTITUSI → NAMA_PENUH_INSTITUSI, KOD TAHUN TINGKATAN → KOD_THN_TING, JUMLAH PATUT HADIR → JUMPATUTHADIR, JUMLAH HADIR → JUMHADIR, JUMLAH TIDAK HADIR → JUMTHADIR.",
		"Columns not in your data, dropped (4): JPN, PERINGKAT, JENIS SEKOLAH, TARIKH BULAN.",
		"<th scope=\"col\">JUMHADIR<br>")
	if strings.Contains(body, "NAMA INSTITUSI</th>") {
		t.Fatalf("the preview's table has the file's names: %s", body)
	}

	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	rows := dataRows(t, body)
	if len(rows) != 2 {
		t.Fatalf("data.json has %d rows, want 2: %s", len(rows), body)
	}
	for k, want := range map[string]any{"KOD_INSTITUSI": "JBA2002", "NAMA_PENUH_INSTITUSI": "SK Jaya", "KOD_THN_TING": "T1", "NAMA_KELAS": "1 Amanah", "JUMPATUTHADIR": 120.0, "JUMHADIR": 108.0, "JUMTHADIR": 12.0, "PERATUS": 90.0} {
		if rows[1][k] != want {
			t.Fatalf("data.json row 2 has %s = %v, want %v: %s", k, rows[1][k], want, body)
		}
	}
	if _, ok := rows[1]["JUMLAH HADIR"]; ok {
		t.Fatalf("data.json has the file's column names: %s", body)
	}
}

// The mapping is the Dataset's, kept in its recipe: next month's file with the same headers maps itself, and says so.
func TestTheNextFileWithTheSameHeadersMapsItselfAndSaysSo(t *testing.T) {
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	ada.post(path+"/dataset/confirm", nil)

	res, body := ada.postFile(path+"/dataset", "kehadiran_092026.xlsx", augustReport(t, [2]string{"JBA3003", "SK Baru"}), adding)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "Columns mapped to your data&#39;s (5): NAMA INSTITUSI → NAMA_PENUH_INSTITUSI")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	if rows := dataRows(t, body); len(rows) != 3 || rows[2]["NAMA_PENUH_INSTITUSI"] != "SK Baru" || rows[2]["JUMHADIR"] != 108.0 {
		t.Fatalf("data.json = %s, want a third row mapped by itself", body)
	}
}

// Replace data starts the Dataset afresh, so the file's columns become the Dataset's with no mapping offered, and the
// recipe's map goes with the rest of the recipe: the next Add is not mapped by the old choices.
func TestReplacingDataOffersNoMappingAndClearsTheRecipesMap(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	ada.post(path+"/dataset/confirm", nil)

	// The August file replaces the data: its columns are the Dataset's, and there is nothing to map.
	res, body := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), url.Values{"mode": {"replace"}})
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, "<th scope=\"col\">JUMLAH HADIR<br>")
	if strings.Contains(body, `name="map:`) || strings.Contains(body, "Columns mapped") {
		t.Fatalf("Replace data offers to map columns: %s", body)
	}

	// June's report replaces it again; then August's, which is no longer mapped by the choices made before.
	res, body = ada.postFile(path+"/dataset", "kehadiran_062026.xlsx", juneReport(t, [2]string{"JBA1001", "SK Maju"}), url.Values{"mode": {"replace"}})
	expectRedirect(t, res, body, path+"/data")
	ada.post(path+"/dataset/confirm", nil)
	res, body = ada.postFile(path+"/dataset", "kehadiran_092026.xlsx", augustReport(t, [2]string{"JBA3003", "SK Baru"}), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "Missing: NAMA_PENUH_INSTITUSI, KOD_THN_TING, JUMPATUTHADIR, JUMHADIR, JUMTHADIR.")
}

var replaceAction = regexp.MustCompile(`href="(/lab/dashboards/\d+/parts/\d+/replace)"`)

// Replace one Part reads its file with the Dataset's columns too, so it offers the mapping, posting back to the Part's
// own Replace address; the Part keeps its place and takes the mapped rows.
func TestReplacingOnePartOffersTheMappingAndTakesTheMappedRows(t *testing.T) {
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	replace := replaceAction.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]

	res, refused := ada.postFile(replace, "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}))
	expectStatus(t, res, refused, http.StatusUnprocessableEntity, "")
	expectAll(t, refused, `action="`+replace+`"`, `name="map:JUMHADIR"`)
	if strings.Contains(refused, `<input type="hidden" name="mode"`) {
		t.Fatalf("the Replace of a Part carries a mode: %s", refused)
	}

	res, body := postMapping(t, ada, replace, refused, augustPicks, nil)
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Columns mapped to your data&#39;s (5)")
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.second().get(link + "data.json")
	if rows := dataRows(t, body); len(rows) != 1 || rows[0]["KOD_INSTITUSI"] != "JBA2002" || rows[0]["JUMHADIR"] != 108.0 {
		t.Fatalf("data.json = %s, want the one Part replaced by the mapped rows", body)
	}
}

// A choice can be changed when the file is read again: the preview shows a dropdown for each column the Learner mapped,
// choosing another file column replaces the choice, and choosing none refuses the upload naming the column. Nothing is
// remembered until the Part is confirmed, so a refused re-read leaves the recipe's map as it was.
func TestAChosenMappingCanBeChangedOrClearedWhenTheFileIsReadAgain(t *testing.T) {
	_, ada := labLearner(t)
	path, link := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	body := mustGet(t, ada, path+"/data")
	expectAll(t, body, `name="map:JUMHADIR"`, `<option value="JUMLAH HADIR" selected>`, `name="map:NAMA_PENUH_INSTITUSI"`)
	if strings.Contains(body, `name="map:KOD_INSTITUSI"`) {
		t.Fatalf("the preview asks about a column Cave matched by name: %s", body)
	}
	ada.post(path+"/dataset/confirm", nil)

	// The saved mapping shows as a dropdown on the next file's preview; JUMHADIR read from another column instead.
	september := augustReport(t, [2]string{"JBA3003", "SK Baru"})
	changed := url.Values{"mode": {"add"}, "map:JUMHADIR": {"JUMLAH TIDAK HADIR"}, "map:JUMTHADIR": {"JUMLAH HADIR"}}
	res, body := ada.postFile(path+"/dataset", "kehadiran_092026.xlsx", september, changed)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "JUMLAH TIDAK HADIR → JUMHADIR", "JUMLAH HADIR → JUMTHADIR")
	ada.post(path+"/dataset/confirm", nil)
	_, body = ada.second().get(link + "data.json")
	if rows := dataRows(t, body); len(rows) != 3 || rows[2]["JUMHADIR"] != 12.0 || rows[2]["JUMTHADIR"] != 108.0 {
		t.Fatalf("data.json = %s, want the swapped choice in the third row", body)
	}

	// Choosing none for a column refuses that file, naming it; the mapping is not touched until a Part is confirmed.
	october := augustReport(t, [2]string{"JBA4004", "SK Lagi"})
	cleared := url.Values{"mode": {"add"}, "map:JUMHADIR": {""}}
	res, body = ada.postFile(path+"/dataset", "kehadiran_102026.xlsx", october, cleared)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "Missing: JUMHADIR.")
	res, body = ada.postFile(path+"/dataset", "kehadiran_102026.xlsx", october, adding)
	expectRedirect(t, res, body, path+"/data")
}

// The mapping form and the notes read in Malay too.
func TestTheMappingFormAndNotesReadInMalay(t *testing.T) {
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	ada.switchLanguage("ms", path)
	res, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	expectStatus(t, res, refused, http.StatusUnprocessableEntity, "")
	expectAll(t, refused, "Padankan lajur yang hilang", "Dalam data anda:", "Tiada dalam fail ini", "Padankan dan teruskan")

	postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	expectAll(t, mustGet(t, ada, path+"/data"),
		"Lajur dipadankan dengan lajur data anda mengikut nama (2): KOD INSTITUSI → KOD_INSTITUSI, NAMA KELAS → NAMA_KELAS.",
		"Lajur dipetakan kepada lajur data anda (5): NAMA INSTITUSI → NAMA_PENUH_INSTITUSI", "Lajur yang anda padankan")
}
