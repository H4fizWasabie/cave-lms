package mailer

import (
	"context"
	"errors"
)

var ErrNotConfigured = errors.New("email sending is not configured")

type Message struct {
	To          string
	Subject     string
	Body        string
	Attachments []Attachment
}

// Attachment is a file sent with a Message. Its JSON is Resend's: Content goes as base64.
type Attachment struct {
	Filename string `json:"filename"`
	Content  []byte `json:"content"`
}

type Mailer interface {
	Send(context.Context, Message) error
}

type Unconfigured struct{}

func (Unconfigured) Send(context.Context, Message) error {
	return ErrNotConfigured
}
