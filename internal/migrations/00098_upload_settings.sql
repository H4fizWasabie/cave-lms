-- +goose Up
-- upload_settings is what a Work Dashboard remembers for its next upload (lab.Settings): the picked sheet,
-- whether same-column sheets combine, and "2-row header" (spec #237).
ALTER TABLE dashboards ADD COLUMN upload_settings jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE dashboards DROP COLUMN upload_settings;
