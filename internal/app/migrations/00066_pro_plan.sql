-- +goose Up
-- The Pro Plan: the one Plan the Admin marks as the one CaveText Pro comes with. At most one Plan has the mark.
ALTER TABLE plans ADD COLUMN pro boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX plans_one_pro ON plans (pro) WHERE pro;

-- +goose Down
DROP INDEX plans_one_pro;
ALTER TABLE plans DROP COLUMN pro;
