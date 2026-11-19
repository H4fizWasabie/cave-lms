-- +goose Up
-- The last Sync of a CaveHost Deployment (spec #536): when it ran, whether it worked, why not (the text in each language, as
-- the Learner reads it) and its change notes (a JSON list of Clean-up changes, each with its sheet). A Sync overwrites them; no
-- history is kept. synced_at is NULL until the first Sync, which the hourly Sync runs first.
ALTER TABLE cavehost_deployments
    ADD COLUMN synced_at timestamptz,
    ADD COLUMN sync_status text CHECK (sync_status IN ('ok', 'failed')),
    ADD COLUMN sync_reason jsonb,
    ADD COLUMN sync_changes jsonb;

-- +goose Down
ALTER TABLE cavehost_deployments
    DROP COLUMN sync_changes,
    DROP COLUMN sync_reason,
    DROP COLUMN sync_status,
    DROP COLUMN synced_at;
