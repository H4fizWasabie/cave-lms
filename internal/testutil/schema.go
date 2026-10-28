package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// NewSchema gives the test its own migrated schema on TEST_DATABASE_URL, dropped when the test ends: a database handle
// scoped to it and the URL that opens another. It skips the test when TEST_DATABASE_URL is unset.
func NewSchema(t *testing.T) (db *sql.DB, url string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated test database")
	}
	ctx := context.Background()

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
	if err := baseDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	var schemaBytes [16]byte
	if _, err := rand.Read(schemaBytes[:]); err != nil {
		t.Fatal(err)
	}
	schema := "cave_lms_test_" + hex.EncodeToString(schemaBytes[:])
	if _, err := baseDB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := baseDB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	// Extensions belong to the database, not a test's schema: keep pg_trgm in public, which outlives every schema.
	if _, err := baseDB.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	pgConfig.RuntimeParams["search_path"] = schema + ",public"
	url = stdlib.RegisterConnConfig(pgConfig)
	db, err = sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close scoped database: %v", err)
		}
	})
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, url
}
