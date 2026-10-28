-- +goose Up
-- A Learner's CaveText account, made at their first CaveText sign-in. The place is handed out once and never reused.
CREATE TABLE cavetext_accounts (
	learner_id bigint PRIMARY KEY REFERENCES learners ON DELETE CASCADE,
	place bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
	created_at timestamptz NOT NULL DEFAULT now()
);

-- CaveText tokens, apart from site sessions so logging out of the site keeps CaveText signed in.
CREATE TABLE cavetext_tokens (
	token_hash bytea PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
	expires_at timestamptz NOT NULL
);
CREATE INDEX cavetext_tokens_learner ON cavetext_tokens (learner_id);

-- +goose Down
DROP TABLE cavetext_tokens;
DROP TABLE cavetext_accounts;
