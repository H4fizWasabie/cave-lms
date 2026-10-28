-- +goose Up
-- The Templates are gone (ADR 0019) and nothing has read dashboards.template or choices since the release that dropped them.
ALTER TABLE dashboards DROP COLUMN template, DROP COLUMN choices;

-- +goose Down
ALTER TABLE dashboards ADD COLUMN template text, ADD COLUMN choices jsonb;
