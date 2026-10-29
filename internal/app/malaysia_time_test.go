package app_test

import (
	"context"
	"strings"
	"testing"
)

// The known instant: 06:20 UTC is 14:20 in Malaysia (UTC+8), on the same date.
const (
	knownInstant   = "2026-03-05 06:20:00+00"
	knownEnglish   = "5 March 2026, 14:20"
	knownMalay     = "5 Mac 2026, 14:20"
	knownTakenDown = "2026-03-05 17:30:00+00" // 01:30 on the 6th in Malaysia: the date moves too
	takenEnglish   = "6 March 2026, 01:30"
)

func (s *site) exec(query string) {
	s.t.Helper()
	if _, err := s.db.ExecContext(context.Background(), query); err != nil {
		s.t.Fatal(err)
	}
}

func TestAWorkbooksUploadTimeOnTheFolderIsMalaysiaTimeInThePagesLanguage(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	ada.postFile("/workbooks", "laporan.xlsx", shopReport(t, "Beras", 120))
	ada.exec(`UPDATE workbooks SET uploaded_at = '` + knownInstant + `'`)

	_, body := ada.getFolder()
	expectAll(t, body, `<td class="when" data-label="Uploaded"><span>5 March 2026<small>14:20</small></span></td>`)
	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, `<td class="when" data-label="Dimuat naik"><span>5 Mac 2026<small>14:20</small></span></td>`)
	if strings.Contains(body, "UTC") {
		t.Fatalf("the Folder still shows UTC: %s", body)
	}
}

func TestADashboardPagesUploadTimeOnTheFolderIsMalaysiaTime(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	ada.createPage("Jualan", myPage)
	ada.exec(`UPDATE dashboard_pages SET uploaded_at = '` + knownInstant + `'`)

	_, body := ada.getFolder()
	expectAll(t, body, knownEnglish)
	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, knownMalay)
}

func TestDeploymentTimesAreMalaysiaTimeInThePagesLanguage(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	ada.deploy(path)
	ada.exec(`UPDATE deployments SET deployed_at = '` + knownInstant + `'`)

	_, body := ada.get(path + "/deploy")
	expectAll(t, body, knownEnglish)
	if strings.Contains(body, "UTC") {
		t.Fatalf("the Deploy tab still shows UTC: %s", body)
	}
	ada.switchLanguage("ms", "/")
	_, body = ada.get(path + "/deploy")
	expectAll(t, body, knownMalay)

	_, body = admin.get("/admin/deployments")
	admin.post(takedownAction.FindStringSubmatch(body)[1], map[string][]string{"reason": {"Aduan"}})
	admin.exec(`UPDATE deployments SET taken_down_at = '` + knownTakenDown + `'`)
	ada.switchLanguage("en", "/")
	_, body = ada.get(path + "/deploy")
	expectAll(t, body, takenEnglish)
	_, body = admin.get("/admin/deployments")
	expectAll(t, body, takenEnglish)
}

func TestTheLabDashboardPagesTabShowsEveryPagesUploadTimeInMalaysiaTime(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	ada.uploadPage(path, versionOf(2)) // the first is an earlier page now
	ada.exec(`UPDATE dashboard_pages SET uploaded_at = '` + knownInstant + `'`)

	_, body := ada.get(path)
	if n := strings.Count(body, "Uploaded "+knownEnglish); n != 2 {
		t.Fatalf("%d pages show Uploaded %s, want 2 (current and earlier): %s", n, knownEnglish, body)
	}
	if strings.Contains(body, "UTC") {
		t.Fatalf("the Pages tab still shows UTC: %s", body)
	}
	_, body = ada.getAsking(path, "ms")
	expectAll(t, body, "Dimuat naik "+knownMalay)
}

func TestTheLabShowsThePlanEndAndWhenADashboardChangedInMalaysiaTime(t *testing.T) {
	t.Parallel()
	_, ada := labLearner(t)
	path := ada.createPage("Jualan", versionOf(1))
	ada.exec(`UPDATE plan_periods SET ends_at = '2099-03-05 06:20:00+00'`)
	ada.exec(`UPDATE dashboards SET updated_at = '` + knownInstant + `'`)

	_, body := ada.get("/lab")
	expectAll(t, body, "Ends 5 March 2099, 14:20", "Changed "+knownEnglish)
	if strings.Contains(body, "UTC") {
		t.Fatalf("the Lab still shows UTC: %s", body)
	}
	_, body = ada.getAsking("/lab", "ms")
	expectAll(t, body, "Tamat 5 Mac 2099, 14:20", "Diubah "+knownMalay)

	_, body = ada.get(path) // the Dashboard's own page says when its data changed
	if strings.Contains(body, "UTC") {
		t.Fatalf("the Dashboard page still shows UTC: %s", body)
	}
}
