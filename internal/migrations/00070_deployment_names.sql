-- +goose Up
-- A Deployment's link carries a name its Learner picks (ADR 0007). Existing random slugs become names and keep working;
-- the row outlives its Dashboard, so a name is never given to another Deployment.
ALTER TABLE deployments RENAME COLUMN slug TO name;

-- +goose Down
ALTER TABLE deployments RENAME COLUMN name TO slug;
