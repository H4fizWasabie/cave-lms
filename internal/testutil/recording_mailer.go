package testutil

import (
	"context"
	"sync"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

type RecordingMailer struct {
	mu            sync.Mutex
	sent          []mailer.Message
	err           error
	attachmentErr error // returned instead of err for a message with attachments, when set
}

// FailAttachmentsWith makes every later Send of a message with attachments return err without recording it, as a
// large upload timing out would; nil makes them succeed again.
func (m *RecordingMailer) FailAttachmentsWith(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attachmentErr = err
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
	if m.attachmentErr != nil && len(message.Attachments) > 0 {
		return m.attachmentErr
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
