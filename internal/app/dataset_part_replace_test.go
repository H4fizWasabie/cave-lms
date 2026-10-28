package app_test

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

var partReplaceLink = regexp.MustCompile(`href="(/lab/dashboards/\d+/parts/\d+/replace)"`)

// threeParts is twoParts with a third Part, november.xlsx, with Garam.
func threeParts(t *testing.T, ada *site) (path, link string) {
	t.Helper()
	path, link = twoParts(t, ada)
	ada.addFile(path, "november.xlsx", shopColumns(t, "Garam", 70))
	return path, link
}

// replaceLinks are the Data tab's Replace links, one per Part in order.
func replaceLinks(t *testing.T, ada *site, tab string) []string {
	t.Helper()
	var out []string
	for _, m := range partReplaceLink.FindAllStringSubmatch(mustGet(t, ada, tab), -1) {
		out = append(out, m[1])
	}
	return out
}

// markahReport is a workbook whose columns are not the shop's.
func markahReport(t *testing.T) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Nama", "Markah"}, []any{"Ali", 80.0}, []any{"Siti", 90.0})
}

func TestReplacingTheFirstOfThreePartsKeepsItFirstWithTheNewRowsAndQueuesTheOldRows(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := threeParts(t, ada)
	visitor := ada.second()
	links := replaceLinks(t, ada, path+"/data")
	if len(links) != 3 {
		t.Fatalf("%d Replace links, want one per Part", len(links))
	}

	body := mustGet(t, ada, links[0])
	expectAll(t, body, "Replace Part", "september.xlsx", `action="`+links[0]+`"`, `type="file"`)
	ada.switchLanguage("ms", links[0])
	expectAll(t, mustGet(t, ada, links[0]), "Gantikan Bahagian", "september.xlsx")
	ada.switchLanguage("en", links[0])

	res, body := ada.postFile(links[0], "september-betul.xlsx", shopColumns(t, "Tepung", 60))
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Replacing september.xlsx with these 2 rows: 6 rows in all", `action="`+path+`/dataset/confirm"`, `name="part-date"`)
	if strings.Contains(body, "Confirming removes") {
		t.Fatalf("a Replace of one Part warns that the whole Dataset goes: %s", body)
	}
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Tepung") {
		t.Fatalf("before confirming, data.json = %s", body)
	}
	ada.switchLanguage("ms", path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Menggantikan september.xlsx dengan 2 baris ini: 6 baris semuanya")
	ada.switchLanguage("en", path+"/data")
	if n := ada.datasetFiles(); n != 4 {
		t.Fatalf("%d stored files with the replacement waiting, want 4", n)
	}

	ada.forgetWorkbooks()
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")
	_, body = visitor.get(link + "data.json")
	tepung, gula, garam := strings.Index(body, "Tepung"), strings.Index(body, "Gula"), strings.Index(body, "Garam")
	if strings.Contains(body, "Beras") || tepung < 0 || gula < tepung || garam < gula {
		t.Fatalf("after replacing the first Part data.json = %s, want Tepung, Gula, Garam in order and no Beras", body)
	}
	if n := ada.datasetFiles(); n != 3 {
		t.Fatalf("%d stored files after the replace, want one per Part", n)
	}
	var queued int
	if err := ada.db.QueryRow(`SELECT count(*) FROM stored_file_deletions`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("%d files still queued for deletion (%v), want the replaced Part's deleted", queued, err)
	}
	list := partItems(t, mustGet(t, ada, path+"/data"))
	first, second, third := strings.Index(list, "september-betul.xlsx"), strings.Index(list, "oktober.xlsx"), strings.Index(list, "november.xlsx")
	if strings.Contains(list, "september.xlsx") || first < 0 || second < first || third < second {
		t.Fatalf("the Part list after the replace: %s", list)
	}
	expectAll(t, mustGet(t, ada, "/lab"), ada.storedKB()) // the swapped Part counts, the old one does not
}

