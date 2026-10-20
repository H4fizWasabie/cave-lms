-- +goose Up
-- The Admin deploys without a Plan, so a Deployment names its owner itself and may have no period.
ALTER TABLE deployments ADD COLUMN learner_id bigint REFERENCES learners ON DELETE CASCADE;
UPDATE deployments dep SET learner_id = pp.learner_id FROM plan_periods pp WHERE pp.id = dep.period_id;
ALTER TABLE deployments ALTER COLUMN learner_id SET NOT NULL, ALTER COLUMN period_id DROP NOT NULL;

-- +goose Down
DELETE FROM deployments WHERE period_id IS NULL;
ALTER TABLE deployments ALTER COLUMN period_id SET NOT NULL, DROP COLUMN learner_id;
