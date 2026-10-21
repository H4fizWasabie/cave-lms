-- +goose Up
-- A Learner's Snippets, kept in CAVELMS. The extension keeps a copy for insertion; this is the source of truth.
CREATE TABLE cavetext_snippets (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES cavetext_accounts ON DELETE CASCADE,
	folder text NOT NULL,
	name text NOT NULL,
	shortcut text NOT NULL,
	body text NOT NULL,
	with_master_prompt boolean NOT NULL DEFAULT true,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cavetext_snippets_learner ON cavetext_snippets (learner_id);

-- +goose Down
DROP TABLE cavetext_snippets;