func TestAReplaceOfOnePartCountsItsOldPartAsFreeAndIsRefusedWhenItFitsNeitherWay(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "4") // each bigFinance is about 0.6 MB as a Workbook and 1.1 MB as a Part
	path := ada.createPage("Kedai", myPage)
	dataLink := ada.deployAs(path, "kedai-ada")
	visitor := ada.second()
	res, body := ada.postFile(path+"/dataset", "satu.xlsx", bigFinance(t, 1))
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	ada.addFile(path, "dua.xlsx", testutil.XLSX(t, []string{"Tarikh", "Perkara", "Kategori", "Jenis", "Jumlah"}, []any{time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), "Gula", "Dapur", "Keluar", 80.0}))
	link := replaceLinks(t, ada, path+"/data")[0]
	_, before := visitor.get(dataLink + "data.json")

	// 3 MB holds the Workbooks and the new Part only once the old Part is counted as free.
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 3`); err != nil {
		t.Fatal(err)
	}
	res, body = ada.postFile(link, "satu-betul.xlsx", bigFinance(t, 2))
	expectRedirect(t, res, body, path+"/data")
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 2`); err != nil { // the allowance shrinks before the confirm
		t.Fatal(err)
	}
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
	if list := partItems(t, mustGet(t, ada, path+"/data")); !strings.Contains(list, "satu.xlsx") || strings.Contains(list, "satu-betul.xlsx") {
		t.Fatalf("a refused confirm changed the Parts: %s", list)
	}

	// At 2 MB a new upload does not fit even with the old Part gone: refused, and nothing changes.
	res, body = ada.postFile(link, "satu-lagi.xlsx", bigFinance(t, 3))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Storage allowance")
	if list := partItems(t, mustGet(t, ada, path+"/data")); !strings.Contains(list, "satu.xlsx") || strings.Contains(list, "satu-lagi.xlsx") {
		t.Fatalf("a refused Replace changed the Parts: %s", list)
	}
	if _, err := ada.db.Exec(`UPDATE plan_periods SET storage_mb = 3`); err != nil {
		t.Fatal(err)
	}
	res, body = ada.postFile(link, "satu-betul.xlsx", bigFinance(t, 2))
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	if list := partItems(t, mustGet(t, ada, path+"/data")); !strings.Contains(list, "satu-betul.xlsx") || strings.Contains(list, "satu.xlsx") {
		t.Fatalf("the Replace that fits did not swap the Part: %s", list)
	}
	if _, after := visitor.get(dataLink + "data.json"); after == before {
		t.Fatal("the Replace that fits changed nothing")
	}
}

func TestAThemeBuiltDashboardCountsTheRowTotalWithoutTheReplacedPart(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createWork("Kedai")
	ada.confirmFile(path, "september.xlsx", bigReport(t, "Beras", 25000))
	ada.addFile(path, "oktober.xlsx", bigReport(t, "Gula", 25000))
	link := replaceLinks(t, ada, path)[0]
	before := ada.rowsOf(path + "/view/")

	res, body := ada.postFile(link, "september-lagi.xlsx", bigReport(t, "Tepung", 25001))
	expectStatus(t, res, body, http.StatusUnprocessableEntity,
		"These 25,001 rows and the 25,000 in the other Parts make 50,001. A Dashboard built in a Theme holds up to 50,000 rows. Use fewer rows, or delete another Part to make room.")
	ada.switchLanguage("ms", path)
	res, body = ada.postFile(link, "september-lagi.xlsx", bigReport(t, "Tepung", 25001))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Dashboard yang dibina dalam Tema memuatkan sehingga 50,000 baris")
	if after := ada.rowsOf(path + "/view/"); after != before {
		t.Fatal("a refused Replace changed the Dashboard's data")
	}

	res, body = ada.postFile(link, "september-betul.xlsx", bigReport(t, "Tepung", 25000))
	expectRedirect(t, res, body, path)
	ada.confirmWork(path, nil)
	if after := ada.rowsOf(path + "/view/"); after == before {
		t.Fatal("the Replace within the total changed nothing")
	}
}

