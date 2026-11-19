package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// Sync (spec #536) fetches a CaveHost Deployment's Spreadsheet again and, for each of its ticked sheets whose first Clean-up was
// confirmed, reads it with the saved Recipe, without asking, and replaces the sheet's rows with what it reads. No history is
// kept. Every hour the ticker Syncs each awake Deployment, the oldest Sync first; Sync now Syncs one. At most one Sync runs per
// Deployment at a time (caveHostSyncing).
// A sheet that fails (syncFailure) keeps its last good rows and the others still Sync; the Sync records the reasons, and the first
// failing Sync of a run of them emails the Learner, once, until a Sync works (recordSync).
// ponytail: Syncs run one after another; a worker pool if one hour stops being enough for all of them.
const caveHostSyncEvery = time.Hour

// syncNote is one of Clean-up's changes in a Sync, with the sheet it was made to.
type syncNote struct {
	Sheet string `json:"sheet"`
	lab.Change
}

// syncFailure is why a Sync failed: for a sheet, or for the whole Spreadsheet when sheet is empty.
type syncFailure struct {
	sheet string
	err   error
}

// in words f in lang, for the Learner to act on.
func (f syncFailure) in(lang string) string {
	var problem lab.Problem
	var msg string
	if errors.As(f.err, &problem) && problem.Key == "lab.err.on-sheet" { // the sheet is named in front of the reason already
		if inner, ok := problem.Args[1].(lab.Problem); ok {
			problem, f.err = inner, inner
		}
	}
	switch {
	case errors.Is(f.err, errSpreadsheetClosed):
		msg = text.T(lang, "cavehost.err.closed")
	case errors.Is(f.err, errSpreadsheetTooBig):
		msg = text.T(lang, "cavehost.err.too-big")
	case errors.As(f.err, &problem) && problem.Key == "lab.err.add-columns":
		msg = text.T(lang, "cavehost.sync.err.columns", problem.Args...)
	case errors.As(f.err, &problem) && problem.Key == "lab.err.too-many-rows":
		msg = text.T(lang, "cavehost.sync.err.rows", problem.Args[0], problem.Args[1])
	case errors.As(f.err, &problem):
		msg = problem.In(lang)
	default:
		msg = text.T(lang, "cavehost.sync.err.unknown")
	}
	if f.sheet != "" {
		return f.sheet + ": " + msg
	}
	return msg
}

// syncCaveHostEveryHour runs syncCaveHostDeployments once an hour until done is closed.
func (a *App) syncCaveHostEveryHour(done <-chan struct{}) {
	tick := time.NewTicker(caveHostSyncEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			a.survive("CaveHost Sync", func() { a.syncCaveHostDeployments(context.Background()) })
		}
	}
}

// syncCaveHostDeployments Syncs every awake CaveHost Deployment with a confirmed sheet, one at a time, the one Synced longest ago
// first and those never Synced before all. One that fails or panics does not stop the others.
func (a *App) syncCaveHostDeployments(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `SELECT dep.id FROM deployments dep JOIN cavehost_deployments ch ON ch.deployment_id = dep.id
		WHERE `+caveHostAwakeSQL+` AND EXISTS (SELECT 1 FROM cavehost_sheets sh WHERE sh.deployment_id = dep.id AND sh.ticked AND NOT sh.reset AND sh.rows_key IS NOT NULL)
		ORDER BY ch.synced_at NULLS FIRST, dep.id`)
	if err != nil {
		a.logError(nil, err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		a.logError(nil, err)
		return
	}
	for _, id := range ids {
		a.survive("CaveHost Sync", func() { a.syncCaveHost(ctx, id) })
	}
}

// caveHostSheet is a ticked, confirmed sheet to Sync.
type caveHostSheet struct {
	id       int64
	name     string
	rowsKey  string
	columns  []lab.Column
	settings lab.Settings
}

// syncCaveHost Syncs the CaveHost Deployment with this id, and records the Sync on it. It does nothing while another Sync of the
// Deployment runs.
func (a *App) syncCaveHost(ctx context.Context, id int64) {
	if _, busy := a.caveHostSyncing.LoadOrStore(id, true); busy {
		return
	}
	defer a.caveHostSyncing.Delete(id)
	spreadsheet, sheets, err := a.caveHostSheetsToSync(ctx, id)
	if err != nil {
		a.logError(nil, err)
		return
	}
	if len(sheets) == 0 {
		return
	}
	notes, failures := []syncNote{}, []syncFailure{}
	if data, err := a.fetchSpreadsheet(ctx, spreadsheet); err != nil {
		failures = append(failures, syncFailure{err: err})
	} else {
		if err := a.addNewCaveHostSheets(ctx, id, data); err != nil { // a Spreadsheet that does not open fails each sheet below too
			a.logError(nil, err)
		}
		for _, sh := range sheets { // one sheet failing does not stop the next
			changes, err := a.syncCaveHostSheet(ctx, data, sh)
			if err != nil {
				failures = append(failures, syncFailure{sh.name, err})
				continue
			}
			for _, c := range changes {
				notes = append(notes, syncNote{sh.name, c})
			}
		}
	}
	if err := a.recordSync(ctx, id, notes, failures); err != nil {
		a.logError(nil, err)
	}
}

