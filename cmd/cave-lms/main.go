package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	application, err := app.New(context.Background(), cfg, mailer.Unconfigured{})
	if err != nil {
		return err
	}
	defer application.Close()

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           application,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Cave LMS listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
