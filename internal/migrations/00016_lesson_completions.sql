-- +goose Up
-- Completions survive leaving a Course. Progress counts only the Lessons still published, so removing a Lesson
-- stops it counting, and deleting it removes its completions.
CREATE TABLE lesson_completions (
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    lesson_id bigint NOT NULL REFERENCES lessons ON DELETE CASCADE,
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (learner_id, lesson_id)
);

-- +goose Down
DROP TABLE lesson_completions;
