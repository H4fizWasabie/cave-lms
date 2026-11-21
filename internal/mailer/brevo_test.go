package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type brevoBody struct {
	Sender struct {
		Name  *string `json:"name"`
		Email string  `json:"email"`
	} `json:"sender"`
	To          []map[string]string `json:"to"`
	Subject     string              `json:"subject"`
	TextContent string              `json:"textContent"`
	Attachment  []map[string]string `json:"attachment"`
}

func TestBrevoSendsTheMessageThroughBrevosAPI(t *testing.T) {
	var got struct {
		method, path, key, contentType, accept string
		body                                   brevoBody
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.key, got.contentType, got.accept = r.Header.Get("api-key"), r.Header.Get("Content-Type"), r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<201798300811.5787683@relay.domain.com>"}`))
	}))
	defer server.Close()

	m := Brevo{APIKey: "xkeysib-test", From: "Cave LMS <noreply@courses.example.test>", Endpoint: server.URL + "/v3/smtp/email"}
	err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "Verify your email", Body: "Open this link."})
	if err != nil {
		t.Fatal(err)
	}

	if got.method != http.MethodPost || got.path != "/v3/smtp/email" {
		t.Errorf("request = %s %s, want POST /v3/smtp/email", got.method, got.path)
	}
	if got.key != "xkeysib-test" || got.contentType != "application/json" || got.accept != "application/json" {
		t.Errorf("headers api-key = %q, Content-Type = %q, Accept = %q", got.key, got.contentType, got.accept)
	}
	b := got.body
	if b.Sender.Name == nil || *b.Sender.Name != "Cave LMS" || b.Sender.Email != "noreply@courses.example.test" {
		t.Errorf("sender = %+v, want Cave LMS <noreply@courses.example.test>", b.Sender)
	}
	if len(b.To) != 1 || b.To[0]["email"] != "learner@example.test" || b.Subject != "Verify your email" || b.TextContent != "Open this link." {
		t.Errorf("body = %+v", b)
	}
	if len(b.Attachment) != 0 {
		t.Errorf("attachment = %v, want none", b.Attachment)
	}
}

func TestBrevoSendsABareFromAddressWithoutAName(t *testing.T) {
	var raw struct{ Sender map[string]any }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	m := Brevo{APIKey: "xkeysib-test", From: "noreply@courses.example.test", Endpoint: server.URL}
	if err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "s", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, hasName := raw.Sender["name"]; hasName || raw.Sender["email"] != "noreply@courses.example.test" {
		t.Errorf("sender = %v, want only the email", raw.Sender)
	}
}

func TestBrevoReportsWhyBrevoRefusedTheMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized","message":"Key not found"}`))
	}))
	defer server.Close()

	m := Brevo{APIKey: "xkeysib-test", From: "noreply@courses.example.test", Endpoint: server.URL}
	err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Key not found") {
		t.Fatalf("Send() error = %v, want the status and Brevo's message", err)
	}
}

func TestBrevoRefusesAFromThatIsNotAnAddress(t *testing.T) {
	m := Brevo{APIKey: "xkeysib-test", From: "not an address", Endpoint: "http://127.0.0.1:1"}
	if err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "s", Body: "b"}); err == nil {
		t.Fatal("Send() with a bad From = nil, want an error")
	}
}

func TestBrevoSendsAttachmentsBase64Encoded(t *testing.T) {
	var body brevoBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	m := Brevo{APIKey: "xkeysib-test", From: "noreply@courses.example.test", Endpoint: server.URL}
	err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "Your Result", Body: "You passed.",
		Attachments: []Attachment{{Filename: "answer-sheet.pdf", Content: []byte("%PDF-1.3")}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Attachment) != 1 || body.Attachment[0]["name"] != "answer-sheet.pdf" || body.Attachment[0]["content"] != "JVBERi0xLjM=" {
		t.Fatalf("attachment = %v, want answer-sheet.pdf with its content in base64", body.Attachment)
	}
}

func TestBrevoGivesAMessageWithAttachmentsLongerToSend(t *testing.T) {
	sendTimeout, attachmentSendTimeout = 50*time.Millisecond, 5*time.Second
	t.Cleanup(func() { sendTimeout, attachmentSendTimeout = 15*time.Second, 3*time.Minute })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	m := Brevo{APIKey: "xkeysib-test", From: "Cave LMS <noreply@courses.example.test>", Endpoint: server.URL}

	if err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "s", Body: "b"}); err == nil {
		t.Error("a plain message slower than sendTimeout was sent, want it to time out")
	}
	withFile := Message{To: "learner@example.test", Subject: "s", Body: "b", Attachments: []Attachment{{Filename: "a.pdf", Content: []byte("%PDF")}}}
	if err := m.Send(context.Background(), withFile); err != nil {
		t.Errorf("a message with attachments slower than sendTimeout: %v, want it sent", err)
	}
}
