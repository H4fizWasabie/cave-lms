-- +goose Up
-- theme is how a Dashboard looks, the key of one of lab.Themes; every Dashboard starts in Corporate Executive.
ALTER TABLE dashboards ADD COLUMN theme text NOT NULL DEFAULT 'corporate';

-- +goose Down
ALTER TABLE dashboards DROP COLUMN theme;
