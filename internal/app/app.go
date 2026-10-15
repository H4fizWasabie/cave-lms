package app

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql templates/home.html
var appFiles embed.FS

type App struct {
	db      *sql.DB
	handler http.Handler
	mailer  mailer.Mailer
}

func New(ctx context.Context, cfg config.Config, mailer mailer.Mailer) (_ *App, err error) {
	if mailer == nil {
		return nil, fmt.Errorf("mailer is required")
	}
	handler, err := homeHandler(cfg.PublicBaseURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	goose.SetBaseFS(appFiles)
	if err = goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("set migration dialect: %w", err)
	}
	if err = goose.UpContext(ctx, db, "migrations"); err != nil {
		return nil, fmt.Errorf("apply database migrations: %w", err)
	}
	return &App{db: db, handler: handler, mailer: mailer}, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.handler.ServeHTTP(w, r)
}

func (a *App) Close() error {
	return a.db.Close()
}

func homeHandler(publicBaseURL string) (http.Handler, error) {
	page, err := template.ParseFS(appFiles, "templates/home.html")
	if err != nil {
		return nil, fmt.Errorf("parse home page: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.ExecuteTemplate(w, "home.html", struct{ PublicBaseURL string }{publicBaseURL}); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	})
	return mux, nil
}
