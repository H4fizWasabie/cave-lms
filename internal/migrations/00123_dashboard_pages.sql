-- +goose Up
-- A Dashboard's pages (ADR 0018): the HTML a Learner made, such as with an AI chatbot, one row each. The bytes live in
-- the store (ADR 0008) under storage_key; size counts against the Storage allowance. A Dashboard with a current page
-- is served from it; one without is an engine Dashboard until the conversion (#375).
CREATE TABLE dashboard_pages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dashboard_id bigint NOT NULL REFERENCES dashboards ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    size bigint NOT NULL CHECK (size > 0),
    current boolean NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dashboard_pages_dashboard ON dashboard_pages (dashboard_id);
CREATE UNIQUE INDEX dashboard_pages_one_current ON dashboard_pages (dashboard_id) WHERE current;
-- Deleting a record, by hand or by cascade, queues its page for deletion from the store.
CREATE TRIGGER dashboard_pages_deleted AFTER DELETE ON dashboard_pages
    FOR EACH ROW EXECUTE FUNCTION queue_stored_file_deletion();

-- +goose Down
DROP TABLE dashboard_pages;
