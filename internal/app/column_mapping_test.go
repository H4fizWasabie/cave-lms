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
	t.Parallel()
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

// The Learner maps the five columns and the Add goes on from the same Workbook: the preview shows a note for what was
// matched, mapped and dropped.
func TestMappingTheMissingColumnsPreviewsTheNotesForWhatWasMatchedMappedAndDropped(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
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
}

var replaceAction = regexp.MustCompile(`href="(/lab/dashboards/\d+/parts/\d+/replace)"`)

// Replace one Part reads its file with the Dataset's columns too, so it offers the mapping, posting back to the Part's
// own Replace address; the Part keeps its place and takes the mapped rows.
func TestReplacingOnePartOffersTheMappingAndTakesTheMappedRows(t *testing.T) {
	t.Parallel()
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

// The preview shows a dropdown for each column the Learner mapped, with their file column selected, so a wrong choice can
// be changed when the file is read again; the columns Cave matched by name are not asked about.
func TestThePreviewOffersADropdownForEachColumnTheLearnerMappedButNotForThoseMatchedByName(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	postMapping(t, ada, path+"/dataset", refused, augustPicks, adding)
	body := mustGet(t, ada, path+"/data")
	expectAll(t, body, `name="map:JUMHADIR"`, `<option value="JUMLAH HADIR" selected>`, `name="map:NAMA_PENUH_INSTITUSI"`)
	if strings.Contains(body, `name="map:KOD_INSTITUSI"`) {
		t.Fatalf("the preview asks about a column Cave matched by name: %s", body)
	}
}

// The mapping form and the notes read in Malay too.
func TestTheMappingFormAndNotesReadInMalay(t *testing.T) {
	t.Parallel()
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

// One file column cannot be two of the Dataset's: the upload is refused naming the file column and both of the
// Dataset's, in either language, and the form asks again.
func TestOneFileColumnPickedForTwoDatasetColumnsIsRefusedNamingBoth(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)

	twice := map[string]string{}
	for column, file := range augustPicks {
		twice[column] = file
	}
	twice["JUMTHADIR"] = "JUMLAH HADIR"
	res, body := postMapping(t, ada, path+"/dataset", refused, twice, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "JUMLAH HADIR", "both JUMHADIR and JUMTHADIR", `name="map:JUMHADIR"`, `name="map:JUMTHADIR"`)

	ada.switchLanguage("ms", path)
	res, body = postMapping(t, ada, path+"/dataset", refused, twice, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "JUMLAH HADIR", "JUMHADIR dan JUMTHADIR")
}

// With the form to match the columns on the page, the refusal says the Learner can match them, not only fix the headers.
func TestTheRefusalWithAMappingFormSaysTheColumnsCanBeMatched(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	res, body := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA1001", "SK Maju"}), adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	expectAll(t, body, "Match the columns below, fix the headers in Excel, or choose Replace data.")
	if strings.Contains(body, "Fix the headers in Excel, or choose Replace data.") {
		t.Fatalf("the refusal still says only to fix the headers: %s", body)
	}

	ada.switchLanguage("ms", path)
	_, body = ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA1001", "SK Maju"}), adding)
	expectAll(t, body, "Padankan lajur di bawah, betulkan tajuk lajur dalam Excel, atau pilih Gantikan data.")

	// A file with nothing left over to pick from has no form, and keeps the old advice.
	ada.switchLanguage("en", path)
	short := testutil.XLSX(t, juneColumns[:3], []any{"Batu Pahat", "JBA1001", "SK Maju"})
	res, body = ada.postFile(path+"/dataset", "short.xlsx", short, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Fix the headers in Excel, or choose Replace data.")
	if strings.Contains(body, `name="map:`) {
		t.Fatalf("a file with no spare columns is offered a mapping: %s", body)
	}
}

// The mapping form posts the Workbook it was made from, which is checked like any pick: another Learner's Workbook
// answers 404 and nothing is added.
func TestTheMappingFormsWorkbookIsOwnershipChecked(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path, _ := kehadiranPage(t, ada)
	_, refused := ada.postFile(path+"/dataset", "kehadiran_082026.xlsx", augustReport(t, [2]string{"JBA2002", "SK Jaya"}), adding)
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	admin.assignPlan("bea@example.com", admin.onlyPlanID())
	hers := bea.createPage("Kehadiran", myPage)
	res, body := bea.postFile(hers+"/dataset", "kehadiran_062026.xlsx", juneReport(t, [2]string{"JBA1001", "SK Maju"}))
	expectRedirect(t, res, body, hers+"/data")
	res, body = bea.post(hers+"/dataset/confirm", nil)
	expectRedirect(t, res, body, hers+"/data")
	files := bea.datasetFiles()

	res, body = postMapping(t, bea, hers+"/dataset", refused, augustPicks, adding)
	expectStatus(t, res, body, http.StatusNotFound, "")
	if n := bea.datasetFiles(); n != files {
		t.Fatalf("%d stored files after the refused remap, want %d", n, files)
	}
	if list := partItems(t, mustGet(t, bea, hers+"/data")); strings.Contains(list, "kehadiran_082026.xlsx") {
		t.Fatalf("the refused remap added a Part: %s", list)
	}
}