func TestDeletingAnotherPartKeepsAPendingReplaceOfOnePartAndDeletingItsPartDropsIt(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := threeParts(t, ada)
	visitor := ada.second()
	links := replaceLinks(t, ada, path+"/data")
	del := partDeleteLink.FindAllStringSubmatch(mustGet(t, ada, path+"/data"), -1)

	ada.postFile(links[0], "september-betul.xlsx", shopColumns(t, "Tepung", 60))
	res, body := ada.post(del[1][1], confirming) // oktober.xlsx
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Replacing september.xlsx")
	ada.confirmWork(path, nil)
	_, body = visitor.get(link + "data.json")
	if !strings.Contains(body, "Tepung") || strings.Contains(body, "Gula") || strings.Contains(body, "Beras") || !strings.Contains(body, "Garam") {
		t.Fatalf("data.json = %s, want Tepung and Garam", body)
	}

	links = replaceLinks(t, ada, path+"/data")
	ada.postFile(links[1], "november-betul.xlsx", shopColumns(t, "Minyak", 50))
	res, body = ada.post(del[2][1], confirming) // november.xlsx, the Part the pending file replaces
	expectRedirect(t, res, body, path+"/data")
	if body = mustGet(t, ada, path+"/data"); strings.Contains(body, "Replacing") || strings.Contains(body, `name="part-date"`) {
		t.Fatalf("a pending Replace of a deleted Part is still shown: %s", body)
	}
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files, want the one Part left", n)
	}
}

func TestAReplaceOfOneOfSeveralPartsNeedsTheDatasetsColumnsAndOnlyReachesTheLearnersOwnParts(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := twoParts(t, ada)
	links := replaceLinks(t, ada, path+"/data")
	res, body := ada.postFile(links[0], "markah.xlsx", underTitleRows(t, []any{"Nama", "Markah"}, []any{"Ali", 80.0}))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing:")
	if list := partItems(t, mustGet(t, ada, path+"/data")); !strings.Contains(list, "september.xlsx") || strings.Contains(list, "markah.xlsx") {
		t.Fatalf("a refused Replace changed the Parts: %s", list)
	}

	other := ada.createPage("Lain", myPage)
	crossed := other + links[0][strings.Index(links[0], "/parts/"):]
	if res, _ = ada.get(crossed); res.StatusCode != http.StatusNotFound {
		t.Fatalf("the Replace screen of another Dashboard's Part answered %d, want 404", res.StatusCode)
	}
	if res, _ = ada.postFile(crossed, "markah.xlsx", markahReport(t)); res.StatusCode != http.StatusNotFound {
		t.Fatalf("replacing another Dashboard's Part answered %d, want 404", res.StatusCode)
	}
}

// A Part replacing one of several is read with the Dataset's recipe, as an Add is: only the recipe sheet, never
// another sheet with the same columns; and confirming it keeps the recipe.
func TestAReplaceOfOneOfSeveralPartsIsReadWithTheRecipeAndKeepsIt(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := twoParts(t, ada)
	visitor := ada.second()
	file := testutil.Workbook(t, shopSheet("SHEET1", "Tepung", "Minyak"), shopSheet("Subset", "Gula"))
	res, body := ada.postFile(replaceLinks(t, ada, path+"/data")[0], "september-betul.xlsx", file)
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, "Replacing september.xlsx with these 2 rows")
	if strings.Contains(body, "Combined") {
		t.Fatalf("the preview says sheets were combined: %s", body)
	}
	ada.confirmWork(path, nil)
	_, body = visitor.get(link + "data.json")
	if got, want := perkara(t, body), []string{"Tepung", "Minyak", "Gula", "Jualan"}; !slices.Equal(got, want) {
		t.Fatalf("data.json Perkara = %v, want %v", got, want)
	}
	// The recipe is still the first Part's: an Add after the replace reads only its sheet.
	ada.postFile(path+"/dataset", "november.xlsx", testutil.Workbook(t, shopSheet("SHEET1", "Garam"), shopSheet("Subset", "Lada")), adding)
	expectAll(t, mustGet(t, ada, path+"/data"), "Adding 1 row to your 4: 5 rows in all.")
}

// partItems is the Part list of a Data tab alone, without the preview that may follow it.
func partItems(t *testing.T, body string) string {
	t.Helper()
	list := partList(t, body)
	return list[:strings.Index(list, "</ul>")]
}
