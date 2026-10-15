package testutil

import (
	"context"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

type RecordingMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *RecordingMailer) Send(ctx context.Context, message mailer.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, message)
	return nil
}

func (m *RecordingMailer) Messages() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mailer.Message(nil), m.sent...)
}
