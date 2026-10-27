-- +goose Up
-- A Learner's Workbooks (ADR 0021): the Excel files they uploaded, kept as uploaded, owned by the Learner and not by a
-- Dashboard. The bytes live in the store (ADR 0008) under storage_key and are served only through the app; size counts
-- against the Storage allowance. A Dataset records the Workbook it was cleaned from, if any; deleting the Workbook
-- clears that link and leaves the Dataset.
CREATE TABLE workbooks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    name text NOT NULL,
    size bigint NOT NULL CHECK (size > 0),
    sha256 bytea NOT NULL, -- the same file uploaded again by the same Learner stays one Workbook
    uploaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workbooks_learner ON workbooks (learner_id, sha256);
-- Deleting a record, by hand or by cascade, queues its file for deletion from the store.
CREATE TRIGGER workbooks_deleted AFTER DELETE ON workbooks
    FOR EACH ROW EXECUTE FUNCTION queue_stored_file_deletion();

ALTER TABLE datasets ADD COLUMN workbook_id bigint REFERENCES workbooks ON DELETE SET NULL;
CREATE INDEX datasets_workbook ON datasets (workbook_id);

-- +goose Down
ALTER TABLE datasets DROP COLUMN workbook_id;
DROP TABLE workbooks;
