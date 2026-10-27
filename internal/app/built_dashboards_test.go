package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// build makes a Dashboard from name and file by Build from Excel and returns its path, its Dataset pending.
func (s *site) build(name string, file []byte) string {
	s.t.Helper()
	res, body := s.postFile("/lab/dashboards/build", "laporan.xlsx", file, url.Values{"name": {name}})
	expectStatus(s.t, res, body, http.StatusSeeOther, "")
	loc := res.Header.Get("Location")
	if !dashboardPath.MatchString(loc) {
		s.t.Fatalf("Build from Excel redirects to %q", loc)
	}
	return loc
}

// countDashboards is how many Dashboards there are.
func (s *site) countDashboards() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM dashboards`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// New Dashboard offers two ways in both languages (ADR 0019): Build from Excel, a name and an .xlsx, and Paste your
// own page.
func TestNewDashboardOffersBuildFromExcelAndPasteYourOwnPage(t *testing.T) {
	_, ada := labLearner(t)
	for lang, words := range map[string][]string{
		"en": {"Build from Excel", "Paste your own page"},
		"ms": {"Bina daripada Excel", "Tampal halaman anda sendiri"},
	} {
		ada.switchLanguage(lang, "/lab/new")
		res, body := ada.get("/lab/new")
		expectAll(t, body, append(words, `<html lang="`+lang+`">`, `action="/lab/dashboards/build"`, `accept=".xlsx"`,
			`action="/lab/dashboards"`, `name="html"`)...)
		expectStatus(t, res, body, http.StatusOK, "")
		for _, gone := range []string{"/lab/samples/", `name="template"`, `name="sample"`} {
			if strings.Contains(body, gone) {
				t.Errorf("New Dashboard in %s offers %s", lang, gone)
			}
		}
	}
}

// Build from Excel cleans the upload and previews it, with no Template pick, Sample or page to paste; the confirm
// builds the blocks by the General rules and opens the Preview, in Corporate Executive with the five Themes to pick.
func TestBuildFromExcelPreviewsTheCleanUpAndOpensThePreviewOnConfirm(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.build("Kedai", shopReport(t, "Beras", 120))

	res, body := ada.get(path)
	expectStatus(t, res, body, http.StatusOK, "")
	expectAll(t, body, "Skipped rows 1 to 3 above the header in row 4.", "Beras", `action="`+path+`/dataset/confirm"`)
	for _, gone := range []string{"?template=", `name="template"`, "/lab/samples/", `action="` + path + `/page"`, `name="sample"`} {
		if strings.Contains(body, gone) {
			t.Errorf("a built Dashboard's preview offers %s:\n%s", gone, body)
		}
	}

	res, body = ada.post(path+"/dataset/confirm", url.Values{})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != path+"/preview" {
		t.Fatalf("the confirm redirects to %q, want the Preview", loc)
	}
	res, body = ada.get(path + "/preview")
	expectStatus(t, res, body, http.StatusOK, `<option value="corporate" selected>Corporate Executive</option>`)
	if n := strings.Count(body, "<option value="); n != 5 {
		t.Fatalf("the Theme select offers %d Themes, want 5:\n%s", n, body)
	}
	var template string
	if err := ada.db.QueryRow(`SELECT template FROM dashboards WHERE id = $1`, idOf(path)).Scan(&template); err != nil {
		t.Fatal(err)
	}
	if template != "general" {
		t.Fatalf("a built Dashboard's Template is %q, want general", template)
	}
	if theme, blocks := ada.drawn(path + "/view/"); theme != "corporate" || len(blocks) == 0 {
		t.Fatalf("the built Dashboard draws theme %q, blocks %q", theme, blocks)
	}
}

// A refused build saves nothing and says why, on New Dashboard.
func TestARefusedBuildSavesNothing(t *testing.T) {
	_, ada := labLearner(t)
	for _, c := range []struct {
		name, file string
		content    []byte
		want       string
	}{
		{"", "laporan.xlsx", shopReport(t, "Beras", 120), "Give the Dashboard a name."},
		{"Kedai", "laporan.xls", []byte("old"), ".xls"},
		{"Kedai", "laporan.csv", []byte("a,b"), "laporan.csv"},
		{"Kedai", "laporan.xlsx", []byte("not a workbook"), "laporan.xlsx"},
	} {
		res, body := ada.postFile("/lab/dashboards/build", c.file, c.content, url.Values{"name": {c.name}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, `action="/lab/dashboards/build"`)
	}
	if n := ada.countDashboards(); n != 0 {
		t.Fatalf("refused builds left %d Dashboards", n)
	}

	// Without a Plan, nothing is built.
	_, bob, _, _ := labSetup(t)
	res, body := bob.postFile("/lab/dashboards/build", "laporan.xlsx", shopReport(t, "Beras", 120), url.Values{"name": {"Kedai"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	if n := bob.countDashboards(); n != 0 {
		t.Fatalf("a build without a Plan left %d Dashboards", n)
	}
}

// The Lab list names each Dashboard's kind, and a built one's Theme.
func TestTheLabListShowsBuiltWithItsThemeOrOwnPage(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.build("Kedai", shopReport(t, "Beras", 120))
	ada.confirmWork(path, url.Values{})
	ada.post(path+"/theme", url.Values{"theme": {"midnight"}})
	res, body := ada.post("/lab/dashboards", url.Values{"name": {"Halaman"}, "html": {myPage}})
	expectStatus(t, res, body, http.StatusSeeOther, "")

	_, body = ada.get("/lab")
	expectAll(t, body, "<li>Built · Midnight Intelligence</li>", "<li>Own page</li>")
	ada.switchLanguage("ms", "/lab")
	_, body = ada.get("/lab")
	expectAll(t, body, "<li>Dibina · Midnight Intelligence</li>", "<li>Halaman sendiri</li>")
}

// A built Dashboard's Deployment draws its current Dataset in its current Theme: a Theme saved, a Replace and an Add
// all reach the live page, and a restart leaves it built.
func TestABuiltDashboardsLivePageFollowsItsThemeAndItsNewData(t *testing.T) {
	_, ada := labLearner(t)
	path := ada.build("Kedai", shopReport(t, "Beras", 120))
	ada.confirmWork(path, url.Values{})
	link := ada.deploy(path)
	visitor := ada.second()
	visitor.client.Jar = nil
	if theme, _ := visitor.drawn(link); theme != "corporate" {
		t.Fatalf("the live page draws %q, want corporate", theme)
	}

	ada.post(path+"/theme", url.Values{"theme": {"midnight"}})
	if theme, _ := visitor.drawn(link); theme != "midnight" {
		t.Fatalf("after saving a Theme the live page draws %q, want midnight", theme)
	}

	// Replace: the routine monthly workbook goes live at once.
	res, body := ada.postFile(path+"/dataset", "november.xlsx", shopReport(t, "Gula", 80), url.Values{"mode": {"replace"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if rows := visitor.rowsOf(link); !strings.Contains(rows, "Gula") || strings.Contains(rows, "Beras") {
		t.Fatalf("after a Replace the live page reads\n%s", rows)
	}

	// Add: shown for a check, as its rows join the ones there, then live.
	_, body = ada.get(path)
	expectAll(t, body, `name="mode" value="replace" checked`, `name="mode" value="add"`, "Replace data", "Add to data")
	res, body = ada.postFile(path+"/dataset", "tambah.xlsx", shopReport(t, "Tepung", 40), adding)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get(path)
	expectAll(t, body, `name="mode" value="add" checked`, `action="`+path+`/dataset/confirm"`)
	ada.confirmWork(path, url.Values{})
	if rows := visitor.rowsOf(link); !strings.Contains(rows, "Gula") || !strings.Contains(rows, "Tepung") {
		t.Fatalf("after an Add the live page reads\n%s", rows)
	}

	ada.restart()
	var pages int
	if err := ada.db.QueryRow(`SELECT count(*) FROM dashboard_pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages != 0 {
		t.Fatalf("a restart gave the built Dashboard %d pages", pages)
	}
	if theme, _ := visitor.drawn(link); theme != "midnight" {
		t.Fatalf("after a restart the live page draws %q, want midnight", theme)
	}
}

func idOf(path string) string { return path[strings.LastIndex(path, "/")+1:] }
