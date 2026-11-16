-- +goose Up
-- The Daily prompt limit (see the glossary) is one for RPH prompts and one for OPR prompts, so the record of prompts made now
-- says which kind each is. Every prompt made so far is an RPH prompt.
ALTER TABLE rph_prompts RENAME TO prompts_made;
ALTER TABLE prompts_made ADD COLUMN kind text NOT NULL DEFAULT 'rph' CHECK (kind IN ('rph', 'opr'));
ALTER TABLE prompts_made ALTER COLUMN kind DROP DEFAULT;
DROP INDEX rph_prompts_learner_made;
CREATE INDEX prompts_made_learner_kind_made ON prompts_made (learner_id, kind, made_at);

-- +goose Down
DELETE FROM prompts_made WHERE kind <> 'rph';
DROP INDEX prompts_made_learner_kind_made;
ALTER TABLE prompts_made DROP COLUMN kind;
CREATE INDEX rph_prompts_learner_made ON prompts_made (learner_id, made_at);
ALTER TABLE prompts_made RENAME TO rph_prompts;
