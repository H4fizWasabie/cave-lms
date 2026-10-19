-- +goose Up
-- At most one Exam per Course: course_id is the primary key. Deleting the Course deletes its Exam.
CREATE TABLE exams (
    course_id bigint PRIMARY KEY REFERENCES courses ON DELETE CASCADE,
    pass_mark integer NOT NULL DEFAULT 50 CHECK (pass_mark BETWEEN 0 AND 100),
    time_limit_minutes integer CHECK (time_limit_minutes > 0),
    max_attempts integer CHECK (max_attempts > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Drafts are never asked. An Exam is usable only once it has a published question.
CREATE TABLE exam_questions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    course_id bigint NOT NULL REFERENCES exams ON DELETE CASCADE,
    position integer NOT NULL,
    prompt text NOT NULL,
    is_draft boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX exam_questions_course_position ON exam_questions (course_id, position);

-- position is the option's slot on the form (0 to 4); slots left blank have no row.
CREATE TABLE exam_options (
    question_id bigint NOT NULL REFERENCES exam_questions ON DELETE CASCADE,
    position integer NOT NULL CHECK (position BETWEEN 0 AND 4),
    text text NOT NULL,
    is_correct boolean NOT NULL,
    PRIMARY KEY (question_id, position)
);
CREATE UNIQUE INDEX exam_options_one_correct ON exam_options (question_id) WHERE is_correct;

-- +goose Down
DROP TABLE exam_options;
DROP TABLE exam_questions;
DROP TABLE exams;
