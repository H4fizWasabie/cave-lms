-- +goose Up
-- A Lesson's optional Lesson image, stored like a Course picture: image_key names it in the same storage as Gallery
-- files, empty when there is none. A Lesson has at most one of a Gallery item and a Lesson image, and a Lesson image
-- comes with text and a description.
ALTER TABLE lessons ADD COLUMN image_key text NOT NULL DEFAULT '',
    ADD COLUMN image_type text NOT NULL DEFAULT '',
    ADD COLUMN image_description text NOT NULL DEFAULT '',
    ADD CONSTRAINT lessons_one_media CHECK (gallery_item_id IS NULL OR image_key = ''),
    ADD CONSTRAINT lessons_image_has_text CHECK (image_key = '' OR (body <> '' AND image_description <> ''));

-- +goose Down
ALTER TABLE lessons DROP CONSTRAINT lessons_image_has_text, DROP CONSTRAINT lessons_one_media,
    DROP COLUMN image_description, DROP COLUMN image_type, DROP COLUMN image_key;
