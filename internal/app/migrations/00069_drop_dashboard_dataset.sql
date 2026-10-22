-- +goose Up
-- Every Dataset is in the Dataset store by now: App.New copies them in between 00068 and this migration.
ALTER TABLE dashboards DROP COLUMN dataset;

-- +goose Down
ALTER TABLE dashboards ADD COLUMN dataset jsonb;
