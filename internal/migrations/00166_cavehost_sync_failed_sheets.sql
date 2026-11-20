-- +goose Up
-- The names of the sheets the last Sync of a CaveHost Deployment failed to read (a JSON list), so the page can say of each sheet
-- whether its last Sync failed. A Sync overwrites it, like the other columns of the last Sync.
ALTER TABLE cavehost_deployments ADD COLUMN sync_failed_sheets jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE cavehost_deployments DROP COLUMN sync_failed_sheets;
