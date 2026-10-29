package testutil

import (
	"context"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

type RecordingMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
	err  error
}

func (m *RecordingMailer) Send(ctx context.Context, message mailer.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, message)
	return nil
}

// FailWith makes every later Send return err without recording it; nil makes sends succeed again.
func (m *RecordingMailer) FailWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *RecordingMailer) Messages() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mailer.Message(nil), m.sent...)
}
