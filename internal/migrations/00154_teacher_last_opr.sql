-- +goose Up
-- The institution and reporting teacher a Teacher last typed for an OPR prompt (spec #578), kept on their Teacher profile to
-- prefill the next form. The rest of an OPR is for one report and is not kept. Empty means none yet.
ALTER TABLE teacher_profiles
	ADD COLUMN last_institution text NOT NULL DEFAULT '',
	ADD COLUMN last_reporter text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE teacher_profiles DROP COLUMN last_institution, DROP COLUMN last_reporter;
