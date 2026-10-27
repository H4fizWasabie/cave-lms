-- +goose Up
-- The Templates are gone (ADR 0019): a built Dashboard's blocks come from the General rules, and a Dashboard built on
-- another Template keeps the blocks it was given, which name its Dataset's columns, so it draws as before. Nothing
-- reads a Dashboard's Template, or its column choices for the Template's blanks, any more.
ALTER TABLE dashboards DROP COLUMN template, DROP COLUMN choices;

-- +goose Down
ALTER TABLE dashboards ADD COLUMN template text, ADD COLUMN choices jsonb;
