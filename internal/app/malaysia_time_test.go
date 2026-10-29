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
	expectAll(t, body, knownEnglish)
	_, body = ada.getFolderAsking("ms")
	expectAll(t, body, knownMalay)
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
