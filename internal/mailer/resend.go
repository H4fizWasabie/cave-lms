package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const resendEndpoint = "https://api.resend.com/emails"

// Resend sends email through Resend's HTTP API (https://resend.com/docs/api-reference/emails/send-email).
type Resend struct {
	APIKey string
	From   string
	// Endpoint defaults to Resend's send-email URL.
	Endpoint string
}

// sendTimeout is how long one email may take; one with attachments gets attachmentSendTimeout, since uploading a
// 19 MB Ebook email from the VPS took over 15 seconds.
const (
	sendTimeout           = 15 * time.Second
	attachmentSendTimeout = 3 * time.Minute
)

var resendClient = &http.Client{}

func (m Resend) Send(ctx context.Context, message Message) error {
	email := map[string]any{
		"from":    m.From,
		"to":      []string{message.To},
		"subject": message.Subject,
		"text":    message.Body,
	}
	if len(message.Attachments) > 0 {
		email["attachments"] = message.Attachments
	}
	body, err := json.Marshal(email)
	if err != nil {
		return err
	}
	endpoint := m.Endpoint
	if endpoint == "" {
		endpoint = resendEndpoint
	}
	timeout := sendTimeout
	if len(message.Attachments) > 0 {
		timeout = attachmentSendTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := resendClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("resend refused the email: %s: %s", resp.Status, bytes.TrimSpace(reply))
	}
	return nil
}
