package app

import (
	"context"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

// allowlistMailer, used when EMAIL_ALLOWLIST is set (staging, ADR 0010), sends only to the allowed addresses and
// domains and keeps every other message, link included, in the System log for the Owner.
type allowlistMailer struct {
	next    mailer.Mailer
	allowed []string                         // addresses, or "@domain"
	hold    func(format string, args ...any) // writes a held message to the System log
}

func (m allowlistMailer) Send(ctx context.Context, message mailer.Message) error {
	to := strings.ToLower(message.To)
	for _, a := range m.allowed {
		if a = strings.ToLower(a); to == a || strings.HasPrefix(a, "@") && strings.HasSuffix(to, a) {
			return m.next.Send(ctx, message)
		}
	}
	m.hold("email to %s not sent, not on EMAIL_ALLOWLIST: %s\n\n%s", message.To, message.Subject, message.Body)
	return nil
}
