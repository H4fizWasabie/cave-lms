package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/cave_lms")
	t.Setenv("PUBLIC_BASE_URL", "https://courses.example.test")
	t.Setenv("EMAIL_FROM", "Cave LMS <courses@example.test>")
	t.Setenv("LISTEN_ADDR", "")
}

func TestLoadReadsConfiguration(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/cave_lms" || cfg.PublicBaseURL != "https://courses.example.test" || cfg.EmailFrom != "Cave LMS <courses@example.test>" || cfg.ListenAddr != ":8080" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRequiresEmailSender(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("EMAIL_FROM", " ")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "EMAIL_FROM") {
		t.Fatalf("Load() error = %v, want EMAIL_FROM error", err)
	}
}

func TestLoadRequiresAbsolutePublicURL(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("PUBLIC_BASE_URL", "/courses")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PUBLIC_BASE_URL") {
		t.Fatalf("Load() error = %v, want PUBLIC_BASE_URL error", err)
	}
}
