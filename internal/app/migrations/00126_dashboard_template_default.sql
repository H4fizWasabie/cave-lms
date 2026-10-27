-- +goose Up
-- The Templates are gone (ADR 0019): nothing reads or writes dashboards.template or choices now, but the columns stay
-- until a later release, as the binary before this one still selects them if a deploy rolls back. A new Dashboard is
-- on General with no choices, as that binary reads it; choices already defaults to '{}'.
ALTER TABLE dashboards ALTER COLUMN template SET DEFAULT 'general';

-- +goose Down
ALTER TABLE dashboards ALTER COLUMN template DROP DEFAULT;
