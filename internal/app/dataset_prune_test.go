package app_test

import (
	"context"
	"net/http"
	"testing"
)

// prune runs the daily prune now.
func (s *site) prune() {
	s.t.Helper()
	s.application.RunDailyPrune(context.Background())
}

// lastLogLine returns the newest System log line containing part, or "".
func (s *site) lastLogLine(part string) string {
	s.t.Helper()
	var msg string
	err := s.db.QueryRowContext(context.Background(),
		`SELECT message FROM system_log WHERE message LIKE '%' || $1 || '%' ORDER BY id DESC LIMIT 1`, part).Scan(&msg)
	if err != nil {
		s.t.Fatalf("no System log line with %q: %v", part, err)
	}
	return msg
}

func TestNinetyDaysAfterTheLastPeriodEndsTheLearnersDatasetsAreDeleted(t *testing.T) {
	admin, ada := labLearner(t)
	path := ada.readyDashboard("Kewangan")
	link := ada.deploy(path)
	admin.readyDashboard("Demo") // the Admin's Datasets are never pruned
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("%d stored files, want 2", n)
	}

	admin.ageAllPeriods(30 + 89) // the 30-day period ended 89 days ago
	admin.prune()
	if n := ada.datasetFiles(); n != 2 {
		t.Fatalf("at 89 days %d stored files, want 2", n)
	}
	if line := admin.lastLogLine("Parts"); line != "Daily prune deleted 0 Parts" {
		t.Fatalf("System log line = %q", line)
	}

	admin.ageAllPeriods(2) // 91 days
	admin.prune()
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("at 91 days %d stored files, want the Admin's 1", n)
	}
	if line := admin.lastLogLine("Parts"); line != "Daily prune deleted 1 Parts" {
		t.Fatalf("System log line = %q", line)
	}
	// The Dashboard stays, without its Dataset. Lapsed, Ada can't open the Lab (it sends her to /pricing), so ask the database.
	var dashboards, parts int
	if err := admin.db.QueryRow(`SELECT count(*), (SELECT count(*) FROM dataset_parts ds JOIN dashboards d ON d.id = ds.dashboard_id
		WHERE d.name = 'Kewangan') FROM dashboards WHERE name = 'Kewangan'`).Scan(&dashboards, &parts); err != nil {
		t.Fatal(err)
	}
	if dashboards != 1 || parts != 0 {
		t.Fatalf("Kewangan: %d Dashboards and %d Parts, want 1 and 0", dashboards, parts)
	}
	res, body := ada.second().get(link)
	expectStatus(t, res, body, http.StatusOK, "This Dashboard is asleep")
}

func TestALearnerWhoRenewedOrHasAnUpcomingPeriodKeepsTheirDatasets(t *testing.T) {
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")

	// An upcoming period follows the current one; then the current one is pushed 200 days into the past.
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	if _, err := admin.db.Exec(`UPDATE plan_periods SET starts_at = starts_at - interval '200 days',
		ends_at = ends_at - interval '200 days' WHERE starts_at <= now()`); err != nil {
		t.Fatal(err)
	}
	admin.prune()
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("with an upcoming period %d stored files, want 1", n)
	}

	admin.ageAllPeriods(400) // both periods long over
	admin.assignPlan("ada@example.com", admin.onlyPlanID())
	admin.prune()
	if n := ada.datasetFiles(); n != 1 {
		t.Fatalf("after renewing %d stored files, want 1", n)
	}
}

func TestNinetyDaysAfterTheLastPeriodEndsTheLearnersWorkbooksAreDeletedToo(t *testing.T) {
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	link := ada.workbookPath()
	admin.readyDashboard("Demo") // the Admin's Workbooks are never pruned
	workbooks := func() (adas, admins int) {
		t.Helper()
		if err := admin.db.QueryRow(`SELECT count(*) FILTER (WHERE l.email = 'ada@example.com'), count(*) FILTER (WHERE l.email <> 'ada@example.com')
			FROM workbooks wb JOIN learners l ON l.id = wb.learner_id`).Scan(&adas, &admins); err != nil {
			t.Fatal(err)
		}
		return adas, admins
	}

	admin.ageAllPeriods(30 + 89)
	admin.prune()
	if adas, admins := workbooks(); adas != 1 || admins != 1 {
		t.Fatalf("at 89 days Ada has %d Workbooks and the Admin %d, want 1 and 1", adas, admins)
	}
	admin.ageAllPeriods(2) // 91 days
	admin.prune()
	if adas, admins := workbooks(); adas != 0 || admins != 1 {
		t.Fatalf("at 91 days Ada has %d Workbooks and the Admin %d, want 0 and 1", adas, admins)
	}
	if res, body := admin.get(link); res.StatusCode != http.StatusNotFound {
		t.Fatalf("Ada's pruned Workbook: status %d, body %.200s", res.StatusCode, body)
	}
}
