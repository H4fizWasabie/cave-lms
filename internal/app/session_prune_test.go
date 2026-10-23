package app_test

import (
	"context"
	"testing"
)

// The daily prune deletes sessions past their expiry and keeps live ones.
func TestTheDailyPruneDeletesExpiredSessions(t *testing.T) {
	s := newSite(t)
	s.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	s.second().loginAsNewLearner("Bea", "bea@example.com", "correct horse battery")
	if _, err := s.db.Exec(`UPDATE sessions SET expires_at = now() - interval '1 minute'
		WHERE learner_id = (SELECT id FROM learners WHERE email = 'ada@example.com')`); err != nil {
		t.Fatal(err)
	}

	s.application.RunDailyPrune(context.Background())

	var expired, live int
	if err := s.db.QueryRow(`SELECT count(*) FILTER (WHERE expires_at <= now()), count(*) FILTER (WHERE expires_at > now()) FROM sessions`).Scan(&expired, &live); err != nil {
		t.Fatal(err)
	}
	if expired != 0 || live != 1 {
		t.Fatalf("after the prune: %d expired and %d live sessions, want 0 and 1", expired, live)
	}
}
