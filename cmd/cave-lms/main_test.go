package main

import (
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

func TestNewMailerPrefersBrevoThenResendThenUnconfigured(t *testing.T) {
	both := config.Config{BrevoAPIKey: "b", ResendAPIKey: "r", EmailFrom: "noreply@example.test"}
	if m, name := newMailer(both); name != "Brevo" || m != (mailer.Brevo{APIKey: "b", From: "noreply@example.test"}) {
		t.Errorf("with both keys: %#v, %q; want Brevo", m, name)
	}
	resend := config.Config{ResendAPIKey: "r", EmailFrom: "noreply@example.test"}
	if m, name := newMailer(resend); name != "Resend" || m != (mailer.Resend{APIKey: "r", From: "noreply@example.test"}) {
		t.Errorf("with the Resend key: %#v, %q; want Resend", m, name)
	}
	if m, name := newMailer(config.Config{}); name != "" || m != (mailer.Unconfigured{}) {
		t.Errorf("with no key: %#v, %q; want Unconfigured", m, name)
	}
}
