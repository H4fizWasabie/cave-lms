-- +goose Up
-- Review fixes to CaveHost (spec #536).
-- xlsx_key is the fetched Spreadsheet's file in the Disk store (ADR 0025), kept while a ticked sheet waits for its Clean-up to be
-- confirmed; it was called workbook_key, but a Workbook is the Lab's Excel file and a Spreadsheet is CaveHost's.
ALTER TABLE cavehost_deployments RENAME COLUMN workbook_key TO xlsx_key;

-- When a sheet's first Clean-up was confirmed: data.json is the ticked, confirmed sheet confirmed first, so it does not change
-- when another sheet is confirmed later. A sheet confirmed before this column keeps its place by position.
ALTER TABLE cavehost_sheets ADD COLUMN confirmed_at timestamptz;
UPDATE cavehost_sheets SET confirmed_at = now() WHERE rows_key IS NOT NULL;

-- How a CaveHost Deployment's row of deployments ended: its Learner deleted it, or the 90 days after their last CaveHost Plan
-- period did. NULL for every other Deployment, so the Admin's list can say which.
ALTER TABLE deployments ADD COLUMN deleted_by text CHECK (deleted_by IN ('learner', 'lapsed'));

-- +goose Down
ALTER TABLE deployments DROP COLUMN deleted_by;
ALTER TABLE cavehost_sheets DROP COLUMN confirmed_at;
ALTER TABLE cavehost_deployments RENAME COLUMN xlsx_key TO workbook_key;
