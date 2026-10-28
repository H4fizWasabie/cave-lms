-- +goose Up
CREATE TABLE courses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL,
    description text NOT NULL,
    start_date date,
    is_open boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE courses;
