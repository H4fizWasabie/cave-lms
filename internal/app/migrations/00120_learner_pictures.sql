-- +goose Up
-- A Learner's optional profile picture (#345): picture_key names its 256px JPEG in the same storage as Gallery files,
-- empty when there is none, and their initials show instead.
ALTER TABLE learners ADD COLUMN picture_key text NOT NULL DEFAULT '';
-- The picture route looks a key up on every chat avatar it serves.
CREATE INDEX learners_picture_key ON learners (picture_key) WHERE picture_key <> '';

-- +goose Down
DROP INDEX learners_picture_key;
ALTER TABLE learners DROP COLUMN picture_key;
