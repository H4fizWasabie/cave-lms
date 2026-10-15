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
	ListenAddr    string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PublicBaseURL: strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		EmailFrom:     strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		ListenAddr:    strings.TrimSpace(os.Getenv("LISTEN_ADDR")),
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
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	return cfg, nil
}
