-- +goose Up
-- A Dataset is stored as Parts, one per confirmed upload (ADR 0022). The datasets table now holds Parts: a Part is
-- 'current' (in position order), 'pending' (an upload awaiting confirmation) or, for a Dataset from before Parts,
-- 'earlier': kept read-only and counted in storage until its Dashboard's data next changes, then deleted. Each Part
-- keeps its Workbook link. part_date is the day the Part was uploaded, or for one from before Parts its confirm day;
-- date_source says which. legacy marks a Part whose stored object is still the old whole-table JSON, which the
-- startup conversion rewrites into the Part shape.
ALTER TABLE datasets RENAME TO dataset_parts;
ALTER INDEX datasets_dashboard RENAME TO dataset_parts_dashboard;
ALTER INDEX datasets_workbook RENAME TO dataset_parts_workbook;
ALTER TRIGGER datasets_deleted ON dataset_parts RENAME TO dataset_parts_deleted;

DROP INDEX datasets_one_current;
ALTER TABLE dataset_parts
    ADD COLUMN position integer,
    ADD COLUMN part_date date,
    ADD COLUMN date_source text NOT NULL DEFAULT 'upload' CHECK (date_source IN ('upload', 'before-parts')),
    ADD COLUMN legacy boolean NOT NULL DEFAULT false;
UPDATE dataset_parts SET position = 1 WHERE state = 'current';
UPDATE dataset_parts SET part_date = COALESCE(confirmed_at, uploaded_at)::date, date_source = 'before-parts', legacy = true;
ALTER TABLE dataset_parts ALTER COLUMN part_date SET NOT NULL;
CREATE UNIQUE INDEX dataset_parts_position ON dataset_parts (dashboard_id, position) WHERE state = 'current';

-- The Dataset's columns (names and types), set by its first Part and replaced by Replace data; NULL while it has none.
-- Filled for Datasets from before Parts by the startup conversion.
ALTER TABLE dashboards ADD COLUMN dataset_columns jsonb;

-- +goose Down
-- A Dashboard with several Parts keeps its last one only.
DELETE FROM dataset_parts p WHERE p.state = 'current'
    AND p.position < (SELECT max(o.position) FROM dataset_parts o WHERE o.dashboard_id = p.dashboard_id AND o.state = 'current');
ALTER TABLE dashboards DROP COLUMN dataset_columns;
DROP INDEX dataset_parts_position;
ALTER TABLE dataset_parts DROP COLUMN position, DROP COLUMN part_date, DROP COLUMN date_source, DROP COLUMN legacy;
CREATE UNIQUE INDEX datasets_one_current ON dataset_parts (dashboard_id) WHERE state = 'current';
ALTER TRIGGER dataset_parts_deleted ON dataset_parts RENAME TO datasets_deleted;
ALTER INDEX dataset_parts_workbook RENAME TO datasets_workbook;
ALTER INDEX dataset_parts_dashboard RENAME TO datasets_dashboard;
ALTER TABLE dataset_parts RENAME TO datasets;
