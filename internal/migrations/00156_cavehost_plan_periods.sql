-- +goose Up
-- A CaveHost Plan period is access to CaveHost for a Learner (spec #536), kept apart from Plan periods: it has no Plan,
-- price, Deployment allowance or Storage allowance of its own. It runs from starts_at until ends_at, or until
-- ended_early_at when the Admin ended it early. The columns match plan_periods so activePeriodSQL fits both.
CREATE TABLE cavehost_plan_periods (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    ended_early_at timestamptz,
    CHECK (ends_at > starts_at)
);
CREATE INDEX cavehost_plan_periods_learner ON cavehost_plan_periods (learner_id, starts_at);

-- +goose Down
DROP TABLE cavehost_plan_periods;
