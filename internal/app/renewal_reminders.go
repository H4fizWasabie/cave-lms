package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// deletionWarningDays is how many days before the deletion of a lapsed Learner's Lab data (datasetKeepDays) they are warned.
const deletionWarningDays = 7

// dueRenewalsSQL lists the renewal reminders (ADR 0026) now due and not yet sent, one row for each Learner, item and
// stage. For each Learner's Plan and each CaveText Pro it looks at the last stacked period only, so a Learner who has
// renewed has a later end and is skipped. A Plan counts only if it is the Lab Plan. The stage is the latest one reached
// (a Learner whose end is 5 days away gets the 7-day reminder, not the 30-day one); an end more than 7 days ago gets
// none, so a first deploy does not write to long-lapsed Learners. The Lab data stage is 'deletion': the last Plan
// period of any Plan ended between $1 less $2 and $1 days ago, with Workbooks or Datasets to lose
// (the rule of pruneDatasets). $1 is datasetKeepDays and $2 deletionWarningDays. Admins have no Plan to renew.
const dueRenewalsSQL = `
WITH last AS (
	(SELECT DISTINCT ON (pp.learner_id) pp.learner_id, 'lab-plan' AS item, p.lab AS counted, LEAST(pp.ends_at, COALESCE(pp.ended_early_at, pp.ends_at)) AS ends_at, false AS ebooks
		FROM plan_periods pp JOIN plans p ON p.id = pp.plan_id
		ORDER BY pp.learner_id, LEAST(pp.ends_at, COALESCE(pp.ended_early_at, pp.ends_at)) DESC)
	UNION ALL
	(SELECT DISTINCT ON (cp.learner_id) cp.learner_id, 'cavetext-pro', true, LEAST(cp.ends_at, COALESCE(cp.ended_early_at, cp.ends_at)), cp.ebooks
		FROM cavetext_pro_periods cp
		ORDER BY cp.learner_id, LEAST(cp.ends_at, COALESCE(cp.ended_early_at, cp.ends_at)) DESC)
), due AS (
	SELECT learner_id, item, ends_at, ebooks,
		CASE WHEN ends_at <= now() THEN 'ended' WHEN ends_at <= now() + interval '7 days' THEN '7-days' ELSE '30-days' END AS stage
	FROM last WHERE counted AND ends_at > now() - interval '7 days' AND ends_at <= now() + interval '30 days'
	UNION ALL
	SELECT learner_id, 'lab-data', ends_at, false, 'deletion' FROM last
	WHERE item = 'lab-plan' AND ends_at <= now() - make_interval(days => $1::int - $2::int) AND ends_at > now() - make_interval(days => $1::int)
		AND (EXISTS (SELECT 1 FROM workbooks wb WHERE wb.learner_id = last.learner_id)
			OR EXISTS (SELECT 1 FROM dataset_parts ds JOIN dashboards d ON d.id = ds.dashboard_id WHERE d.learner_id = last.learner_id))
)
SELECT l.id, l.name, l.email, COALESCE(l.language, ''), d.stage, d.item, d.ends_at, d.ebooks
FROM due d JOIN learners l ON l.id = d.learner_id
WHERE NOT l.is_admin AND NOT EXISTS (SELECT 1 FROM renewal_reminders r
	WHERE r.learner_id = d.learner_id AND r.item = d.item AND r.ends_at = d.ends_at AND r.stage = d.stage)
ORDER BY l.id, d.stage, d.item`

// renewalReminder is one email to one Learner for one stage, listing every item reaching it.
type renewalReminder struct {
	learnerID   int64
	name, email string
	lang, stage string
	items       []renewalItem
}

type renewalItem struct {
	item   string
	endsAt time.Time
	ebooks bool
}

// sendRenewalReminders sends every renewal reminder now due and not yet sent (dueRenewalsSQL): one email for each
// Learner and stage. Each is recorded before it is sent, so a restart in between loses an email and never doubles one;
// a failed send is logged and not retried. Reminders go out one at a time, in the daily prune.
func (a *App) sendRenewalReminders(ctx context.Context) error {
	rows, err := a.db.QueryContext(ctx, dueRenewalsSQL, datasetKeepDays, deletionWarningDays)
	if err != nil {
		return err
	}
	var due []*renewalReminder
	for rows.Next() {
		var id int64
		var name, email, lang, stage string
		var it renewalItem
		if err := rows.Scan(&id, &name, &email, &lang, &stage, &it.item, &it.endsAt, &it.ebooks); err != nil {
			rows.Close()
			return err
		}
		if n := len(due); n > 0 && due[n-1].learnerID == id && due[n-1].stage == stage {
			due[n-1].items = append(due[n-1].items, it)
			continue
		}
		due = append(due, &renewalReminder{learnerID: id, name: name, email: email, lang: lang, stage: stage, items: []renewalItem{it}})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, d := range due {
		a.sendRenewalReminder(ctx, d)
	}
	return nil
}

func (a *App) sendRenewalReminder(ctx context.Context, d *renewalReminder) {
	var fresh []renewalItem
	for _, it := range d.items {
		res, err := a.db.ExecContext(ctx, `INSERT INTO renewal_reminders (learner_id, item, ends_at, stage) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, d.learnerID, it.item, it.endsAt, d.stage)
		if err != nil {
			a.logError(nil, fmt.Errorf("record renewal reminder for Learner %d: %w", d.learnerID, err))
			return
		}
		if n, err := res.RowsAffected(); err == nil && n == 1 {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) == 0 {
		return
	}
	lang := d.lang
	if !text.Supported(lang) {
		lang = text.Languages[0]
	}
	pricing, membership := a.publicBaseURL+"/pricing", a.publicBaseURL+"/my-membership"
	var subject, body string
	if d.stage == "deletion" {
		ended := fresh[0].endsAt.UTC()
		subject = text.T(lang, "email.renewal.deletion.subject")
		body = text.T(lang, "email.renewal.deletion.body", d.name, text.Date(lang, ended),
			text.Date(lang, ended.AddDate(0, 0, datasetKeepDays)), pricing, membership)
	} else {
		line := "email.renewal.line"
		if d.stage == "ended" {
			line = "email.renewal.line-ended"
		}
		var lines strings.Builder
		for _, it := range fresh {
			name := "email.renewal.item." + it.item
			if it.item == "cavetext-pro" && it.ebooks {
				name = "email.renewal.item.pro-ebooks"
			}
			lines.WriteString(text.T(lang, line, text.T(lang, name), text.Date(lang, it.endsAt.UTC())) + "\n")
		}
		subject = text.T(lang, "email.renewal."+d.stage+".subject")
		body = text.T(lang, "email.renewal."+d.stage+".body", d.name, strings.TrimRight(lines.String(), "\n"), pricing, membership)
	}
	if err := a.mailer.Send(ctx, mailer.Message{To: d.email, Subject: subject, Body: body}); err != nil {
		a.logError(nil, fmt.Errorf("send renewal reminder to Learner %d: %w", d.learnerID, err))
	}
}
