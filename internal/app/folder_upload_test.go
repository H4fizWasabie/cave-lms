package app_test

import (
	"net/http"
	"strings"
	"testing"
)

const folderUploadForm = `action="/workbooks"`

func TestAWorkbookUploadedInTheFolderIsListedAndDownloadsByteForByte(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	_, body := ada.getFolder()
	if !strings.Contains(body, folderUploadForm) {
		t.Fatalf("a Learner with a Plan sees no upload form in the Folder: %s", body)
	}
	file := shopReport(t, "Beras", 120)
	res, body := ada.postFile("/workbooks", "laporan.xlsx", file)
	expectRedirect(t, res, body, "/my-folder")

	res, body = ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
	res, got := ada.get(workbookLink.FindStringSubmatch(body)[1])
	expectStatus(t, res, got, http.StatusOK, "")
	if got != string(file) {
		t.Fatalf("the download is %d bytes, want the %d uploaded", len(got), len(file))
	}
	if n := ada.storedFiles(); n != 1 {
		t.Fatalf("%d stored files, want only the Workbook: a Folder upload has no Clean-up", n)
	}
}

func TestAFolderUploadThatIsNotExcelOrIsTooBigIsRefusedWithAMessage(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"nota.csv", []byte("a,b"), "nota.csv is not an .xlsx file"},
		{"lama.xls", []byte("x"), "Save As"},
		{"besar.xlsx", make([]byte, 11<<20), "The file is over 10 MB"},
		{"kosong.xlsx", nil, "Choose an Excel file"},
	} {
		res, body := ada.postFile("/workbooks", c.name, c.data)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	if _, body := ada.getFolder(); strings.Contains(body, "/workbooks/") {
		t.Fatalf("a refused upload is listed: %s", body)
	}
	ada.switchLanguage("ms", "/")
	res, body := ada.postFile("/workbooks", "nota.csv", []byte("a,b"))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "bukan fail .xlsx")
}

func TestAFolderUploadThatDoesNotFitTheStorageAllowanceIsRefusedWithTheStorageFullMessage(t *testing.T) {
	t.Parallel()
	_, ada, _ := storageSite(t, "1")
	res, body := ada.postFile("/workbooks", "satu.xlsx", bigFinance(t, 1))
	expectRedirect(t, res, body, "/my-folder")
	res, body = ada.postFile("/workbooks", "dua.xlsx", bigFinance(t, 2))
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "would take you over your Storage allowance")
	if n := len(workbookLink.FindAllString(func() string { _, b := ada.getFolder(); return b }(), -1)); n != 1 {
		t.Fatalf("%d Workbooks listed, want only the one that fit", n)
	}
}

func TestWithoutAnActivePlanThereIsNoUploadFormAndAPostedUploadIsRefused(t *testing.T) {
	t.Parallel()
	admin, ada, planID, _ := labSetup(t)
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "")
	if strings.Contains(body, folderUploadForm) {
		t.Fatalf("a free Learner is offered an upload: %s", body)
	}
	res, body = ada.postFile("/workbooks", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectStatus(t, res, body, http.StatusForbidden, "")

	admin.assignPlan("ada@example.com", planID)
	res, body = ada.postFile("/workbooks", "laporan.xlsx", shopReport(t, "Beras", 120))
	expectRedirect(t, res, body, "/my-folder")
	admin.ageAllPeriods(31) // the Plan ended
	res, body = ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "laporan.xlsx")
	if strings.Contains(body, folderUploadForm) {
		t.Fatalf("a Learner whose Plan ended is offered an upload: %s", body)
	}
	res, body = ada.postFile("/workbooks", "lagi.xlsx", shopReport(t, "Gula", 120))
	expectStatus(t, res, body, http.StatusForbidden, "")
}

func TestTheFolderUploadFormIsInMalay(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	ada.switchLanguage("ms", "/")
	res, body := ada.getFolder()
	expectStatus(t, res, body, http.StatusOK, "Muat naik Buku kerja")
}
