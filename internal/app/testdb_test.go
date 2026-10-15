package app_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func newTestApplication(t *testing.T, databaseURL string) (*app.App, *testutil.RecordingMailer) {
	t.Helper()

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

	pgConfig.RuntimeParams["search_path"] = schema
	scopedURL := stdlib.RegisterConnConfig(pgConfig)
	var application *app.App
	t.Cleanup(func() {
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
	application, err = app.New(context.Background(), config.Config{
		DatabaseURL:   scopedURL,
		PublicBaseURL: "https://cave.example.invalid",
		EmailFrom:     "Cave LMS <courses@example.invalid>",
		ListenAddr:    ":8080",
	}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	return application, recorder
}

var _ mailer.Mailer = (*testutil.RecordingMailer)(nil)
