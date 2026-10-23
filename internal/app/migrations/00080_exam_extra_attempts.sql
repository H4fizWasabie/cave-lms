-- +goose Up
-- The Admin can give a Learner who has used every attempt at an Exam one more. extra counts those given; the Learner's
-- attempt limit on the Exam is the Exam's plus extra.
CREATE TABLE exam_extra_attempts (
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    exam_id bigint NOT NULL REFERENCES exams ON DELETE CASCADE,
    extra integer NOT NULL CHECK (extra >= 1),
    PRIMARY KEY (learner_id, exam_id)
);
CREATE INDEX exam_extra_attempts_exam ON exam_extra_attempts (exam_id);

-- +goose Down
DROP TABLE exam_extra_attempts;
