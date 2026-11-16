-- +goose Up
-- Whether a Teacher last ticked "Use my school logo" on a prompt (the School logo), kept on their Teacher profile to prefill
-- both the RPH prompt form and the OPR prompt form: one setting for both kinds. The picture is never held. False means not ticked.
ALTER TABLE teacher_profiles ADD COLUMN last_logo boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE teacher_profiles DROP COLUMN last_logo;
