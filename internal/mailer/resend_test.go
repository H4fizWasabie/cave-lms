package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendSendsTheMessageThroughResendsAPI(t *testing.T) {
	var got struct {
		method, path, auth, contentType string
		body                            map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.auth, got.contentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`))
	}))
	defer server.Close()

	m := Resend{APIKey: "re_test_key", From: "Cave LMS <noreply@courses.example.test>", Endpoint: server.URL + "/emails"}
	err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "Verify your email", Body: "Open this link."})
	if err != nil {
		t.Fatal(err)
	}

	if got.method != http.MethodPost || got.path != "/emails" {
		t.Errorf("request = %s %s, want POST /emails", got.method, got.path)
	}
	if got.auth != "Bearer re_test_key" {
		t.Errorf("Authorization = %q, want Bearer re_test_key", got.auth)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	to, _ := got.body["to"].([]any)
	if got.body["from"] != "Cave LMS <noreply@courses.example.test>" || len(to) != 1 || to[0] != "learner@example.test" ||
		got.body["subject"] != "Verify your email" || got.body["text"] != "Open this link." {
		t.Errorf("body = %v", got.body)
	}
}

func TestResendReportsWhyResendRefusedTheMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"message":"The courses.example.test domain is not verified.","name":"validation_error"}`))
	}))
	defer server.Close()

	m := Resend{APIKey: "re_test_key", From: "noreply@courses.example.test", Endpoint: server.URL}
	err := m.Send(context.Background(), Message{To: "learner@example.test", Subject: "Verify your email", Body: "Open this link."})
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "domain is not verified") {
		t.Fatalf("Send() error = %v, want the status and Resend's message", err)
	}
}
