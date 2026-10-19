-- +goose Up
-- A Course with Enrollments cannot be deleted: the Admin closes it instead.
CREATE TABLE enrollments (
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    course_id bigint NOT NULL REFERENCES courses ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (learner_id, course_id)
);

-- +goose Down
DROP TABLE enrollments;
