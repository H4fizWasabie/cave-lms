-- +goose Up
-- The Storage allowance: how many MB of Dataset data a Learner may keep while in a period of the Plan. A period
-- copies it from its Plan when assigned, as it copies the Deployment allowance. Existing Plans and periods get 1 GB.
ALTER TABLE plans ADD COLUMN storage_mb integer NOT NULL DEFAULT 1024;
ALTER TABLE plan_periods ADD COLUMN storage_mb integer NOT NULL DEFAULT 1024;

-- +goose Down
ALTER TABLE plans DROP COLUMN storage_mb;
ALTER TABLE plan_periods DROP COLUMN storage_mb;
