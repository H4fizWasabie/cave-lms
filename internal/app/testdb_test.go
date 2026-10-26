package app_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// newTestApplication returns the app, its recording Mailer, a handle on its isolated schema
// and the temp directory that holds its uploaded files. opts may change the config first.
func newTestApplication(t *testing.T, opts ...func(*config.Config)) (*app.App, *testutil.RecordingMailer, *sql.DB, string) {
	t.Helper()
	return newTestApplicationWith(t, &testutil.FakeWriter{}, opts...)
}

// newTestApplicationWith is newTestApplication with writer as the AI writer.
func newTestApplicationWith(t *testing.T, writer aiwriter.Writer, opts ...func(*config.Config)) (*app.App, *testutil.RecordingMailer, *sql.DB, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated test database")
	}

	pgConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	baseDB := stdlib.OpenDB(*pgConfig)
	t.Cleanup(func() {
		if err := baseDB.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := baseDB.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	var schemaBytes [16]byte
	if _, err := rand.Read(schemaBytes[:]); err != nil {
		t.Fatal(err)
	}
	schema := "cave_lms_test_" + hex.EncodeToString(schemaBytes[:])
	if _, err := baseDB.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}

	// Extensions belong to the database, not a test's schema: keep pg_trgm in public, which outlives every schema.
	if _, err := baseDB.ExecContext(context.Background(), "CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	pgConfig.RuntimeParams["search_path"] = schema + ",public"
	scopedURL := stdlib.RegisterConnConfig(pgConfig)
	var application *app.App
	scopedDB, err := sql.Open("pgx", scopedURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := scopedDB.Close(); err != nil {
			t.Errorf("close scoped database: %v", err)
		}
		if application != nil {
			if err := application.Close(); err != nil {
				t.Errorf("close app: %v", err)
			}
		}
		if _, err := baseDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

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
	application, err = app.New(context.Background(), cfg, recorder, writer)
	if err != nil {
		t.Fatal(err)
	}
	return application, recorder, scopedDB, storageDir
}

var _ mailer.Mailer = (*testutil.RecordingMailer)(nil)
