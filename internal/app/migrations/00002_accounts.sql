-- +goose Up
CREATE TABLE learners (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX learners_email_key ON learners (lower(email));

-- Session and one-time tokens are stored as SHA-256 hashes, never in the clear.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);

CREATE TABLE one_time_tokens (
    token_hash bytea PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    purpose text NOT NULL,
    expires_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE one_time_tokens;
DROP TABLE sessions;
DROP TABLE learners;
