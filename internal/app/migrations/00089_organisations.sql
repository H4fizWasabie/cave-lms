-- +goose Up
-- An Organisation is a body a Live Course is run for; its logo prints on that Course's Certificates (spec #195).
-- logo_key names a PNG or JPEG in the same storage as the Certificate background. A replaced logo's file is kept,
-- since Certificates issued with it still name it.
CREATE TABLE organisations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (name <> ''),
    logo_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- Only a Live Course names one. No cascade: an Organisation in use is not deleted.
ALTER TABLE courses ADD COLUMN organisation_id bigint REFERENCES organisations (id);

-- +goose Down
ALTER TABLE courses DROP COLUMN organisation_id;
DROP TABLE organisations;
