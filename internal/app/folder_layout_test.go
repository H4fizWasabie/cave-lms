package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// adaWithTwoWorkbooks has a Plan (1 GB), a Dashboard built from finance-sample.xlsx and a second, unused Workbook.
func adaWithTwoWorkbooks(t *testing.T) (admin, ada *site, dashboard string) {
	t.Helper()
	admin, ada = labLearner(t)
	dashboard = ada.readyDashboard("Kewangan")
	ada.postFile("/workbooks", "rak.xlsx", shopReport(t, "Gula", 80))
	return admin, ada, dashboard
}

func TestTheFolderRailShowsStorageUsedOfTheAllowanceAndSectionLinksWithCounts(t *testing.T) {
	t.Parallel()
	_, ada, _ := adaWithTwoWorkbooks(t)
	ada.createPage("Jualan", myPage)
	_, body := ada.getFolder()
	expectAll(t, body, "<meter", storageText(ada.usedBytes())+" of 1 GB",
		`<a href="#workbooks">Workbooks <span class="folder-count num">2</span></a>`,
		`<a href="#dashboard-pages">Dashboard pages <span class="folder-count num">1</span></a>`)
	if n := strings.Count(body, `action="/workbooks"`); n != 1 {
		t.Errorf("%d Upload forms, want 1: %s", n, body)
	}
	if strings.Contains(body, `href="#submissions"`) {
		t.Errorf("the rail links to a Submissions section that is not there: %s", body)
	}
}

func TestTheFolderShowsOnlyTheAmountUsedWithoutAnActivePlan(t *testing.T) {
	t.Parallel()
	admin, ada, _ := adaWithTwoWorkbooks(t)
	admin.ageAllPeriods(31)
	_, body := ada.getFolder()
	expectAll(t, body, storageText(ada.usedBytes())+" used")
	for _, not := range []string{"<meter", " of 1 GB", folderUploadForm} {
		if strings.Contains(body, not) {
			t.Errorf("an ended Plan's Folder shows %q: %s", not, body)
		}
	}
}

func TestTheEmptyFolderOffersTheDropAreaAsItsOneAction(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	_, body := ada.getFolder()
	expectAll(t, body, `class="folder-drop"`, "0 KB of 1 GB")
	if n := strings.Count(body, `action="/workbooks"`); n != 1 {
		t.Errorf("%d Upload forms, want 1: %s", n, body)
	}
	if strings.Contains(body, `href="#workbooks"`) {
		t.Errorf("the empty Folder has a section index: %s", body)
	}
	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, "0 KB daripada 1 GB", "Lepaskan fail Excel di sini")
}

func TestTheWorkbooksTableHasHeadersUsedOnLinksUnusedAndAQuietDeleteToTheConfirmationPage(t *testing.T) {
	t.Parallel()
	_, ada, dashboard := adaWithTwoWorkbooks(t)
	_, body := ada.getFolder()
	expectAll(t, body, `<th scope="col">Name</th>`, `<th scope="col">Size</th>`, `<th scope="col">Uploaded (MYT)</th>`,
		`<th scope="col">Used on</th>`, `<a href="`+dashboard+`">Kewangan</a>`, `<span class="muted">Unused</span>`)
	deletes := regexp.MustCompile(`<a class="btn btn-danger btn-sm" href="/workbooks/\d+/delete"`).FindAllString(body, -1)
	if len(deletes) != 2 {
		t.Errorf("%d quiet Delete links, want 2: %s", len(deletes), body)
	}
	res, page := ada.get(workbookDeleteLink.FindStringSubmatch(body)[1])
	expectStatus(t, res, page, http.StatusOK, "")

	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, `<th scope="col">Nama</th>`, `<th scope="col">Digunakan pada</th>`, "Tidak digunakan", "Padam")
}

func TestTheAdminReadsStorageInALearnersFolderButCannotUploadOrDelete(t *testing.T) {
	t.Parallel()
	admin, ada, _ := adaWithTwoWorkbooks(t)
	res, body := admin.get("/my-folder/ada-example-com")
	expectStatus(t, res, body, http.StatusOK, storageText(ada.usedBytes())+" of 1 GB")
	for _, not := range []string{folderUploadForm, "/delete", "folder-drop"} {
		if strings.Contains(body, not) {
			t.Errorf("the Admin's read-only Folder shows %q: %s", not, body)
		}
	}
}
