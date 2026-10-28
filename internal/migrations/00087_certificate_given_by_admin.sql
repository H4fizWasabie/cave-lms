-- +goose Up
-- A Certificate the Admin gave by hand, skipping the Exam check, is marked so the Admin Certificates list can tell
-- it apart. Learners, the check page and the PDF never show it.
ALTER TABLE certificates ADD COLUMN given_by_admin boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE certificates DROP COLUMN given_by_admin;
