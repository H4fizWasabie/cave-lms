-- +goose Up
-- A Lesson has text, a Gallery item, or both (a video with notes under it), no longer one or the other.
ALTER TABLE lessons DROP CONSTRAINT lessons_check,
    ADD CONSTRAINT lessons_has_content CHECK (gallery_item_id IS NOT NULL OR body <> '');

-- +goose Down
ALTER TABLE lessons DROP CONSTRAINT lessons_has_content,
    ADD CONSTRAINT lessons_check CHECK ((gallery_item_id IS NULL) = (body <> ''));
