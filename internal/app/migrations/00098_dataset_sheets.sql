-- +goose Up
-- upload_settings is what a Work Dashboard remembers for its next upload (lab.Settings): the picked sheet,
-- whether same-column sheets combine, and "2-row header". sheets lists the upload's sheets (lab.Sheet) and
-- used the ones that went into the Dataset, shown in the preview (spec #237).
ALTER TABLE dashboards ADD COLUMN upload_settings jsonb NOT NULL DEFAULT '{}';
ALTER TABLE datasets ADD COLUMN sheets jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN used jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE datasets DROP COLUMN sheets, DROP COLUMN used;
ALTER TABLE dashboards DROP COLUMN upload_settings;
