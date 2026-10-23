-- +goose Up
-- A Programme is an upcoming programme the Admin announces; it is not a Course and not a Gallery item.
-- picture_key names its optional picture in the same storage as Gallery files; empty when there is none.
CREATE TABLE programmes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL CHECK (title <> ''),
    starts_on date NOT NULL,
    blurb text NOT NULL DEFAULT '',
    link text NOT NULL DEFAULT '',
    picture_key text NOT NULL DEFAULT '',
    picture_type text NOT NULL DEFAULT ''
);
CREATE INDEX programmes_starts_on ON programmes (starts_on);

-- +goose Down
DROP TABLE programmes;
