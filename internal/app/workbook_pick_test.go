package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// workbookID is the ID of the Learner's Workbook called name, as the pick form posts it.
func (s *site) workbookID(name string) string {
	s.t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id::text FROM workbooks WHERE name = $1`, name).Scan(&id); err != nil {
		s.t.Fatalf("no Workbook %s: %v", name, err)
	}
	return id
}

// pick posts the pick of Workbook id to a Data tab's form at action; fields are extra form fields, such as the mode.
func (s *site) pick(action, id string, fields ...url.Values) (*http.Response, string) {
	s.t.Helper()
	form := url.Values{"workbook": {id}}
	for _, f := range fields {
		for k, vs := range f {
			form[k] = vs
		}
	}
	return s.post(action, form)
}

// workbookCount is how many Workbooks the site keeps.
func (s *site) workbookCount() (n int) {
	s.t.Helper()
	if err := s.db.QueryRow(`SELECT count(*) FROM workbooks`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// shelved uploads file as a Workbook of Ada's that no Part uses: it goes to a Dashboard of its own and is never confirmed.
func shelved(t *testing.T, ada *site, name string, file []byte) string {
	t.Helper()
	other := ada.createPage("Rak "+name, myPage)
	res, body := ada.postFile(other+"/dataset", name, file)
	expectRedirect(t, res, body, other+"/data")
	return ada.workbookID(name)
}

func TestPickingAWorkbookAddsAPartLinkedToItWithoutANewUploadOrWorkbook(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	visitor := ada.second()
	id := shelved(t, ada, "oktober.xlsx", shopColumns(t, "Gula", 80))
	before := ada.workbookCount()

	body := mustGet(t, ada, path+"/data")
	expectAll(t, body, "Pick a Workbook", "oktober.xlsx", `name="workbook"`, `value="`+id+`"`)
	ada.switchLanguage("ms", path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Pilih Buku kerja", "oktober.xlsx")
	ada.switchLanguage("en", path+"/data")

	res, body := ada.pick(path+"/dataset", id, adding)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Adding 2 rows to your 2: 4 rows in all.", "oktober.xlsx", `name="part-date"`)
	if _, body = visitor.get(link + "data.json"); strings.Contains(body, "Gula") {
		t.Fatalf("before confirming, data.json = %s", body)
	}
	res, body = ada.post(path+"/dataset/confirm", nil)
	expectRedirect(t, res, body, path+"/data")

	_, body = visitor.get(link + "data.json")
	if got := perkara(t, body); strings.Join(got, ",") != "Beras,Jualan,Gula,Jualan" {
		t.Fatalf("data.json Perkara = %v, want the picked Workbook's rows after the first Part's", got)
	}
	if n := ada.workbookCount(); n != before {
		t.Fatalf("%d Workbooks after the pick, want the %d from before: a pick uploads nothing", n, before)
	}
	var linked string
	if err := ada.db.QueryRow(`SELECT workbook_id::text FROM dataset_parts WHERE file_name = 'oktober.xlsx' AND state = 'current'`).Scan(&linked); err != nil || linked != id {
		t.Fatalf("the new Part links Workbook %q (%v), want %s", linked, err, id)
	}
	expectAll(t, mustGet(t, ada, path+"/data"), "2 Parts", "oktober.xlsx")
}

func TestPickingAWorkbookToReplaceAPartTakesThatPartsPlace(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := threeParts(t, ada)
	visitor := ada.second()
	id := shelved(t, ada, "oktober-betul.xlsx", shopColumns(t, "Tepung", 60))
	links := replaceLinks(t, ada, path+"/data")
	before := ada.workbookCount()

	body := mustGet(t, ada, links[1])
	expectAll(t, body, "Pick a Workbook", "oktober-betul.xlsx", `action="`+links[1]+`"`)
	if strings.Contains(body, `name="mode"`) {
		t.Fatalf("the Replace screen offers Replace data or Add: %s", body)
	}
	res, body := ada.pick(links[1], id)
	expectRedirect(t, res, body, path+"/data")
	expectAll(t, mustGet(t, ada, path+"/data"), "Replacing oktober.xlsx with these 2 rows")
	ada.confirmWork(path, nil)

	_, body = visitor.get(link + "data.json")
	if got := perkara(t, body); strings.Join(got, ",") != "Beras,Jualan,Tepung,Jualan,Garam,Jualan" {
		t.Fatalf("data.json Perkara = %v, want the picked Workbook's rows in the replaced Part's place", got)
	}
	list := partItems(t, mustGet(t, ada, path+"/data"))
	if a, b, c := strings.Index(list, "september.xlsx"), strings.Index(list, "oktober-betul.xlsx"), strings.Index(list, "november.xlsx"); strings.Contains(list, "oktober.xlsx") || a < 0 || b < a || c < b {
		t.Fatalf("the Part list after the pick: %s", list)
	}
	if n := ada.workbookCount(); n != before {
		t.Fatalf("%d Workbooks after the pick, want %d", n, before)
	}
}

func TestAnEmptyDatasetTakesAnyReadableWorkbookAsItsFirstPart(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	del := partDeleteLink.FindStringSubmatch(mustGet(t, ada, path+"/data"))[1]
	res, body := ada.post(del, confirming)
	expectRedirect(t, res, body, path+"/data")
	id := ada.workbookID("september.xlsx") // the deleted Part's Workbook stays in the Folder
	expectAll(t, mustGet(t, ada, path+"/data"), "Pick a Workbook", "september.xlsx")

	res, body = ada.pick(path+"/dataset", id)
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	if _, body = ada.second().get(link + "data.json"); !strings.Contains(body, "Beras") {
		t.Fatalf("data.json = %s, want the picked Workbook's rows", body)
	}
}

func TestAPickedWorkbookWithoutTheDatasetsColumnsIsRefusedAndNothingChanges(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := twoParts(t, ada)
	id := shelved(t, ada, "markah.xlsx", underTitleRows(t, []any{"Nama", "Markah"}, []any{"Ali", 80.0}))
	_, before := ada.second().get(link + "data.json")

	res, body := ada.pick(path+"/dataset", id, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing:")
	res, body = ada.pick(replaceLinks(t, ada, path+"/data")[0], id)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Missing:")
	if _, after := ada.second().get(link + "data.json"); after != before {
		t.Fatal("a refused pick changed data.json")
	}
	if list := partItems(t, mustGet(t, ada, path+"/data")); strings.Contains(list, "markah.xlsx") {
		t.Fatalf("a refused pick added a Part: %s", list)
	}
}

func TestALearnerCanOnlyPickTheirOwnWorkbooks(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	pageWithData(t, ada)
	adas := ada.workbookID("september.xlsx")
	bea := admin.second()
	bea.loginAsNewLearner("Bea Hopper", "bea@example.com", "correct horse battery")
	admin.assignPlan("bea@example.com", admin.onlyPlanID())
	hers := bea.createPage("Kedai Bea", myPage)

	if strings.Contains(mustGet(t, bea, hers+"/data"), "Pick a Workbook") {
		t.Fatal("a Learner with no Workbooks is offered a pick")
	}
	res, body := bea.pick(hers+"/dataset", adas)
	expectStatus(t, res, body, http.StatusNotFound, "")
	res, body = bea.pick(hers+"/dataset", "999999")
	expectStatus(t, res, body, http.StatusNotFound, "")
	if n := bea.datasetFiles(); n != 1 {
		t.Fatalf("%d stored files after Bea's refused picks, want only Ada's Part", n)
	}
}

func TestDeletingAPickedWorkbookLeavesThePartAndDataJSONAsTheyAre(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	id := shelved(t, ada, "oktober.xlsx", shopColumns(t, "Gula", 80))
	res, body := ada.pick(path+"/dataset", id, adding)
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	_, before := ada.second().get(link + "data.json")

	res, body = ada.post("/workbooks/"+id+"/delete", confirming)
	expectRedirect(t, res, body, "/my-folder")
	if _, after := ada.second().get(link + "data.json"); after != before {
		t.Fatalf("deleting the Workbook changed data.json:\n%s\nwas\n%s", after, before)
	}
	expectAll(t, mustGet(t, ada, path+"/data"), "2 Parts", "oktober.xlsx")
}

// A picked Workbook with no sheet by the recipe's name offers the sheets that fit; the choice goes back without a
// file, as the Workbook is not uploaded again.
func TestAPickedWorkbookWithoutTheRecipeSheetLetsTheLearnerPickASheetWithoutUploadingAgain(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, link := pageWithData(t, ada)
	file := testutil.Workbook(t, shopSheet("Okt", "Gula"), shopSheet("Nov", "Garam", "Lada"))
	id := shelved(t, ada, "november.xlsx", file)

	res, body := ada.pick(path+"/dataset", id, adding)
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, `value="Okt"`, `value="Nov"`, `name="workbook" value="`+id+`"`)
	if strings.Contains(body, `type="file"`) {
		t.Fatalf("the preview of a pick asks for the file again: %s", body)
	}
	res, body = ada.post(path+"/dataset", url.Values{"workbook": {id}, "mode": {"add"}, "sheet": {"Nov"}, "shown": {"Okt"}})
	expectRedirect(t, res, body, path+"/data")
	ada.confirmWork(path, nil)
	_, body = ada.second().get(link + "data.json")
	if got := perkara(t, body); strings.Join(got, ",") != "Beras,Jualan,Garam,Lada" {
		t.Fatalf("data.json Perkara = %v", got)
	}
}

// A pick goes the way of an upload, so a Workbook that is already a current Part is refused as the same file is.
func TestPickingTheWorkbookOfACurrentPartIsRefusedAsTheSameFile(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := pageWithData(t, ada)
	id := ada.workbookID("september.xlsx")

	res, body := ada.pick(path+"/dataset", id, adding)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "exactly the same file")
	expectAll(t, body, "Replace that Part")
}
