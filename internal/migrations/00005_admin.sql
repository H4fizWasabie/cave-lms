-- +goose Up
-- Only the command-line step (CreateAdmin) sets this; registration never does.
ALTER TABLE learners ADD COLUMN is_admin boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE learners DROP COLUMN is_admin;
