package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/config"
)

const bclKey = "k3y-from-the-env"

// bclSite is a site that takes BCL webhooks at /cavetext/bcl/<bclKey>, logged in as the Owner.
func bclSite(t *testing.T) *site {
	t.Helper()
	s := newSite(t, func(c *config.Config) {
		c.OwnerEmail = ownerEmail
		c.BCLWebhookKey = bclKey
	})
	s.loginAsNewAdmin("Abah", ownerEmail, "correct horse battery")
	return s
}

// bclPost sends body to the webhook address with key, as BCL's server would: no cookie, no browser headers.
func (s *site) bclPost(key, body string) (*http.Response, string) {
	s.t.Helper()
	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/cavetext/bcl/"+key, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.server.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	res.Body.Close()
	return res, ""
}

func TestABCLWebhookWithTheKeyIsKeptInTheSystemLog(t *testing.T) {
	s := bclSite(t)
	res, _ := s.bclPost(bclKey, `{"order_number":"BCL-123","status":"success"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("webhook with the key: status %d, want 200", res.StatusCode)
	}
	_, body := s.get("/admin/log")
	for _, want := range []string{"BCL webhook", "BCL-123"} {
		if !strings.Contains(body, want) {
			t.Errorf("System log lacks %q:\n%s", want, body)
		}
	}
}

func TestABCLWebhookWithAWrongKeyIsRefusedAndNotKept(t *testing.T) {
	s := bclSite(t)
	for _, key := range []string{"wrong", ""} {
		res, _ := s.bclPost(key, `{"order_number":"FAKE-9"}`)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("webhook with key %q: status %d, want 404", key, res.StatusCode)
		}
	}
	_, body := s.get("/admin/log")
	if strings.Contains(body, "FAKE-9") {
		t.Errorf("a webhook with a wrong key reached the System log:\n%s", body)
	}
}

func TestBCLWebhooksAreRefusedWhenNoKeyIsSet(t *testing.T) {
	s := ownerSite(t)
	res, _ := s.bclPost("", `{"order_number":"FAKE-1"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("webhook with no key set: status %d, want 404", res.StatusCode)
	}
}
