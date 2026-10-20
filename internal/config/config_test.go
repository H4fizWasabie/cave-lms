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
	t.Setenv("STORAGE_DIR", "/var/lib/cave-lms/uploads")
	t.Setenv("RESEND_API_KEY", "")
}

func TestLoadReadsConfiguration(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/cave_lms" || cfg.PublicBaseURL != "https://courses.example.test" || cfg.EmailFrom != "Cave LMS <courses@example.test>" || cfg.ListenAddr != ":8080" || cfg.StorageDir != "/var/lib/cave-lms/uploads" {
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

func TestLoadRequiresStorageDir(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("STORAGE_DIR", " ")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "STORAGE_DIR") {
		t.Fatalf("Load() error = %v, want STORAGE_DIR error", err)
	}
}

func TestLoadSetsUploadLimitToAboutTwoGigabytes(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUploadBytes != 2<<30 {
		t.Fatalf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, int64(2<<30))
	}
}

func TestLoadReadsOptionalResendKey(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil || cfg.ResendAPIKey != "" {
		t.Fatalf("without a key: cfg.ResendAPIKey = %q, err = %v; want empty, nil", cfg.ResendAPIKey, err)
	}

	t.Setenv("RESEND_API_KEY", " re_test_key ")
	cfg, err = Load()
	if err != nil || cfg.ResendAPIKey != "re_test_key" {
		t.Fatalf("with a key: cfg.ResendAPIKey = %q, err = %v; want re_test_key, nil", cfg.ResendAPIKey, err)
	}
}
