package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL   string
	PublicBaseURL string
	EmailFrom     string
	// ResendAPIKey is optional; when set, email is sent through Resend.
	ResendAPIKey string
	ListenAddr   string
	// StorageDir holds uploaded Gallery files. It must never be a public static path.
	StorageDir string
	// OwnerEmail names the Owner, the one Admin who sees the System log. Optional: unset, nobody does.
	OwnerEmail string
	// MaxUploadBytes caps one upload. Not read from the environment: about 2 GB.
	MaxUploadBytes int64
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PublicBaseURL:  strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		EmailFrom:      strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		ResendAPIKey:   strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		ListenAddr:     strings.TrimSpace(os.Getenv("LISTEN_ADDR")),
		StorageDir:     strings.TrimSpace(os.Getenv("STORAGE_DIR")),
		OwnerEmail:     strings.TrimSpace(os.Getenv("OWNER_EMAIL")),
		MaxUploadBytes: 2 << 30,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	publicURL, err := url.Parse(cfg.PublicBaseURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return Config{}, fmt.Errorf("PUBLIC_BASE_URL must be an absolute http or https URL")
	}
	if cfg.EmailFrom == "" {
		return Config{}, fmt.Errorf("EMAIL_FROM is required")
	}
	if _, err := mail.ParseAddress(cfg.EmailFrom); err != nil {
		return Config{}, fmt.Errorf("EMAIL_FROM must be a valid email address: %w", err)
	}
	if cfg.StorageDir == "" {
		return Config{}, fmt.Errorf("STORAGE_DIR is required")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	return cfg, nil
}
