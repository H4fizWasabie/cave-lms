package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
)

// A CaveHost Deployment lists every sheet of its Spreadsheet (spec #536). The Learner ticks the ones the page may read; only the
// sheet in the pasted link starts ticked (the first when Cave cannot match it), and a sheet added to the Spreadsheet later is listed unticked by the next Sync (addNewCaveHostSheets).
// A ticked sheet shows the Clean-up preview of its first read for the Learner to confirm, as the first sheet does, and is served at
// sheets/<sheet name>.json once confirmed. Unticking takes it offline and drops its rows and Recipe, so ticking it again asks again.
// ponytail: a sheet taken out of the Spreadsheet stays in the list; a ticked one fails its Sync (#542), an unticked one reads as
// unreadable if ticked. Prune the list in Sync if that confuses Learners.

// tickCaveHostSheet ticks the sheet called by the form's sheet on the Learner's Deployment. Its preview is then waiting on the
// Deployment's page. A sheet that is not listed, or is ticked already, is left as it is.
func (a *App) tickCaveHostSheet(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavehost_sheets SET ticked = true WHERE deployment_id = $1 AND name = $2 AND NOT ticked`,
		v.ID, r.PostFormValue("sheet")); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// untickCaveHostSheet takes the ticked sheet called by the form's sheet offline: its link stops at once, and its rows and Recipe go,
// from the store too.
func (a *App) untickCaveHostSheet(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	var rowsKey, xlsx sql.NullString // the sheet's rows, and the fetched file if no ticked sheet waits now
	err := a.inTx(r.Context(), func(tx *sql.Tx) error {
		err := tx.QueryRowContext(r.Context(), `UPDATE cavehost_sheets sh SET ticked = false, reset = false, columns = NULL, settings = NULL, rows_key = NULL,
				confirmed_at = NULL
			FROM (SELECT id, rows_key FROM cavehost_sheets WHERE deployment_id = $1 AND name = $2 AND ticked FOR UPDATE) old
			WHERE sh.id = old.id RETURNING old.rows_key`, v.ID, r.PostFormValue("sheet")).Scan(&rowsKey)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		xlsx, err = dropCaveHostXlsx(r.Context(), tx, v.ID)
		return err
	})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	for _, key := range []sql.NullString{rowsKey, xlsx} {
		if key.Valid {
			if err := a.store.Delete(context.WithoutCancel(r.Context()), key.String); err != nil {
				a.logError(r, err)
			}
		}
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// resetCaveHostSheet is Reset columns: the confirmed sheet called by the form's sheet is read afresh. Its last good rows stay online
// and Sync leaves it alone, while its page shows a new Clean-up preview to confirm, which sets its columns and Recipe again. For a
// Learner who restructured the sheet on purpose, whose Syncs would fail on the columns they confirmed. Another sheet is left as it is.
func (a *App) resetCaveHostSheet(w http.ResponseWriter, r *http.Request) {
	v, ok := a.caveHostViewOf(w, r)
	if !ok {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `UPDATE cavehost_sheets SET reset = true WHERE deployment_id = $1 AND name = $2 AND ticked AND rows_key IS NOT NULL`,
		v.ID, r.PostFormValue("sheet")); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/cavehost/"+v.Name, http.StatusSeeOther)
}

// dropCaveHostXlsx forgets the fetched Spreadsheet file of the Deployment with this id once no ticked sheet waits for its Clean-up to
// be confirmed, whether a first one or after Reset columns, and gives the file's key to delete from the store, if it was kept.
func dropCaveHostXlsx(ctx context.Context, tx *sql.Tx, id int64) (key sql.NullString, err error) {
	err = tx.QueryRowContext(ctx, `UPDATE cavehost_deployments ch SET xlsx_key = NULL
		FROM (SELECT xlsx_key FROM cavehost_deployments WHERE deployment_id = $1 FOR UPDATE) old
		WHERE ch.deployment_id = $1 AND NOT EXISTS (SELECT 1 FROM cavehost_sheets sh WHERE sh.deployment_id = $1 AND sh.ticked AND (sh.rows_key IS NULL OR sh.reset))
		RETURNING old.xlsx_key`, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return key, err
}

// addNewCaveHostSheets lists the sheets of the fetched Spreadsheet data that the Deployment does not list yet, unticked and after
// the others, so nothing goes online without the Learner's say.
func (a *App) addNewCaveHostSheets(ctx context.Context, id int64, data []byte) error {
	names, err := a.sheetNamesOf(ctx, data)
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := a.db.ExecContext(ctx, `INSERT INTO cavehost_sheets (deployment_id, name, position, ticked)
			SELECT $1, $2, COALESCE(max(position) + 1, 0), false FROM cavehost_sheets WHERE deployment_id = $1
			ON CONFLICT (deployment_id, name) DO NOTHING`, id, name); err != nil {
			return err
		}
	}
	return nil
}
