-- +goose Up
-- A Dashboard is a Practice Dashboard or a Work Dashboard (ADR 0013); every existing one is Practice. config is a
-- Work Dashboard's blocks and filter bar (lab.Config), naming its Dataset's columns; NULL until its first confirm.
ALTER TABLE dashboards ADD COLUMN kind text NOT NULL DEFAULT 'practice' CHECK (kind IN ('practice', 'work')),
    ADD COLUMN config jsonb;

-- A Work Dashboard's Datasets, one row each, pending until the Learner confirms Clean-up's preview (spec #237).
-- Practice Dashboards keep theirs on dashboards. The data lives in the store (ADR 0008) under storage_key;
-- saved_size is the bytes written there, which count against the Storage allowance unless pending.
-- cleanup is what Clean-up found besides the data: its changes, guesses, sheets, header row and the upload settings
-- it read with, which the Dashboard remembers on confirm (app.cleaned).
CREATE TABLE datasets (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dashboard_id bigint NOT NULL REFERENCES dashboards ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    file_name text NOT NULL,
    saved_size bigint NOT NULL CHECK (saved_size > 0),
    row_count integer NOT NULL,
    state text NOT NULL CHECK (state IN ('pending', 'current', 'earlier')),
    cleanup jsonb NOT NULL DEFAULT '{}',
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz
);
CREATE INDEX datasets_dashboard ON datasets (dashboard_id);
CREATE UNIQUE INDEX datasets_one_current ON datasets (dashboard_id) WHERE state = 'current';
CREATE UNIQUE INDEX datasets_one_pending ON datasets (dashboard_id) WHERE state = 'pending';
-- Deleting a record, by hand, by cascade or by the prune, queues its data for deletion from the store.
CREATE TRIGGER datasets_deleted AFTER DELETE ON datasets
    FOR EACH ROW EXECUTE FUNCTION queue_stored_file_deletion();

-- +goose Down
DROP TABLE datasets;
ALTER TABLE dashboards DROP COLUMN kind, DROP COLUMN config;
