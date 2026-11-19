package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

// deploymentCount is how many Deployments, Lab and CaveHost, the site has.
func (s *site) deploymentCount() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM deployments`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestOnlyAGoogleSpreadsheetLinkIsAcceptedAndNothingElseIsEverFetched(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	for _, link := range []string{
		"",
		"not a link",
		"http://docs.google.com/spreadsheets/d/1AbC-dEf_123/edit",
		"https://evil.example/spreadsheets/d/1AbC-dEf_123/edit",
		"https://docs.google.com.evil.example/spreadsheets/d/1AbC-dEf_123/edit",
		"https://docs.google.com@evil.example/spreadsheets/d/1AbC-dEf_123/edit",
		"https://docs.google.com:8443/spreadsheets/d/1AbC-dEf_123/edit",
		"https://docs.google.com/document/d/1AbC-dEf_123/edit",
		"https://docs.google.com/spreadsheets/d/",
		"https://docs.google.com/spreadsheets/d/../../etc/passwd",
		"https://docs.google.com/spreadsheets/1AbC-dEf_123/edit",
		"https://drive.google.com/spreadsheets/d/1AbC-dEf_123/edit",
		"file:///etc/passwd",
		google.server.URL + "/spreadsheets/d/1AbC-dEf_123/edit",
	} {
		res, body := ada.linkSheet(link, "jualan-ada")
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "That is not a Google Spreadsheet link")
		if got := google.requests(); len(got) != 0 {
			t.Fatalf("pasting %q made Cave fetch %v, want nothing fetched", link, got)
		}
	}
	if n := ada.deploymentCount(); n != 0 {
		t.Fatalf("refused links made %d Deployments", n)
	}
}

func TestEveryKindOfGoogleSpreadsheetLinkReadsTheSameID(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	for i, link := range []string{
		"https://docs.google.com/spreadsheets/d/1AbC-dEf_123",
		"https://docs.google.com/spreadsheets/d/1AbC-dEf_123/",
		"https://docs.google.com/spreadsheets/d/1AbC-dEf_123/edit?usp=sharing",
		"https://docs.google.com/spreadsheets/d/1AbC-dEf_123/edit#gid=7",
		"  https://DOCS.google.com/spreadsheets/d/1AbC-dEf_123/htmlview  ",
	} {
		res, body := ada.linkSheet(link, "jualan-"+strings.Repeat("a", i+1))
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	for _, got := range google.requests() {
		if got != "/spreadsheets/d/1AbC-dEf_123/export?format=xlsx" {
			t.Errorf("Google was asked for %q, want the export of the ID alone", got)
		}
	}
}

func TestRedirectsAreFollowedOnlyToGoogle(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))

	// A redirect inside Google, to where it serves the file, is followed.
	google.mu.Lock()
	google.to = "/moved/file.xlsx"
	google.mu.Unlock()
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	// A redirect to another host, a sign-in page or any server of the Learner's choosing, is not.
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		_, _ = w.Write(salesSheet(t))
	}))
	t.Cleanup(other.Close)
	for _, to := range []string{other.URL + "/file.xlsx", "https://accounts.google.com/ServiceLogin", "https://docs.google.com.evil.example/x", "https://googleusercontent.com.evil.example/x"} {
		google.mu.Lock()
		google.to = to
		google.mu.Unlock()
		res, body := ada.linkSheet(sheetLink, "jualan-bea")
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Cave cannot open this Spreadsheet")
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("Cave followed a redirect to another host %d times", n)
	}
	if n := ada.deploymentCount(); n != 1 {
		t.Fatalf("%d Deployments, want only the first", n)
	}
}

func TestASpreadsheetCaveCannotOpenGetsTheStepsToShareIt(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	for name, answer := range map[string]func(){
		"forbidden":    func() { google.status = http.StatusForbidden },
		"not found":    func() { google.status = http.StatusNotFound },
		"sign-in page": func() { google.status, google.file = 0, []byte("<!doctype html><title>Sign in</title>") },
	} {
		google.mu.Lock()
		google.status, google.file = 0, nil
		answer()
		google.mu.Unlock()
		res, body := ada.linkSheet(sheetLink, "jualan-ada")
		expectStatus(t, res, body, http.StatusUnprocessableEntity, "Cave cannot open this Spreadsheet")
		for _, step := range []string{"Share", "Anyone with the link", "Viewer"} {
			if !strings.Contains(body, step) {
				t.Errorf("%s: the message lacks the step %q: %s", name, step, body)
			}
		}
	}
	if n := ada.deploymentCount(); n != 0 {
		t.Fatalf("%d Deployments after refusals, want none", n)
	}

	google.mu.Lock()
	google.status = http.StatusInternalServerError
	google.mu.Unlock()
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusBadGateway, "Cave could not reach Google")
}

func TestASpreadsheetOver50MBIsCutOffWithAClearMessage(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, nil)
	google.mu.Lock()
	google.file = append([]byte("PK"), make([]byte, 50<<20)...) // one byte over
	google.mu.Unlock()

	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "over 50 MB")
	if n := ada.deploymentCount(); n != 0 {
		t.Fatalf("%d Deployments after a Spreadsheet over the cap, want none", n)
	}
}

func TestADeploymentNameFollowsTheLabsRulesAndIsUniqueAcrossLabAndCaveHost(t *testing.T) {
	t.Parallel()
	google := newFakeGoogle(t, salesSheet(t))
	admin, ada := labLearner(t, func(c *config.Config) { c.GoogleSheetsBaseURL = google.server.URL })
	admin.assignCaveHost("ada@example.com", 30)
	ada.deployAs(ada.readyDashboard("Kewangan"), "kedai-ada")

	for name, want := range map[string]string{
		"ab":          "3 to 40",
		"Jualan-Ada":  "lowercase letters",
		"jualan--ada": "lowercase letters",
		"admin":       "reserved",
		"kedai-ada":   "already taken", // a Lab Deployment's
	} {
		res, body := ada.linkSheet(sheetLink, name)
		expectStatus(t, res, body, http.StatusUnprocessableEntity, want)
	}
	if got := google.requests(); len(got) != 0 {
		t.Fatalf("a refused name made Cave fetch %v, want nothing fetched", got)
	}

	// The other way round: a CaveHost Deployment's name is taken in the Lab.
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	path := ada.readyDashboard("Jualan")
	res, body = ada.post(path+"/deploy", map[string][]string{"confirm": {"yes"}, "name": {"jualan-ada"}, "right": {"yes"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "already taken")
	// And the same name is refused to CaveHost again.
	res, body = ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "already taken")
}

func TestLinkingNeedsTheTickThatTheDataMayBeShownAndWarnsAboutIt(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))

	res, body := ada.get("/cavehost")
	expectStatus(t, res, body, http.StatusOK, "Anyone with the link can see this data")
	res, body = ada.post("/cavehost", map[string][]string{"link": {sheetLink}, "name": {"jualan-ada"}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "Tick that you have the right")
	if got := google.requests(); len(got) != 0 {
		t.Fatalf("an unticked form made Cave fetch %v", got)
	}
}

func TestCaveHostScreensAreForCaveHostLearnersAlone(t *testing.T) {
	t.Parallel()
	ada, google := caveHostLearner(t, salesSheet(t))
	res, body := ada.linkSheet(sheetLink, "jualan-ada")
	expectStatus(t, res, body, http.StatusSeeOther, "")

	bea := ada.second()
	bea.loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	res, body = bea.linkSheet(sheetLink, "jualan-bea")
	expectStatus(t, res, body, http.StatusForbidden, "CaveHost comes with the Pro Plan")
	if n := len(google.requests()); n != 1 {
		t.Fatalf("Google was asked %d times, want only Ada's link", n)
	}
	for _, path := range []string{"/cavehost/jualan-ada"} {
		if res, _ := bea.get(path); res.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without a CaveHost Plan = %d, want 403", path, res.StatusCode)
		}
	}
	// Another CaveHost Learner does not see Ada's Deployment either.
	admin := ada.second()
	admin.login("admin@example.com", "correct horse battery")
	if res, _ := admin.get("/cavehost/jualan-ada"); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET another Learner's Deployment = %d, want 404", res.StatusCode)
	}
}
