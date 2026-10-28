package migrations_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Migration 00135 fills each pending Part's upload kind and replaced Part from its preview JSON; a confirmed Part keeps
// the default.
func TestUploadKindIsBackfilledFromThePreviewJSONOfEveryPendingPart(t *testing.T) {
	ctx := context.Background()
	db, _ := testutil.NewDatabase(t)
	if err := migrations.DownTo(ctx, db, 134); err != nil {
		t.Fatal(err)
	}
	var learner int64
	if err := db.QueryRow(`INSERT INTO learners (name, username, email, password_hash) VALUES ('Ada', 'ada', 'ada@example.invalid', 'x') RETURNING id`).Scan(&learner); err != nil {
		t.Fatal(err)
	}
	// part makes a Dashboard of its own with one Part in state and returns the Part's ID.
	part := func(name, state, cleanup string) int64 {
		t.Helper()
		var dashboard, id int64
		if err := db.QueryRow(`INSERT INTO dashboards (learner_id, name) VALUES ($1, $2) RETURNING id`, learner, name).Scan(&dashboard); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`INSERT INTO dataset_parts (dashboard_id, storage_key, file_name, saved_size, row_count, state, position, part_date, cleanup)
			VALUES ($1, $2, $2, 1, 1, $3, CASE WHEN $3 = 'current' THEN 1 END, '2026-10-01', $4) RETURNING id`, dashboard, name, state, cleanup).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	replaced := part("replaced", "current", `{}`)
	cases := []struct {
		id       int64
		kind     string
		replaces any
	}{
		{part("data", "pending", `{}`), "replace-data", nil},
		{part("add", "pending", `{"added": {"rows": 2, "had": 3}}`), "add", nil},
		{part("replace", "pending", fmt.Sprintf(`{"replaces": {"part": %d, "file": "a.xlsx", "rows": 1, "had": 1}}`, replaced)), "replace-part", replaced},
		{part("confirmed", "current", `{"added": {"rows": 2, "had": 3}}`), "replace-data", nil},
	}

	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		var kind string
		var replaces any
		if err := db.QueryRow(`SELECT upload_kind, replaces_part_id FROM dataset_parts WHERE id = $1`, c.id).Scan(&kind, &replaces); err != nil {
			t.Fatal(err)
		}
		if kind != c.kind || replaces != c.replaces {
			t.Errorf("Part %d: upload kind %q replacing %v, want %q replacing %v", c.id, kind, replaces, c.kind, c.replaces)
		}
	}
}
