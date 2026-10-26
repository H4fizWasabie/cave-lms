package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/H4fizWasabie/cave-lms/internal/lab"
)

// convertEngineDashboards is a step at every start (#375, ADR 0018): each engine Dashboard with a Dataset and blocks
// gets the engine's page as its first current page, its chart script inlined, as the page policy allows no script
// from Cave, and its rows read from data.json beside it. The page is stored and counted like an uploaded one, over the
// allowance too. A Dashboard that has a page is skipped, so starting again changes nothing; one without a Dataset or
// blocks has no engine page and gets none, so it offers to add one.
// Its Deployment, name and awake state are untouched. One that fails is logged and left to the engine until the next
// start. It goes with the engine (#376), once it has run on production.
func (a *App) convertEngineDashboards(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `SELECT d.id FROM dashboards d
		WHERE NOT EXISTS (SELECT 1 FROM dashboard_pages pg WHERE pg.dashboard_id = d.id) AND jsonb_typeof(d.config) = 'object'
		AND EXISTS (SELECT 1 FROM datasets ds WHERE ds.dashboard_id = d.id AND ds.state = 'current') ORDER BY d.id`)
	if err != nil {
		a.logError(nil, fmt.Errorf("list engine Dashboards: %w", err))
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			a.logError(nil, fmt.Errorf("list engine Dashboards: %w", err))
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		a.logError(nil, fmt.Errorf("list engine Dashboards: %w", err))
		return
	}
	if len(ids) == 0 {
		return
	}
	done := 0
	for _, id := range ids {
		if err := a.convertEngineDashboard(ctx, id); err != nil {
			a.logError(nil, fmt.Errorf("convert Dashboard %d to a page: %w", id, err))
			continue
		}
		done++
	}
	a.logInfo("converted %d of %d engine Dashboards to pages", done, len(ids))
}

// convertEngineDashboard converts one Dashboard, as convertEngineDashboards says.
func (a *App) convertEngineDashboard(ctx context.Context, id int64) error {
	d, err := a.loadDashboard(ctx, "id = $1", id)
	if err != nil || d == nil {
		return err
	}
	if d.Config == nil { // its blocks went since the list
		return nil
	}
	if err := a.readDataset(ctx, d); err != nil {
		return err
	}
	if d.Dataset == nil { // its Dataset went since the list
		return nil
	}
	html, err := lab.Page(d.Template, d.Name, d.Theme, d.Config, d.Dataset.Columns, "./data.json")
	if err != nil {
		return err
	}
	// After the template, so the script is written as it is; charts.js holds no </script.
	tag := []byte(`<script src="charts.js"></script>`)
	if bytes.Count(html, tag) != 1 {
		return fmt.Errorf("the engine page loads charts.js %d times, want 1", bytes.Count(html, tag))
	}
	html = bytes.Replace(html, tag, append(append([]byte("<script>"), lab.ChartScript...), "</script>"...), 1)
	key, err := newPageKey()
	if err != nil {
		return err
	}
	if err := a.datasets.Put(ctx, key, html); err != nil {
		return fmt.Errorf("put page %s: %w", key, err)
	}
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		// Locking the Dashboard makes two starts at once take turns, so it gets one page.
		var paged bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM dashboard_pages WHERE dashboard_id = d.id)
			FROM dashboards d WHERE d.id = $1 FOR UPDATE`, id).Scan(&paged); err != nil {
			return err
		}
		if paged {
			return errPaged
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO dashboard_pages (dashboard_id, storage_key, size, current) VALUES ($1, $2, $3, true)`,
			id, key, len(html))
		return err
	})
	if err != nil {
		a.dropDataset(ctx, nil, key)
	}
	if errors.Is(err, errPaged) {
		return nil
	}
	return err
}

var errPaged = errors.New("the Dashboard has a page")
