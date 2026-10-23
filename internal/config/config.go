package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strconv"
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
	// CaveTextExtensionIDs are the Chrome extension IDs the CaveText connect page may send a token to,
	// from CAVETEXT_EXTENSION_IDS (comma-separated). Empty: CaveText sign-in is refused.
	CaveTextExtensionIDs []string
	// BCLWebhookKey, from BCL_WEBHOOK_KEY, is the secret part of the address BCL posts payment webhooks to.
	// Empty: every webhook is refused.
	BCLWebhookKey string
	// BCLTestPayments, from BCL_PAYMENTS=test (staging only, ADR 0010), counts BCL Test Mode orders as paid and
	// ignores live ones. BCL_PAYMENTS unset or live: the reverse.
	BCLTestPayments bool
	// EmailAllowlist, from EMAIL_ALLOWLIST (comma-separated addresses, or "@domain" for a whole domain), is set on
	// staging only: email goes to these alone, and every other message is kept in the System log. Empty: email goes to all.
	EmailAllowlist []string
	// R2AccountID, R2Bucket, R2AccessKeyID and R2SecretAccessKey pick the R2 bucket Datasets live in (ADR 0008).
	// All or none: none set, Datasets stay on the disk under StorageDir.
	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	// OpenRouterAPIKey and OpenRouterModel let the AI write Exam questions and mark Written answers through OpenRouter
	// (ADR 0009). There is no built-in model: either unset, the Admin is told the AI is not configured and marks
	// Written answers by hand.
	OpenRouterAPIKey string
	OpenRouterModel  string
	// OpenRouterWebSearch, from OPENROUTER_WEB_SEARCH (true or false, true when unset), turns on OpenRouter's web search.
	OpenRouterWebSearch bool
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
		BCLWebhookKey:  strings.TrimSpace(os.Getenv("BCL_WEBHOOK_KEY")),
		MaxUploadBytes: 2 << 30,

		OpenRouterAPIKey:    strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterModel:     strings.TrimSpace(os.Getenv("OPENROUTER_MODEL")),
		OpenRouterWebSearch: true,
	}
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_WEB_SEARCH")); v != "" {
		on, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("OPENROUTER_WEB_SEARCH must be true or false")
		}
		cfg.OpenRouterWebSearch = on
	}
	var r2Missing []string
	for _, v := range []struct {
		name  string
		field *string
	}{{"R2_ACCOUNT_ID", &cfg.R2AccountID}, {"R2_BUCKET", &cfg.R2Bucket}, {"R2_ACCESS_KEY_ID", &cfg.R2AccessKeyID}, {"R2_SECRET_ACCESS_KEY", &cfg.R2SecretAccessKey}} {
		if *v.field = strings.TrimSpace(os.Getenv(v.name)); *v.field == "" {
			r2Missing = append(r2Missing, v.name)
		}
	}
	if len(r2Missing) > 0 && len(r2Missing) < 4 {
		return Config{}, fmt.Errorf("R2 config is incomplete: set %s too, or none of the R2_ variables", strings.Join(r2Missing, " and "))
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
	for _, id := range strings.Split(os.Getenv("CAVETEXT_EXTENSION_IDS"), ",") {
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		// The ID becomes a host name in the redirect, so only Chrome's own ID alphabet is accepted.
		if !extensionID.MatchString(id) {
			return Config{}, fmt.Errorf("CAVETEXT_EXTENSION_IDS: %q is not a Chrome extension ID", id)
		}
		cfg.CaveTextExtensionIDs = append(cfg.CaveTextExtensionIDs, id)
	}
	for _, e := range strings.Split(os.Getenv("EMAIL_ALLOWLIST"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.EmailAllowlist = append(cfg.EmailAllowlist, e)
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BCL_PAYMENTS"))) {
	case "", "live":
	case "test":
		cfg.BCLTestPayments = true
	default:
		return Config{}, fmt.Errorf("BCL_PAYMENTS must be live or test")
	}
	// The key is the webhook's only proof of origin, so a short one is refused.
	if k := cfg.BCLWebhookKey; k != "" && len(k) < 32 {
		return Config{}, fmt.Errorf("BCL_WEBHOOK_KEY must be at least 32 characters")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	return cfg, nil
}

var extensionID = regexp.MustCompile(`^[a-p]{32}$`)
