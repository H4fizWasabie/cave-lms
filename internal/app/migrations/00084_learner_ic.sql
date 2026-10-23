-- +goose Up
-- A Learner's IC or passport number, with spaces and dashes removed. Set once by the Learner; the Admin corrects it.
-- Unique across Learners, so one person's Certificate search never shows another account's Certificates.
ALTER TABLE learners ADD COLUMN ic text;
CREATE UNIQUE INDEX learners_ic_key ON learners (ic);

-- +goose Down
ALTER TABLE learners DROP COLUMN ic;
