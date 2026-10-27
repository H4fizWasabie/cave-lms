-- +goose Up
-- A Part keeps the sha256 of the Excel file it was cleaned from (ADR 0022), so the exact same file is refused as a new
-- Part even after its Workbook is deleted, which clears dataset_parts.workbook_id. A Part with no Workbook has none:
-- NULL matches nothing. Those that still have a Workbook take its fingerprint.
ALTER TABLE dataset_parts ADD COLUMN sha256 bytea;
UPDATE dataset_parts p SET sha256 = w.sha256 FROM workbooks w WHERE w.id = p.workbook_id;

-- +goose Down
ALTER TABLE dataset_parts DROP COLUMN sha256;
