-- +goose Up
-- A Dataset's data lives in the Dataset store (ADR 0008) under dataset_key, a new key on every upload, so a cached
-- copy is never stale. NULL until the first upload. dataset_size is the uploaded file's size in bytes.
-- The old dataset column is copied into the store at startup, then dropped by 00069.
ALTER TABLE dashboards ADD COLUMN dataset_key text UNIQUE, ADD COLUMN dataset_size bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE dashboards DROP COLUMN dataset_key, DROP COLUMN dataset_size;
