-- +goose Up
-- An Assignment is a task in a Course that Learners answer by uploading files (spec #196). A Draft (published false)
-- is shown to the Admin only. Like Lessons, a Course with Assignments is not deleted.
CREATE TABLE assignments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    course_id bigint NOT NULL REFERENCES courses ON DELETE RESTRICT,
    position integer NOT NULL,
    title text NOT NULL CHECK (title <> ''),
    instructions text NOT NULL,
    published boolean NOT NULL DEFAULT false
);
CREATE INDEX assignments_course ON assignments (course_id, position);

-- +goose Down
DROP TABLE assignments;
