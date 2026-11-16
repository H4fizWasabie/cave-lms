-- +goose Up
-- A Teacher profile (ADR 0029): the subjects and years an Account holder teaches, which the Teachers Portal uses to put their
-- DSKPs first. Subjects are the typed names joined by ", " and years are codes (Y1 to Y6, F1 to F5) joined by ",". The row's
-- first insert starts the 14-day Teacher trial, and a row is never made twice for an account, so a Teacher has one trial.
CREATE TABLE teacher_profiles (
	learner_id bigint PRIMARY KEY REFERENCES learners ON DELETE CASCADE,
	subjects text NOT NULL,
	years text NOT NULL,
	trial_started_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE teacher_profiles;
