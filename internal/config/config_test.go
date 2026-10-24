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
	t.Setenv("CAVETEXT_EXTENSION_IDS", "")
	t.Setenv("BCL_WEBHOOK_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")
	t.Setenv("OPENROUTER_WEB_SEARCH", "")
	t.Setenv("BCL_PAYMENTS", "")
	t.Setenv("BCL_API_TOKEN", "")
	t.Setenv("BCL_API_URL", "")
	t.Setenv("EMAIL_ALLOWLIST", "")
	for _, name := range r2Env {
		t.Setenv(name, "")
	}
}

var r2Env = []string{"R2_ACCOUNT_ID", "R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"}

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

func TestLoadReadsCaveTextExtensionIDs(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("CAVETEXT_EXTENSION_IDS", " omcdclpagggfcjlhpkohpcgdmfbelohd, aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ,")

	cfg, err := Load()
	if err != nil || strings.Join(cfg.CaveTextExtensionIDs, " ") != "omcdclpagggfcjlhpkohpcgdmfbelohd aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("CaveTextExtensionIDs = %q, err = %v", cfg.CaveTextExtensionIDs, err)
	}

	t.Setenv("CAVETEXT_EXTENSION_IDS", "evil.example/x")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CAVETEXT_EXTENSION_IDS") {
		t.Fatalf("Load() error = %v, want CAVETEXT_EXTENSION_IDS error", err)
	}
}

func TestLoadReadsALongEnoughBCLWebhookKey(t *testing.T) {
	setValidEnvironment(t)
	key := strings.Repeat("k", 32)
	t.Setenv("BCL_WEBHOOK_KEY", " "+key+" ")
	cfg, err := Load()
	if err != nil || cfg.BCLWebhookKey != key {
		t.Fatalf("BCLWebhookKey = %q, err = %v; want %q, nil", cfg.BCLWebhookKey, err, key)
	}

	t.Setenv("BCL_WEBHOOK_KEY", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BCL_WEBHOOK_KEY") {
		t.Fatalf("Load() error = %v, want BCL_WEBHOOK_KEY error", err)
	}
}

func TestLoadReadsTheBCLAPITokenAndDefaultsItsURLToBCL(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.BCLAPIToken != "" || cfg.BCLAPIURL != "https://bcl.my/api" {
		t.Fatalf("unset: BCLAPIToken = %q, BCLAPIURL = %q, err = %v; want empty, https://bcl.my/api", cfg.BCLAPIToken, cfg.BCLAPIURL, err)
	}
	t.Setenv("BCL_API_TOKEN", " tok ")
	t.Setenv("BCL_API_URL", "http://127.0.0.1:9/api/")
	cfg, err = Load()
	if err != nil || cfg.BCLAPIToken != "tok" || cfg.BCLAPIURL != "http://127.0.0.1:9/api" {
		t.Fatalf("set: BCLAPIToken = %q, BCLAPIURL = %q, err = %v", cfg.BCLAPIToken, cfg.BCLAPIURL, err)
	}
}

func TestLoadReadsR2ConfigOnlyWhenAllOfItIsSet(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.R2Bucket != "" {
		t.Fatalf("without R2 config: cfg.R2Bucket = %q, err = %v; want empty, nil", cfg.R2Bucket, err)
	}

	t.Setenv("R2_ACCOUNT_ID", " acc ")
	t.Setenv("R2_BUCKET", "datasets")
	_, err = Load()
	if err == nil || !strings.Contains(err.Error(), "R2_ACCESS_KEY_ID") || !strings.Contains(err.Error(), "R2_SECRET_ACCESS_KEY") || strings.Contains(err.Error(), "R2_BUCKET") {
		t.Fatalf("with half the R2 config: err = %v, want one naming R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY only", err)
	}

	t.Setenv("R2_ACCESS_KEY_ID", "key")
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret")
	cfg, err = Load()
	if err != nil || cfg.R2AccountID != "acc" || cfg.R2Bucket != "datasets" || cfg.R2AccessKeyID != "key" || cfg.R2SecretAccessKey != "secret" {
		t.Fatalf("with all R2 config: cfg = %#v, err = %v", cfg, err)
	}
}

func TestLoadReadsOpenRouterConfigWithWebSearchOnByDefaultAndNoModel(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.OpenRouterAPIKey != "" || cfg.OpenRouterModel != "" || !cfg.OpenRouterWebSearch {
		t.Fatalf("without OpenRouter config: cfg = %#v, err = %v; want no key, no model, web search on", cfg, err)
	}

	t.Setenv("OPENROUTER_API_KEY", " sk-or-test ")
	t.Setenv("OPENROUTER_MODEL", " openai/gpt-4o-mini ")
	t.Setenv("OPENROUTER_WEB_SEARCH", "false")
	cfg, err = Load()
	if err != nil || cfg.OpenRouterAPIKey != "sk-or-test" || cfg.OpenRouterModel != "openai/gpt-4o-mini" || cfg.OpenRouterWebSearch {
		t.Fatalf("with OpenRouter config: cfg = %#v, err = %v", cfg, err)
	}

	t.Setenv("OPENROUTER_WEB_SEARCH", "maybe")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OPENROUTER_WEB_SEARCH") {
		t.Fatalf("Load() error = %v, want OPENROUTER_WEB_SEARCH error", err)
	}
}

func TestLoadReadsWhichBCLPaymentsCountAndRefusesAnythingElse(t *testing.T) {
	for value, want := range map[string]bool{"": false, "live": false, "test": true, " Test ": true} {
		setValidEnvironment(t)
		t.Setenv("BCL_PAYMENTS", value)
		cfg, err := Load()
		if err != nil || cfg.BCLTestPayments != want {
			t.Errorf("BCL_PAYMENTS=%q: BCLTestPayments = %v, err %v; want %v", value, cfg.BCLTestPayments, err, want)
		}
	}
	setValidEnvironment(t)
	t.Setenv("BCL_PAYMENTS", "tset")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BCL_PAYMENTS") {
		t.Fatalf("Load() error = %v, want a BCL_PAYMENTS error", err)
	}
}

func TestLoadReadsTheEmailAllowList(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("EMAIL_ALLOWLIST", " abah@example.com, ,@client.example ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.EmailAllowlist, "|"); got != "abah@example.com|@client.example" {
		t.Fatalf("EmailAllowlist = %q", got)
	}
}
