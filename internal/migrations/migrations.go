// Package migrations holds the database migrations and applies them, for the App and for the tests' schemas.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// UpTo applies the migrations up to and including version.
func UpTo(ctx context.Context, db *sql.DB, version int64) error {
	if err := use(); err != nil {
		return err
	}
	if err := goose.UpToContext(ctx, db, ".", version); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}

// Up applies every migration not yet applied. The App applies them in two steps, UpTo then Up, as Datasets move into the
// store between 00068, which adds their keys, and 00069, which drops the old column.
func Up(ctx context.Context, db *sql.DB) error {
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
