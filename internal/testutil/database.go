package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// template is the migrated database every test's database is copied from. A test binary makes it once, under a name no
// other binary shares, as the packages' binaries run at the same time against one server.
var template struct {
	once sync.Once
	name string
	err  error
}

// NewDatabase gives the test its own migrated database on the TEST_DATABASE_URL server, dropped when the test ends: a
// handle on it and the URL that opens another. Migrating once per test binary and copying makes it cheap enough for
// tests to run in parallel. It skips the test when TEST_DATABASE_URL is unset.
func NewDatabase(t *testing.T) (db *sql.DB, url string) {
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
	adminDB := stdlib.OpenDB(*pgConfig)
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Errorf("close test server handle: %v", err)
		}
	})
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	template.once.Do(func() { template.name, template.err = migratedTemplate(ctx, adminDB, pgConfig) })
	if template.err != nil {
		t.Fatal(template.err)
	}

	name := randomName("cave_lms_test_")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template.name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", name)); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})

	scoped := pgConfig.Copy()
	scoped.Database = name
	url = stdlib.RegisterConnConfig(scoped)
	db, err = sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close scoped database: %v", err)
		}
		stdlib.UnregisterConnConfig(url)
	})
	return db, url
}

// Main runs a package's tests and then drops the template NewDatabase made, so a test database that outlives the run
// is not left holding a copy. Call it from TestMain: os.Exit(testutil.Main(m)).
func Main(m *testing.M) int {
	code := m.Run()
	if template.name == "" {
		return code
	}
	pgConfig, err := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err == nil {
		adminDB := stdlib.OpenDB(*pgConfig)
		_, err = adminDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", template.name))
		_ = adminDB.Close()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "drop test template: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// migratedTemplate makes a database with pg_trgm and every migration applied and nothing connected to it, which
// CREATE DATABASE ... TEMPLATE requires.
// A template that fails to migrate is dropped, so a run on a database that outlives it leaves none behind.
func migratedTemplate(ctx context.Context, adminDB *sql.DB, pgConfig *pgx.ConnConfig) (string, error) {
	name := randomName("cave_lms_template_")
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return "", err
	}
	templateConfig := pgConfig.Copy()
	templateConfig.Database = name
	templateDB := stdlib.OpenDB(*templateConfig)
	_, err := templateDB.ExecContext(ctx, "CREATE EXTENSION pg_trgm SCHEMA public")
	if err == nil {
		err = migrations.Up(ctx, templateDB)
	}
	err = errors.Join(err, templateDB.Close())
	if err != nil {
		_, dropErr := adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name))
		return "", errors.Join(err, dropErr)
	}
	return name, nil
}

func randomName(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
