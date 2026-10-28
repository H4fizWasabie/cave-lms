-- +goose Up
-- date_source says where a Part's date came from: the sources Clean-up finds in the Excel (cell, sheet, file), the upload
-- day, a date the Learner chose in the preview, or, for a Part from before Parts, 'before-parts'. 00128 allowed only
-- 'upload' and 'before-parts', and the real source sat in the Part's cleanup JSON.
ALTER TABLE dataset_parts DROP CONSTRAINT dataset_parts_date_source_check;
ALTER TABLE dataset_parts ADD CONSTRAINT dataset_parts_date_source_check
    CHECK (date_source IN ('cell', 'sheet', 'file', 'upload', 'chosen', 'before-parts'));
UPDATE dataset_parts SET date_source = cleanup->>'dateFrom'
    WHERE date_source = 'upload' AND cleanup->>'dateFrom' IN ('cell', 'sheet', 'file', 'chosen');

-- +goose Down
UPDATE dataset_parts SET date_source = 'upload' WHERE date_source NOT IN ('upload', 'before-parts');
ALTER TABLE dataset_parts DROP CONSTRAINT dataset_parts_date_source_check;
ALTER TABLE dataset_parts ADD CONSTRAINT dataset_parts_date_source_check CHECK (date_source IN ('upload', 'before-parts'));
