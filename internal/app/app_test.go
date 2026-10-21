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
	for _, want := range []string{"Selamat Datang ke CAVELMS", "LEARN → APPLY → BUILD → SUSTAIN", "Coach Adam", "Powered by The Cave Consult (TCC)", "Past programmes", "Certified Professional in Generative AI (CPGAI)"} {
		if !strings.Contains(body, want) {
			t.Errorf("home page is missing %q", want)
		}
	}
	for text, href := range map[string]string{"Explore eBooks": `/gallery\?format=ebook`, "Join Online Classes": "/courses", "Learn AI Dashboard Development": "/courses"} {
		if !regexp.MustCompile(`href="` + href + `"[^>]*>` + text + `</a>`).MatchString(body) {
			t.Errorf("home page has no %q link to %s", text, href)
		}
	}
}

func TestHeaderShowsTheCAVELMSLogo(t *testing.T) {
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	m := regexp.MustCompile(`<a class="brand" href="/"><img src="(/static/[^"]+)" alt="CAVELMS, your digital learning hub"`).FindStringSubmatch(recorder.Body.String())
	if m == nil {
		t.Fatalf("header has no CAVELMS logo linking home: %s", recorder.Body.String())
	}
	logo := httptest.NewRecorder()
	application.ServeHTTP(logo, httptest.NewRequest(http.MethodGet, m[1], nil))
	if logo.Code != http.StatusOK || !strings.Contains(logo.Body.String(), "<svg") {
		t.Fatalf("GET %s = %d, want the logo SVG", m[1], logo.Code)
	}
}

var _ http.Handler = (*app.App)(nil)

func TestStaticFilesAreServedWithCachingAndMissingOnesAre404(t *testing.T) {
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
	application, _, _, _ := newTestApplication(t)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/gallery?format=ebook", nil))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "eBooks are coming soon.") {
		t.Fatalf("GET /gallery?format=ebook = %d, want 200 saying eBooks are coming soon; body:\n%s", recorder.Code, recorder.Body.String())
	}
}

func TestPagesLinkTheStylesheetByAFingerprintedURL(t *testing.T) {
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
