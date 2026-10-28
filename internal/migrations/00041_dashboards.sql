-- +goose Up
-- A Dashboard is a Template filled with one Learner's Dataset and column choices. Its HTML is generated
-- on each request, never stored. template is a lab.Template key; choices maps blank keys to column names;
-- dataset is the lab.Dataset as JSON, NULL until the first upload; dataset_file is the uploaded file's name.
CREATE TABLE dashboards (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    template text NOT NULL,
    name text NOT NULL CHECK (name <> ''),
    choices jsonb NOT NULL DEFAULT '{}',
    dataset jsonb,
    dataset_file text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dashboards_learner ON dashboards (learner_id);

-- +goose Down
DROP TABLE dashboards;
