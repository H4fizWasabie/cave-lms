-- +goose Up
-- A Learner's Submission to an Assignment is their files in it, up to 5 (spec #196). The data lives in the store
-- (ADR 0008) under storage_key; this keeps the records. Deleting the Assignment or the Learner deletes the records.
CREATE TABLE submission_files (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    assignment_id bigint NOT NULL REFERENCES assignments ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> ''),
    size bigint NOT NULL CHECK (size > 0),
    content_type text NOT NULL,
    storage_key text NOT NULL UNIQUE,
    uploaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submission_files_assignment ON submission_files (assignment_id, learner_id);
CREATE INDEX submission_files_learner ON submission_files (learner_id);

-- The storage keys of deleted Submission files, until the app deletes them from the store. A trigger fills it, so a
-- cascade from an Assignment or a Learner deleted by hand queues their files too.
CREATE TABLE stored_file_deletions (
    storage_key text PRIMARY KEY
);
-- +goose StatementBegin
CREATE FUNCTION queue_stored_file_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO stored_file_deletions (storage_key) VALUES (OLD.storage_key) ON CONFLICT DO NOTHING;
    RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER submission_files_deleted AFTER DELETE ON submission_files
    FOR EACH ROW EXECUTE FUNCTION queue_stored_file_deletion();

-- +goose Down
DROP TABLE submission_files;
DROP FUNCTION queue_stored_file_deletion();
DROP TABLE stored_file_deletions;
