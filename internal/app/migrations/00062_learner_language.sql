-- +goose Up
-- The language a Learner chose with the header switch; NULL until they first switch.
ALTER TABLE learners ADD COLUMN language text CHECK (language IN ('ms', 'en'));

-- +goose Down
ALTER TABLE learners DROP COLUMN language;
