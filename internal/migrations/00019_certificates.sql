-- +goose Up
-- The Admin's Certificate details, one row. Logo and signature are storage keys, never URLs.
CREATE TABLE certificate_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    organisation_name text NOT NULL DEFAULT '',
    signatory_name text NOT NULL DEFAULT '',
    logo_key text,
    signature_key text
);
INSERT INTO certificate_settings (id) VALUES (1);

-- A Certificate copies the Learner's name and the Course title at issue time, so later edits never change it.
-- The ID is 16 random bytes, base64url. revoked_at is for revocation (issue #20); only unrevoked rows count.
-- A Course with Certificates cannot be deleted (RESTRICT), like Enrollments.
CREATE TABLE certificates (
    id text PRIMARY KEY CHECK (length(id) >= 22),
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    course_id bigint NOT NULL REFERENCES courses ON DELETE RESTRICT,
    learner_name text NOT NULL,
    course_title text NOT NULL,
    issued_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE UNIQUE INDEX certificates_one_valid ON certificates (learner_id, course_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE certificates;
DROP TABLE certificate_settings;
