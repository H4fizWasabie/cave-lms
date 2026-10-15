package app_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
)

func TestHomePage(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated test database")
	}

	application, _ := newTestApplication(t, databaseURL)
	recorder := httptest.NewRecorder()
	application.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if !strings.Contains(recorder.Body.String(), "<h1>Cave LMS</h1>") || !strings.Contains(recorder.Body.String(), "https://cave.example.invalid") {
		t.Fatalf("home page did not render configured content: %s", recorder.Body.String())
	}
}

var _ http.Handler = (*app.App)(nil)
