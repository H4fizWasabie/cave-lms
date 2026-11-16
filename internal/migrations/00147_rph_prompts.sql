-- +goose Up
-- The Daily prompt limit (see the glossary): each RPH prompt a Teacher makes is recorded as who and when, never what, so that no
-- history of prompts can be shown and the Teachers Portal can count how many were made since Malaysia midnight.
CREATE TABLE rph_prompts (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
	made_at timestamptz NOT NULL
);
CREATE INDEX rph_prompts_learner_made ON rph_prompts (learner_id, made_at);

-- +goose Down
DROP TABLE rph_prompts;
