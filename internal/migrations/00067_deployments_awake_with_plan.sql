-- +goose Up
-- A Deployment is awake while its Learner has an active Plan period (or is the Admin) and it is not taken down,
-- so it no longer has a fixed sleep time. The allowance caps how many of a Learner's Deployments are awake at
-- once, so a Deployment no longer counts against one period either.
ALTER TABLE deployments DROP COLUMN sleeps_at, DROP COLUMN period_id;

-- +goose Down
ALTER TABLE deployments ADD COLUMN sleeps_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN period_id bigint REFERENCES plan_periods ON DELETE CASCADE;
CREATE INDEX deployments_period ON deployments (period_id);