// caveHostSheetsToSync is the Spreadsheet ID of the Deployment and its ticked sheets with a confirmed Clean-up, in order.
func (a *App) caveHostSheetsToSync(ctx context.Context, id int64) (spreadsheet string, sheets []caveHostSheet, err error) {
	if err = a.db.QueryRowContext(ctx, `SELECT spreadsheet_id FROM cavehost_deployments WHERE deployment_id = $1`, id).Scan(&spreadsheet); err != nil {
		return "", nil, err
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, rows_key, columns, settings FROM cavehost_sheets
		WHERE deployment_id = $1 AND ticked AND NOT reset AND rows_key IS NOT NULL ORDER BY position`, id)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sh caveHostSheet
		var columns, settings []byte
		if err = rows.Scan(&sh.id, &sh.name, &sh.rowsKey, &columns, &settings); err == nil {
			err = json.Unmarshal(columns, &sh.columns)
		}
		if err == nil {
			err = json.Unmarshal(settings, &sh.settings)
		}
		if err != nil {
			return "", nil, err
		}
		sheets = append(sheets, sh)
	}
	return spreadsheet, sheets, rows.Err()
}

// syncCaveHostSheet reads the sheet from the fetched Spreadsheet data with its saved Recipe and, once that works, replaces its rows
// with what it read. It gives the changes Clean-up made. A sheet that does not read leaves its rows as they were.
func (a *App) syncCaveHostSheet(ctx context.Context, data []byte, sh caveHostSheet) ([]lab.Change, error) {
	settings := sh.settings
	if settings.Recipe == nil {
		return nil, errors.New("CaveHost sheet has no Recipe")
	}
	recipe := *settings.Recipe
	recipe.Columns = sh.columns
	settings.Recipe = &recipe
	release, err := a.waitForCleanUp(ctx)
	if err != nil {
		return nil, err
	}
	res, err := lab.Clean(bytes.NewReader(data), settings)
	release()
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(res.PickedSheet, sh.name) { // Clean-up read another sheet in its place
		return nil, lab.Problem{Key: "cavehost.sync.err.sheet", Args: []any{sh.name}}
	}
	mapped, _, mapChanges := lab.MapColumns(sh.columns, res.Dataset, recipe.Map)
	more, changes, err := lab.AddPart(sh.columns, 0, mapped, 0, recipe.MonthColumns)
	if err != nil {
		return nil, err
	}
	if p, ok := mostlyEmptied(more, changes); ok { // a column the Spreadsheet no longer fills as it did: not a change to apply by itself
		return nil, p
	}
	day := res.PartDate
	if day == "" {
		day = a.now().In(malaysia).Format(time.DateOnly)
	}
	rows, err := lab.PartRows(more, day)
	if err != nil {
		return nil, err
	}
	key, err := newCaveHostKey("rows")
	if err == nil {
		err = a.store.Put(ctx, key, rows)
	}
	if err != nil {
		return nil, err
	}
	// The sheet may have been unticked, or reset, while it was read: then these rows are dropped, and what was online stays.
	set, err := a.db.ExecContext(ctx, `UPDATE cavehost_sheets SET rows_key = $2 WHERE id = $1 AND ticked AND NOT reset AND rows_key IS NOT NULL`, sh.id, key)
	if err == nil {
		var n int64
		if n, err = set.RowsAffected(); err == nil && n == 0 {
			_ = a.store.Delete(context.WithoutCancel(ctx), key)
			return nil, nil
		}
	}
	if err != nil {
		_ = a.store.Delete(context.WithoutCancel(ctx), key)
		return nil, err
	}
	_ = a.store.Delete(context.WithoutCancel(ctx), sh.rowsKey) // the rows it replaced are not kept
	return append(append(res.Changes, mapChanges...), changes...), nil
}

// mostlyEmptied is a Problem when a column of the Sync's rows has had more than half of its filled cells left empty by AddPart
// because they do not fit the column's confirmed type, from its changes; a Learner who typed text into a number column would
// otherwise blank their charts without knowing.
func mostlyEmptied(rows *lab.Dataset, changes []lab.Change) (lab.Problem, bool) {
	for _, c := range changes {
		if !strings.HasPrefix(c.Key, "lab.clean.not-") {
			continue
		}
		i := slices.IndexFunc(rows.Columns, func(col lab.Column) bool { return col.Name == c.Column })
		if i < 0 {
			continue
		}
		kept := 0
		for _, row := range rows.Rows {
			if row[i] != nil {
				kept++
			}
		}
		if n := c.Args[0]; n*2 > kept+n {
			return lab.Problem{Key: "cavehost.sync.err.emptied", Args: []any{c.Column, n, kept + n}}, true
		}
	}
	return lab.Problem{}, false
}

// recordSync saves a Sync on its Deployment: when it ran, whether it worked, the reason in each language if not, and its change
// notes. The first failing Sync of a run of them emails the Learner (emailSyncFailure); a Sync that works ends the run.
func (a *App) recordSync(ctx context.Context, id int64, notes []syncNote, failures []syncFailure) error {
	changes, err := json.Marshal(notes)
	if err != nil {
		return err
	}
	status, reason, in := "ok", []byte(nil), map[string]string{}
	if len(failures) > 0 {
		status = "failed"
		for _, lang := range text.Languages {
			var parts []string
			for _, f := range failures {
				parts = append(parts, f.in(lang))
			}
			in[lang] = strings.Join(parts, " ")
		}
		if reason, err = json.Marshal(in); err != nil {
			return err
		}
		for _, f := range failures {
			var problem lab.Problem
			if !errors.Is(f.err, errSpreadsheetClosed) && !errors.Is(f.err, errSpreadsheetTooBig) && !errors.As(f.err, &problem) {
				a.logError(nil, f.err) // Google or the store failing, not something the Learner can fix
			}
		}
	}
	var emailed bool // the Learner was emailed about this run of failures already
	err = a.db.QueryRowContext(ctx, `UPDATE cavehost_deployments SET synced_at = $2, sync_status = $3, sync_reason = $4, sync_changes = $5,
		failure_emailed = failure_emailed AND $3 = 'failed' WHERE deployment_id = $1 RETURNING failure_emailed`,
		id, a.now(), status, reason, changes).Scan(&emailed)
	if err == nil && status == "failed" && !emailed && a.emailSyncFailure(ctx, id, in) {
		_, err = a.db.ExecContext(ctx, `UPDATE cavehost_deployments SET failure_emailed = true WHERE deployment_id = $1`, id)
	}
	return err
}

// emailSyncFailure emails the Learner who owns the Deployment that its Sync failed, with the reason (in is the reason in each
// language) in their language, and reports whether it went. A failed email is only logged: the page shows the failure, and the
// next failing Sync tries again.
func (a *App) emailSyncFailure(ctx context.Context, id int64, in map[string]string) bool {
	var learner, email, lang, name string
	err := a.db.QueryRowContext(ctx, `SELECT l.name, l.email, COALESCE(l.language, ''), dep.name FROM deployments dep
		JOIN learners l ON l.id = dep.learner_id WHERE dep.id = $1`, id).Scan(&learner, &email, &lang, &name)
	if err == nil {
		if !text.Supported(lang) {
			lang = text.Languages[0]
		}
		err = a.mailer.Send(ctx, mailer.Message{To: email, Subject: text.T(lang, "email.cavehost.sync-failed.subject", name),
			Body: text.T(lang, "email.cavehost.sync-failed.body", learner, name, in[lang], a.publicBaseURL+"/cavehost/"+name)})
	}
	if err != nil {
		a.logError(nil, fmt.Errorf("send CaveHost Sync failure email for Deployment %d: %w", id, err))
	}
	return err == nil
}

// syncCaveHostNow is Sync now: it Syncs the Learner's Deployment at once, and shows the CaveHost page of it with the result.
func (a *App) syncCaveHostNow(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	if v.Confirmed && v.Awake {
		a.syncCaveHost(context.WithoutCancel(r.Context()), v.ID) // the Learner closing the page does not stop it half-way
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// caveHostSync is the last Sync of a CaveHost Deployment, as its page shows it.
type caveHostSync struct {
	At      string   // when, in the Learner's language; empty before the first Sync
	Failed  bool     // it did not work
	Reason  string   // why, when it failed
	Changes []string // what Clean-up changed, each with its sheet
}

// caveHostSyncOf reads the last Sync from the columns recordSync saved.
func caveHostSyncOf(lang string, at sql.NullTime, status sql.NullString, reason, changes []byte) (caveHostSync, error) {
	var s caveHostSync
	if !at.Valid {
		return s, nil
	}
	s.At, s.Failed = dateTimeIn(lang, at.Time), status.String == "failed"
	if len(reason) > 0 {
		var in map[string]string
		if err := json.Unmarshal(reason, &in); err != nil {
			return s, err
		}
		s.Reason = in[lang]
	}
	var notes []syncNote
	if len(changes) > 0 {
		if err := json.Unmarshal(changes, &notes); err != nil {
			return s, err
		}
	}
	for _, n := range notes {
		s.Changes = append(s.Changes, n.Sheet+": "+n.In(lang))
	}
	return s, nil
}
