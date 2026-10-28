package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// stockReport is a stock list with its date printed in cell H2, above the column names.
func stockReport(t *testing.T, item, day string) []byte {
	t.Helper()
	return testutil.XLSX(t, []string{"Stock Balance Report"},
		[]any{nil, nil, nil, nil, nil, nil, nil, day},
		[]any{"Perkara", "Baki", "Unit"},
		[]any{item, 3, "kg"},
	)
}

// partDates lists the Part date of every row in the Deployment's data.json at link, in order.
func partDates(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, `"Part date": "`); i >= 0 {
			out = append(out, line[i+len(`"Part date": "`):][:10])
		}
	}
	return out
}

func TestAFileNamedWithADatePreviewsItFromTheFileNameAndDataJSONRowsCarryIt(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	link := ada.deployAs(path, "kedai-ada")
	visitor := ada.second()

	res, body := ada.postFile(path+"/dataset", "Stock_Balance_History_Report_20260702091058.xlsx", shopReport(t, "Beras", 120))
	expectRedirect(t, res, body, path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, `name="part-date"`, `value="2026-07-02"`, "from the file name")
	ada.switchLanguage("ms", path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, `value="2026-07-02"`, "daripada nama fail")

	ada.post(path+"/dataset/confirm", url.Values{"part-date": {"2026-07-02"}})
	_, body = visitor.get(link + "data.json")
	if got := strings.Join(partDates(body), ","); got != "2026-07-02,2026-07-02" {
		t.Fatalf("data.json Part dates = %s, want the file name's date on both rows: %s", got, body)
	}
}

func TestAFileNamedWithAMonthAndYearPreviewsTheFirstOfThatMonthFromTheFileName(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	ada.postFile(path+"/dataset", "kehadiran_062026.xlsx", shopReport(t, "Beras", 120))
	_, body := ada.get(path + "/data")
	expectAll(t, body, `name="part-date"`, `value="2026-06-01"`, "from the file name")
}

func TestAnExcelWithADateInItsTitleRowsPreviewsItFromThatCellAndAnUploadWithNoneTakesTheUploadDay(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	ada.postFile(path+"/dataset", "stok_20260702.xlsx", stockReport(t, "Gula", "01.09.2026"))
	_, body := ada.get(path + "/data")
	expectAll(t, body, `value="2026-09-01"`, "from cell H2")
	ada.switchLanguage("ms", path+"/data")
	_, body = ada.get(path + "/data")
	expectAll(t, body, `value="2026-09-01"`, "daripada sel H2")
	ada.post(path+"/dataset/confirm", nil)

	ada.postFile(path+"/dataset", "stok.xlsx", testutil.XLSX(t, []string{"Stock Balance Report"}, []any{}, []any{"Perkara", "Baki", "Unit"}, []any{"Beras", 5, "kg"}), adding)
	_, body = ada.get(path + "/data")
	expectAll(t, body, `value="`+partDate()+`"`, "hari muat naik")
}

// The Dataset module decides which dates are refused and what the rows carry; the handler answers 422 with the message,
// keeps the Part pending, and redirects once the date is accepted.
func TestAPartDateThatIsNotADateIsRefusedWithAMessageAndAnAcceptedOneRedirects(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Kedai", myPage)
	link := ada.deployAs(path, "kedai-ada")
	visitor := ada.second()
	ada.postFile(path+"/dataset", "Report_20260702091058.xlsx", shopReport(t, "Beras", 120))

	res, body := ada.post(path+"/dataset/confirm", url.Values{"part-date": {"2026-13-45"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Part date")
	if res, _ := visitor.get(link + "data.json"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("data.json after a refused confirm = %d, want 404", res.StatusCode)
	}

	res, body = ada.post(path+"/dataset/confirm", url.Values{"part-date": {"2026-07-09"}})
	expectRedirect(t, res, body, path+"/data")
}
