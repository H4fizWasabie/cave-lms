package migrations_test

import (
	"context"
	"testing"

	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/H4fizWasabie/cave-lms/internal/testutil"
)

// Migration 00160 gives each Learner who paid for a Pro Plan, still has its year of CaveText Pro and has a Lab Plan
// period running a CaveHost Plan period ending when that Lab Plan period ends. Learners who bought CaveText Pro and the
// Lab Plan apart get none, and running it again gives nothing more.
func TestLearnersHoldingAPaidProPlanAtLaunchGetCaveHostUntilTheirLabPlanPeriodEnds(t *testing.T) {
	ctx := context.Background()
	db, _ := testutil.NewDatabase(t)
	if err := migrations.DownTo(ctx, db, 159); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var lab int64
	if err := db.QueryRow(`INSERT INTO plans (name, days, deployments, lab) VALUES ('Lab Month', 30, 5, true) RETURNING id`).Scan(&lab); err != nil {
		t.Fatal(err)
	}
	// learner makes a Learner with a CaveText account, a year of CaveText Pro, and a Lab Plan period running until labDays
	// from now; payment is the product of a granted Payment, or none.
	learner := func(name, payment string, labDays int) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(`INSERT INTO learners (name, username, email, password_hash) VALUES ($1, $1, $1 || '@example.invalid', 'x') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO cavetext_accounts (learner_id) VALUES ($1)`, id)
		exec(`INSERT INTO cavetext_pro_periods (learner_id, starts_at, ends_at) VALUES ($1, now() - interval '5 days', now() + interval '360 days')`, id)
		exec(`INSERT INTO plan_periods (learner_id, plan_id, starts_at, ends_at, deployments, storage_mb)
			VALUES ($1, $2, now() - interval '5 days', now() + make_interval(days => $3), 5, 0)`, id, lab, labDays)
		if payment != "" {
			exec(`INSERT INTO cavetext_payments (order_number, payer_name, payer_email, cavelms_email, subtotal_sen, learner_id, granted, product)
				VALUES ($1, $2, $2, $2, 24900, $3, true, $4)`, name+"-order", name, id, payment)
		}
		return id
	}
	ada := learner("ada", "pro-plan", 20)
	bob := learner("bob", "cavetext-pro", 20) // bought apart
	cy := learner("cy", "pro-plan", 20)
	exec(`INSERT INTO cavehost_plan_periods (learner_id, starts_at, ends_at) VALUES ($1, now() - interval '1 day', now() + interval '3 days')`, cy)
	dee := learner("dee", "pro-plan", 20)
	exec(`UPDATE cavetext_pro_periods SET ends_at = now() - interval '1 day' WHERE learner_id = $1`, dee) // its year is over

	// held lists the Learner's CaveHost Plan periods as seconds from now at which each starts and ends, rounded to days.
	held := func(id int64) (got [][2]int) {
		t.Helper()
		rows, err := db.Query(`SELECT round(extract(epoch FROM starts_at - now()) / 86400)::int, round(extract(epoch FROM ends_at - now()) / 86400)::int
			FROM cavehost_plan_periods WHERE learner_id = $1 ORDER BY starts_at`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var p [2]int
			if err := rows.Scan(&p[0], &p[1]); err != nil {
				t.Fatal(err)
			}
			got = append(got, p)
		}
		return got
	}
	check := func(when string) {
		t.Helper()
		for _, c := range []struct {
			name string
			id   int64
			want [][2]int
		}{
			{"Pro Plan paid", ada, [][2]int{{0, 20}}},
			{"bought apart", bob, nil},
			{"already had CaveHost", cy, [][2]int{{-1, 3}, {3, 20}}},
			{"Pro year over", dee, nil},
		} {
			got := held(c.id)
			if len(got) != len(c.want) {
				t.Fatalf("%s: %s: periods %v, want %v", when, c.name, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("%s: %s: periods %v, want %v", when, c.name, got, c.want)
				}
			}
		}
	}

	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	check("after the migration")

	// Run it again: its Down changes nothing, so going down and up is the migration run twice.
	if err := migrations.DownTo(ctx, db, 159); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	check("after running it again")
}
