package mailer

import (
	"context"
	"errors"
)

var ErrNotConfigured = errors.New("email sending is not configured")

type Message struct {
	To      string
	Subject string
	Body    string
}

type Mailer interface {
	Send(context.Context, Message) error
}

type Unconfigured struct{}

func (Unconfigured) Send(context.Context, Message) error {
	return ErrNotConfigured
}
