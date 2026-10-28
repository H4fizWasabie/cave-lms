-- +goose Up
-- A Course's optional picture, stored like a Programme's: picture_key names it in the same storage as Gallery files,
-- empty when there is none, and the Course card draws a fallback instead.
ALTER TABLE courses ADD COLUMN picture_key text NOT NULL DEFAULT '', ADD COLUMN picture_type text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE courses DROP COLUMN picture_key, DROP COLUMN picture_type;
