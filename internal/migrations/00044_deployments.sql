-- +goose Up
-- A Deployment puts one Dashboard online at /d/<slug> until sleeps_at (7 days); after that, or once taken down,
-- the link shows an inactive page. Deleting the Dashboard clears dashboard_id but keeps the row, so the slug
-- answers "not found" and is never reused, and the Deployment still counts against period_id's allowance.
-- The rows go with the Learner through plan_periods.
CREATE TABLE deployments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dashboard_id bigint UNIQUE REFERENCES dashboards ON DELETE SET NULL,
    period_id bigint NOT NULL REFERENCES plan_periods ON DELETE CASCADE,
    slug text NOT NULL UNIQUE,
    deployed_at timestamptz NOT NULL DEFAULT now(),
    sleeps_at timestamptz NOT NULL,
    taken_down_at timestamptz,
    takedown_reason text NOT NULL DEFAULT ''
);
CREATE INDEX deployments_period ON deployments (period_id);

-- +goose Down
DROP TABLE deployments;
