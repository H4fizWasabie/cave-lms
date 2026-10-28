package app_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

func TestHomePage(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "https://cave.example.invalid") {
		t.Fatalf("home page did not render configured content: %s", body)
	}
	for _, want := range []string{"Selamat Datang ke CAVELMS", "LEARN → APPLY → BUILD → SUSTAIN", "Coach Adam", "Dikuasakan oleh The Cave Consult (TCC)", "Program terdahulu", "Certified Professional in Generative AI (CPGAI)"} {
		if !strings.Contains(body, want) {
			t.Errorf("home page is missing %q", want)
		}
	}
	for text, href := range map[string]string{"Terokai eBook": `/gallery\?format=ebook`, "Sertai Kelas Dalam Talian": "/courses", "Belajar Membina Dashboard AI": "/courses"} {
		if !regexp.MustCompile(`href="` + href + `"[^>]*>` + text + `</a>`).MatchString(body) {
			t.Errorf("home page has no %q link to %s", text, href)
		}
	}
}

func TestDatabasePoolIsCapped(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	if got := application.DBStats().MaxOpenConnections; got != 25 {
		t.Fatalf("MaxOpenConnections = %d, want 25", got)
	}
}

func TestEveryForeignKeyHasAnIndex(t *testing.T) {
	t.Parallel()
	_, _, db, _ := newTestApplication(t)
	// conkey is 1-based and indkey 0-based, so both pick the first column. A partial index doesn't count: it can't
	// serve a plain lookup by the key, such as a cascading delete.
	rows, err := db.Query(`SELECT c.conrelid::regclass::text, a.attname FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f' AND c.connamespace = current_schema()::regnamespace
		AND NOT EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.conrelid AND i.indkey[0] = c.conkey[1]
			AND i.indpred IS NULL)
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		t.Errorf("foreign key %s.%s has no index", table, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

var _ http.Handler = (*app.App)(nil)

func TestStaticFilesAreServedWithCachingAndMissingOnesAre404(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)

	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/static/cavelms.css", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /static/cavelms.css status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css", got)
	}
	if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") {
		t.Fatalf("Cache-Control = %q, want a max-age", got)
	}

	recorder = httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/static/missing.css", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /static/missing.css status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestGalleryShowsEbooksAreComingSoonWhileThereAreNone(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gallery?format=ebook", nil)
	req.Header.Set("Accept-Language", "en")
	application.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "eBooks are coming soon.") {
		t.Fatalf("GET /gallery?format=ebook = %d, want 200 saying eBooks are coming soon; body:\n%s", recorder.Code, recorder.Body.String())
	}
}

func TestPagesLinkTheStylesheetByAFingerprintedURL(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	href := regexp.MustCompile(`/static/cavelms\.css\?v=[0-9a-f]{8,}`).FindString(recorder.Body.String())
	if href == "" {
		t.Fatalf("home page does not link the stylesheet by a fingerprinted URL; body:\n%s", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, href, nil))
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("GET %s = %d %q, want 200 text/css", href, recorder.Code, recorder.Header().Get("Content-Type"))
	}
	if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable for a fingerprinted URL", got)
	}
}

// The policy keeps every page to Cave's own scripts, forms and framing, so an escaping slip can't run someone
// else's script or send a form off-site, and no other site can frame a page to trick a click.
func TestEveryPageCarriesTheContentSecurityPolicy(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	const want = "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"
	for _, path := range []string{"/", "/no-such-page"} {
		recorder := httptest.NewRecorder()
		application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := recorder.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("GET %s Content-Security-Policy = %q, want %q", path, got, want)
		}
		if body := recorder.Body.String(); strings.Contains(body, "<script>") {
			t.Errorf("GET %s has an inline script, which the policy blocks", path)
		}
	}
}

func TestTheHomePageShowsCoachAdamDanielsCardWithHisCredentialsAsText(t *testing.T) {
	t.Parallel()
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()

	expectAll(t, body, `src="/static/coach-adam-daniel.webp`, "Coach Adam Daniel", "Strategic Intelligence Architect", "Founder &amp; Chief Consultant, The Cave Consult",
		"Executive Master in Business Management with Honours (EMBM)", "Certified Generative AI (Associates With Distinction)", "HRDC Accredited Trainer",
		"Department of Skills Development (JPK) Certified Trainer", "Sustainability &amp; ESG Reporting Professional (SERP)")
	if strings.Contains(body, "Adam bin Yusuf") {
		t.Errorf("home page still names the old coach caption")
	}
}
