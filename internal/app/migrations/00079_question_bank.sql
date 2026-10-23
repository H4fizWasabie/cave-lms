-- +goose Up
-- The Question bank holds every Exam question site-wide. An Exam's questions are copies: editing or deleting one
-- side never changes the other. bank_question_id only records which bank question an Exam question came from, so
-- the same bank question is not added to one Exam twice. Topics match ignoring case.
CREATE TABLE bank_questions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    format text NOT NULL CHECK (format IN ('multiple-choice', 'written')),
    topic text NOT NULL,
    language text NOT NULL CHECK (language IN ('ms', 'en')),
    prompt text NOT NULL,
    is_draft boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bank_questions_topic ON bank_questions (lower(topic), format);

CREATE TABLE bank_options (
    question_id bigint NOT NULL REFERENCES bank_questions ON DELETE CASCADE,
    position integer NOT NULL CHECK (position BETWEEN 0 AND 4),
    text text NOT NULL,
    is_correct boolean NOT NULL,
    PRIMARY KEY (question_id, position)
);
CREATE UNIQUE INDEX bank_options_one_correct ON bank_options (question_id) WHERE is_correct;

-- Existing Exam questions are copied in, in Malay, with the Course title as their topic.
ALTER TABLE exam_questions ADD COLUMN bank_question_id bigint;
UPDATE exam_questions SET bank_question_id = nextval(pg_get_serial_sequence('bank_questions', 'id'));
INSERT INTO bank_questions (id, format, topic, language, prompt, is_draft, created_at) OVERRIDING SYSTEM VALUE
SELECT q.bank_question_id, x.format, c.title, 'ms', q.prompt, q.is_draft, q.created_at
FROM exam_questions q JOIN exams x ON x.id = q.exam_id JOIN courses c ON c.id = x.course_id;
INSERT INTO bank_options (question_id, position, text, is_correct)
SELECT q.bank_question_id, o.position, o.text, o.is_correct FROM exam_options o JOIN exam_questions q ON q.id = o.question_id;
ALTER TABLE exam_questions ADD FOREIGN KEY (bank_question_id) REFERENCES bank_questions ON DELETE SET NULL;
CREATE INDEX exam_questions_bank_question ON exam_questions (bank_question_id);

-- +goose Down
ALTER TABLE exam_questions DROP COLUMN bank_question_id;
DROP TABLE bank_options;
DROP TABLE bank_questions;
