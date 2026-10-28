package app_test

import (
	"net/url"
	"strings"
	"testing"
)

func TestTheDataTabsUploadFormAndWorkbookPickerDefaultToAddOnceTheDatasetHasParts(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path, _ := pageWithData(t, ada)
	shelved(t, ada, "oktober.xlsx", shopColumns(t, "Gula", 80))

	// Both forms: the upload form and the Workbook picker.
	body := mustGet(t, ada, path+"/data")
	if n := strings.Count(body, `name="mode" value="add" checked`); n != 2 {
		t.Fatalf("%d of the upload form and Workbook picker default to Add to data, want 2: %s", n, body)
	}
	if strings.Contains(body, `name="mode" value="replace" checked`) {
		t.Fatalf("a Dataset with a Part defaults to Replace data: %s", body)
	}

	// A pending Replace keeps its own mode when its preview is shown again.
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopColumns(t, "Garam", 70), url.Values{"mode": {"replace"}})
	expectRedirect(t, res, body, path+"/data")
	body = mustGet(t, ada, path+"/data")
	expectAll(t, body, `name="mode" value="replace" checked`)
	if strings.Contains(body, `name="mode" value="add" checked`) {
		t.Fatalf("a pending Replace shows Add to data selected: %s", body)
	}
}
