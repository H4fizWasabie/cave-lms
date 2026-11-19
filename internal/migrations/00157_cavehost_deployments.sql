-- +goose Up
-- A CaveHost Deployment (spec #536) is a row of deployments, so its name and Link key share the Lab's one name space and the
-- Username redirect (ADR 0023) finds it, with no Dashboard (dashboard_id stays NULL). This row says it is CaveHost's and
-- holds its Spreadsheet: the ID from the pasted link, and the gid of the sheet in it. workbook_key is the xlsx as fetched,
-- in the local Disk store (ADR 0025), kept only until the Learner confirms the first sheet's Clean-up.
CREATE TABLE cavehost_deployments (
    deployment_id bigint PRIMARY KEY REFERENCES deployments ON DELETE CASCADE,
    spreadsheet_id text NOT NULL,
    gid text NOT NULL DEFAULT '',
    workbook_key text
);

-- One sheet of a Spreadsheet. Until its first Clean-up is confirmed, columns, settings and rows_key are NULL. Then columns
-- are the sheet's columns, settings the Lab's Settings with its Recipe (how every later Sync reads it), and rows_key the
-- store key of its last good rows. position orders the sheets; data.json is the first ticked one.
CREATE TABLE cavehost_sheets (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id bigint NOT NULL REFERENCES cavehost_deployments ON DELETE CASCADE,
    name text NOT NULL,
    position integer NOT NULL,
    ticked boolean NOT NULL DEFAULT true,
    columns jsonb,
    settings jsonb,
    rows_key text,
    UNIQUE (deployment_id, name)
);

-- A CaveHost Deployment's pages, as a Dashboard's (dashboard_pages), kept in the Disk store: the current one and, once a
-- page can be replaced, the earlier ones.
CREATE TABLE cavehost_pages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id bigint NOT NULL REFERENCES cavehost_deployments ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    size bigint NOT NULL CHECK (size > 0),
    current boolean NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cavehost_pages_deployment ON cavehost_pages (deployment_id);
CREATE UNIQUE INDEX cavehost_pages_one_current ON cavehost_pages (deployment_id) WHERE current;

-- +goose Down
DROP TABLE cavehost_pages;
DROP TABLE cavehost_sheets;
DROP TABLE cavehost_deployments;
