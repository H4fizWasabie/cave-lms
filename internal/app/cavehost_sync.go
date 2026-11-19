package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// Sync (spec #536) fetches a CaveHost Deployment's Spreadsheet again and, for each of its ticked sheets whose first Clean-up was
// confirmed, reads it with the saved Recipe, without asking, and replaces the sheet's rows with what it reads. No history is
// kept. Every hour the ticker Syncs each awake Deployment, the oldest Sync first; Sync now Syncs one. At most one Sync runs per
// Deployment at a time (caveHostSyncing).
// ponytail: a failed Sync only records its status and reason and leaves the rows as they were; keeping last good rows per
// sheet by the failure rules, and the failure email, are #542. Syncs run one after another; a worker pool if one hour stops
// being enough for all of them.
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
	switch {
	case errors.Is(f.err, errSpreadsheetClosed):
		msg = text.T(lang, "cavehost.err.closed")
	case errors.Is(f.err, errSpreadsheetTooBig):
		msg = text.T(lang, "cavehost.err.too-big")
	case errors.As(f.err, &problem) && problem.Key == "lab.err.add-columns":
		msg = text.T(lang, "cavehost.sync.err.columns", problem.Args...)
	case errors.As(f.err, &problem):
		msg = problem.In(lang)
	default:
		msg = text.T(lang, "cavehost.sync.err.other")
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
		WHERE `+caveHostAwakeSQL+` AND EXISTS (SELECT 1 FROM cavehost_sheets sh WHERE sh.deployment_id = dep.id AND sh.ticked AND sh.rows_key IS NOT NULL)
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
		WHERE deployment_id = $1 AND ticked AND rows_key IS NOT NULL ORDER BY position`, id)
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
	if _, err = a.db.ExecContext(ctx, `UPDATE cavehost_sheets SET rows_key = $2 WHERE id = $1`, sh.id, key); err != nil {
		_ = a.store.Delete(context.WithoutCancel(ctx), key)
		return nil, err
	}
	_ = a.store.Delete(context.WithoutCancel(ctx), sh.rowsKey) // the rows it replaced are not kept
	return append(append(res.Changes, mapChanges...), changes...), nil
}

// recordSync saves a Sync on its Deployment: when it ran, whether it worked, the reason in each language if not, and its change notes.
func (a *App) recordSync(ctx context.Context, id int64, notes []syncNote, failures []syncFailure) error {
	changes, err := json.Marshal(notes)
	if err != nil {
		return err
	}
	status, reason := "ok", []byte(nil)
	if len(failures) > 0 {
		status = "failed"
		in := map[string]string{}
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
	_, err = a.db.ExecContext(ctx, `UPDATE cavehost_deployments SET synced_at = $2, sync_status = $3, sync_reason = $4, sync_changes = $5
		WHERE deployment_id = $1`, id, a.now(), status, reason, changes)
	return err
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
