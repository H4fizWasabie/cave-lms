-- +goose Up
-- A Plan is what the Admin sells: Lab access for a number of days, with a Deployment allowance per period.
CREATE TABLE plans (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (name <> ''),
    days integer NOT NULL CHECK (days > 0),
    deployments integer NOT NULL CHECK (deployments >= 0),
    payment_link text NOT NULL DEFAULT ''
);

-- A Plan period is one assignment of a Plan to a Learner. deployments is copied from the Plan
-- at assignment, so editing the Plan never changes it. A period is active from starts_at until
-- ends_at, or until ended_early_at when the Admin ended it early.
CREATE TABLE plan_periods (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    plan_id bigint NOT NULL REFERENCES plans ON DELETE RESTRICT,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    deployments integer NOT NULL,
    ended_early_at timestamptz,
    CHECK (ends_at > starts_at)
);
CREATE INDEX plan_periods_learner ON plan_periods (learner_id, starts_at);

-- +goose Down
DROP TABLE plan_periods;
DROP TABLE plans;
