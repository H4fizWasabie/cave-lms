package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
)

const brevoEndpoint = "https://api.brevo.com/v3/smtp/email"

// Brevo sends email through Brevo's transactional API (https://developers.brevo.com/reference/send-transac-email).
type Brevo struct {
	APIKey string
	// From is an address or "Name <address>".
	From string
	// Endpoint defaults to Brevo's send-email URL.
	Endpoint string
}

var brevoClient = &http.Client{}

func (m Brevo) Send(ctx context.Context, message Message) error {
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("brevo sender %q: %w", m.From, err)
	}
	sender := map[string]string{"email": from.Address}
	if from.Name != "" {
		sender["name"] = from.Name
	}
	email := map[string]any{
		"sender":      sender,
		"to":          []map[string]string{{"email": message.To}},
		"subject":     message.Subject,
		"textContent": message.Body,
	}
	if len(message.Attachments) > 0 {
		// Content is []byte, so it goes as base64.
		files := make([]map[string]any, len(message.Attachments))
		for i, a := range message.Attachments {
			files[i] = map[string]any{"name": a.Filename, "content": a.Content}
		}
		email["attachment"] = files
	}
	body, err := json.Marshal(email)
	if err != nil {
		return err
	}
	endpoint := m.Endpoint
	if endpoint == "" {
		endpoint = brevoEndpoint
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
	req.Header.Set("api-key", m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := brevoClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("brevo refused the email: %s: %s", resp.Status, bytes.TrimSpace(reply))
	}
	return nil
}
