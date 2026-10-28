-- +goose Up
-- The Lab no longer depends on Courses: an active Plan period alone gives it (#106).
ALTER TABLE courses DROP COLUMN includes_lab;

-- +goose Down
ALTER TABLE courses ADD COLUMN includes_lab boolean NOT NULL DEFAULT false;
