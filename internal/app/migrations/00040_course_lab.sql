-- +goose Up
-- A Course that includes the Lab is one half of the Lab access rule; an active Plan period is the other.
ALTER TABLE courses ADD COLUMN includes_lab boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE courses DROP COLUMN includes_lab;
