// Package migrations holds the database migrations and applies them, for the App and for the tests' schemas.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// goose keeps its filesystem and dialect in globals, so one migration runs at a time: tests migrate in parallel.
var mu sync.Mutex

// Up applies every migration not yet applied.
func Up(ctx context.Context, db *sql.DB) error {
	mu.Lock()
	defer mu.Unlock()
	if err := use(); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}

// DownTo undoes every migration after version, as if the database were from before them, for tests.
func DownTo(ctx context.Context, db *sql.DB, version int64) error {
	mu.Lock()
	defer mu.Unlock()
	if err := use(); err != nil {
		return err
	}
	return goose.DownToContext(ctx, db, ".", version)
}

func use() error {
	goose.SetBaseFS(files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	return nil
}
