package app_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// newTestApplication returns the app, its recording Mailer, a handle on its isolated database
// and the temp directory that holds its uploaded files. opts may change the config first.
func newTestApplication(t *testing.T, opts ...func(*config.Config)) (*app.App, *testutil.RecordingMailer, *sql.DB, string) {
	t.Helper()
	return newTestApplicationWith(t, &testutil.FakeWriter{}, opts...)
}

// newTestApplicationWith is newTestApplication with writer as the AI writer.
func newTestApplicationWith(t *testing.T, writer aiwriter.Writer, opts ...func(*config.Config)) (*app.App, *testutil.RecordingMailer, *sql.DB, string) {
	t.Helper()
	scopedDB, scopedURL := testutil.NewDatabase(t)

	recorder := &testutil.RecordingMailer{}
	storageDir := t.TempDir()
	cfg := config.Config{
		DatabaseURL:    scopedURL,
		PublicBaseURL:  "https://cave.example.invalid",
		EmailFrom:      "Cave LMS <courses@example.invalid>",
		ListenAddr:     ":8080",
		StorageDir:     storageDir,
		MaxUploadBytes: 2 << 30,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	application, err := app.New(context.Background(), cfg, recorder, writer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})
	return application, recorder, scopedDB, storageDir
}

var _ mailer.Mailer = (*testutil.RecordingMailer)(nil)
