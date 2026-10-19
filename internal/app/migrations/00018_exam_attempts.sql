-- +goose Up
-- An attempt keeps its own copy of the Exam as it stood when started: the time limit and pass mark, and the
-- published questions in the order shown, each with its options, correct option and the Learner's answer (JSON).
-- Later Exam edits never change a past attempt. Deleting the Exam deletes its attempts.
CREATE TABLE exam_attempts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    course_id bigint NOT NULL REFERENCES exams ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    time_limit_minutes integer CHECK (time_limit_minutes > 0),
    pass_mark integer NOT NULL CHECK (pass_mark BETWEEN 0 AND 100),
    questions jsonb NOT NULL,
    finished_at timestamptz,
    score integer CHECK (score BETWEEN 0 AND 100),
    passed boolean,
    CHECK ((finished_at IS NULL) = (score IS NULL) AND (score IS NULL) = (passed IS NULL))
);
CREATE INDEX exam_attempts_learner_course ON exam_attempts (learner_id, course_id);

-- +goose Down
DROP TABLE exam_attempts;
