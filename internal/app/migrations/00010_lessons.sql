-- +goose Up
-- A Course with Lessons cannot be deleted, and a Gallery item used by a Lesson cannot be deleted.
CREATE TABLE lessons (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    course_id bigint NOT NULL REFERENCES courses ON DELETE RESTRICT,
    position integer NOT NULL,
    title text NOT NULL,
    objective text NOT NULL DEFAULT '',
    -- Markdown. A Lesson has text, a Gallery item, or both (00121 replaced the check below, which once allowed only one).
    body text NOT NULL DEFAULT '',
    gallery_item_id bigint REFERENCES gallery_items ON DELETE RESTRICT,
    published boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((gallery_item_id IS NULL) = (body <> ''))
);
CREATE INDEX lessons_course_position ON lessons (course_id, position);
CREATE INDEX lessons_gallery_item ON lessons (gallery_item_id);

-- +goose Down
DROP TABLE lessons;
