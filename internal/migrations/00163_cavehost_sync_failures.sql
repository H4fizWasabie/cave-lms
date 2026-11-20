-- +goose Up
-- Sync failures of a CaveHost Deployment (spec #536, #542). failure_emailed is set once the Learner has been emailed about a
-- run of failing Syncs and cleared by the next Sync that works, so a run of failures sends one email. A sheet's reset is set by
-- Reset columns: its last good rows stay online, Sync leaves it alone, and the Learner confirms a new Clean-up preview, which
-- sets its columns and Recipe again and clears the flag.
ALTER TABLE cavehost_deployments ADD COLUMN failure_emailed boolean NOT NULL DEFAULT false;
ALTER TABLE cavehost_sheets ADD COLUMN reset boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE cavehost_sheets DROP COLUMN reset;
ALTER TABLE cavehost_deployments DROP COLUMN failure_emailed;
