package app_test

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// syncNow presses Sync now on the page of the Deployment called name.
func (s *site) syncNow(name string) (*http.Response, string) {
	return s.post("/cavehost/"+name+"/sync", url.Values{})
}

// syncDue runs the step the hourly ticker runs: Sync every awake CaveHost Deployment.
func (s *site) syncDue() { s.application.SyncCaveHostDeployments(context.Background()) }

// onlineSales links the Sales Spreadsheet as the Deployment called name, confirms its sheet and returns its link.
func onlineSales(t *testing.T, ada *site, name string) string {
	t.Helper()
	res, body := ada.linkSheet(sheetLink, name)
	expectRedirect(t, res, body, "/cavehost/"+name)
	ada.confirmSheet(name, nil)
	ada.putPageOnline(name, "<!doctype html><title>Jualan</title>")
	return ada.caveHostLink(name)
}

func TestSyncNowReplacesTheRowsFromTheEditedSpreadsheetAndTheCaveHostPageShowsTheSync(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")
	if _, body := ada.get("/cavehost/jualan-ada"); !strings.Contains(body, "No Sync yet") {
		t.Fatalf("the page before any Sync lacks \"No Sync yet\": %s", body)
	}

	// Ink is deleted, Pen changes, Cap is new, and a title row is added above the header: the saved Recipe reads it all without asking.
	google.swap(testutil.Workbook(t, testutil.Sheet{Name: "Sales", Rows: [][]any{{"Kedai Runcit Ali"}, {"Item", "Qty"}, {"Pen", 4}, {"Pad", 7}, {"Cap", 9}}}))
	res, body := ada.syncNow("jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")

	day := partDate()
	want := []map[string]any{{"Item": "Pen", "Qty": 4.0, "Part date": day}, {"Item": "Pad", "Qty": 7.0, "Part date": day}, {"Item": "Cap", "Qty": 9.0, "Part date": day}}
	if got := ada.second().dataRowsOf(link); !reflect.DeepEqual(got, want) {
		t.Errorf("data.json after Sync now = %v, want the edited rows, Ink gone", got)
	}
	_, body = ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"Last Sync", "It worked", "Skipped row 1 above the header in row 2."} {
		if !strings.Contains(body, want) {
			t.Errorf("the page after Sync now lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "No Sync yet") {
		t.Errorf("the page still says no Sync yet: %s", body)
	}
	// The rows the Sync replaced are not kept: the store holds the page and the one set of rows.
	if files, _ := filepath.Glob(filepath.Join(ada.storageDir, "cavehost-*")); len(files) != 2 {
		t.Errorf("the store holds %v after a Sync, want the page and one set of rows", files)
	}
}

// answerWith makes the Spreadsheet answer every request with this status, 0 to serve its file again.
func (g *fakeGoogle) answerWith(status int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = status
}

// holdAnswers makes every answer wait until the returned function is called.
func (g *fakeGoogle) holdAnswers() (release func()) {
	hold := make(chan struct{})
	g.mu.Lock()
	g.hold = hold
	g.mu.Unlock()
	return func() { close(hold) }
}

// otherSheetLink is the link to a second Spreadsheet.
const otherSheetLink = "https://docs.google.com/spreadsheets/d/2ZzZ-other_9/edit?gid=0#gid=0"

func TestTheHourlySyncReadsEachAwakeDeploymentOldestSyncFirstAndLeavesASleepingOneAlone(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	for _, d := range []struct{ name, link string }{{"jualan-a", sheetLink}, {"jualan-b", otherSheetLink}} {
		res, body := ada.linkSheet(d.link, d.name)
		expectRedirect(t, res, body, "/cavehost/"+d.name)
		ada.confirmSheet(d.name, nil)
	}
	linkA := ada.caveHostLink("jualan-a")
	ada.putPageOnline("jualan-a", "<!doctype html>")
	// A has been Synced; B never has, so the hourly Sync starts with B.
	res, body := ada.syncNow("jualan-a")
	expectRedirect(t, res, body, "/cavehost/jualan-a")
	asked := len(google.requests())

	google.swap(testutil.Workbook(t, testutil.Sheet{Name: "Sales", Rows: [][]any{{"Item", "Qty"}, {"Pen", 4}}}))
	ada.syncDue()
	if got, want := google.requests()[asked:], []string{"/spreadsheets/d/2ZzZ-other_9/export?format=xlsx", "/spreadsheets/d/1AbC-dEf_123/export?format=xlsx"}; !slices.Equal(got, want) {
		t.Errorf("the hourly Sync asked Google for %q, want B, never Synced, then A", got)
	}
	want := []map[string]any{{"Item": "Pen", "Qty": 4.0, "Part date": partDate()}}
	if got := ada.second().dataRowsOf(linkA); !reflect.DeepEqual(got, want) {
		t.Errorf("data.json after the hourly Sync = %v, want %v", got, want)
	}
	if _, body := ada.get("/cavehost/jualan-b"); !strings.Contains(body, "It worked") {
		t.Errorf("the page of B after the hourly Sync does not say it worked: %s", body)
	}

	// Once the Learner's CaveHost Plan period has ended the Deployments sleep, and nothing Syncs them.
	asked = len(google.requests())
	ada.ageAllCaveHostPeriods(40)
	ada.syncDue()
	if got := google.requests()[asked:]; len(got) != 0 {
		t.Errorf("the hourly Sync asked Google for %q for sleeping Deployments, want nothing", got)
	}
}

func TestASyncThatFailsKeepsTheRowsAndTheCaveHostPageSaysWhyUntilASyncWorks(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	link := onlineSales(t, ada, "jualan-ada")
	before := ada.second().dataRowsOf(link)

	google.answerWith(http.StatusForbidden) // the Spreadsheet was made private
	res, body := ada.syncNow("jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	_, body = ada.get("/cavehost/jualan-ada")
	for _, want := range []string{"It failed", "Cave cannot open this Spreadsheet"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page after a failed Sync lacks %q: %s", want, body)
		}
	}
	if got := ada.second().dataRowsOf(link); !reflect.DeepEqual(got, before) {
		t.Errorf("data.json after a failed Sync = %v, want the last rows %v", got, before)
	}

	google.answerWith(0)
	ada.syncDue()
	_, body = ada.get("/cavehost/jualan-ada")
	if !strings.Contains(body, "It worked") || strings.Contains(body, "It failed") {
		t.Errorf("the page after a Sync that works still shows the failure: %s", body)
	}
}

func TestAtMostOneSyncRunsPerDeploymentAtATime(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	onlineSales(t, ada, "jualan-ada")
	asked := len(google.requests())

	release := google.holdAnswers()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ada.syncNow("jualan-ada")
	}()
	for deadline := time.Now().Add(5 * time.Second); len(google.requests()) == asked && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	// The first Sync waits on Google: pressing Sync now again, and the hourly Sync, start no second one.
	res, body := ada.syncNow("jualan-ada")
	expectRedirect(t, res, body, "/cavehost/jualan-ada")
	ada.syncDue()
	if got := len(google.requests()) - asked; got != 1 {
		t.Errorf("Google was asked %d times while one Sync ran, want 1", got)
	}
	release()
	<-done
}
