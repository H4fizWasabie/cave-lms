-- +goose Up
CREATE TABLE gallery_items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    category text NOT NULL DEFAULT '',
    format text NOT NULL CHECK (format IN ('video', 'ebook', 'picture')),
    promotional boolean NOT NULL DEFAULT false,
    visibility text NOT NULL CHECK (visibility IN ('everyone', 'learners', 'course_members')),
    -- Name of the file in the storage directory; never a public path.
    storage_key text NOT NULL UNIQUE,
    mime_type text NOT NULL,
    size_bytes bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE gallery_items;
