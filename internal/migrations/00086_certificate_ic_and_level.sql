-- +goose Up
-- A Certificate copies the Learner's IC and the Course's days, which give its PCC Level, at issue, as it does the
-- name and title, so later changes never alter it. days is NULL for a self-paced Course; both are NULL on
-- Certificates from before this.
ALTER TABLE certificates ADD COLUMN learner_ic text, ADD COLUMN days integer;

-- +goose Down
ALTER TABLE certificates DROP COLUMN days, DROP COLUMN learner_ic;
