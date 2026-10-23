-- +goose Up
-- An Exam has a format, and a Course has at most one Exam per format. Every Exam so far is Multiple-choice. Exams get
-- their own id, which their questions and attempts now name in place of the Course; their rows are kept as they are.
-- A new Exam allows 2 attempts unless the Admin changes it. Existing Exams keep their limit.
ALTER TABLE exams
    ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN format text NOT NULL DEFAULT 'multiple-choice' CHECK (format IN ('multiple-choice', 'written')),
    ALTER max_attempts SET DEFAULT 2;
ALTER TABLE exams ALTER format DROP DEFAULT;

ALTER TABLE exam_questions ADD COLUMN exam_id bigint;
UPDATE exam_questions q SET exam_id = x.id FROM exams x WHERE x.course_id = q.course_id;
ALTER TABLE exam_attempts ADD COLUMN exam_id bigint;
UPDATE exam_attempts t SET exam_id = x.id FROM exams x WHERE x.course_id = t.course_id;
-- Dropping course_id drops its foreign key to exams and its indexes too.
ALTER TABLE exam_questions DROP COLUMN course_id;
ALTER TABLE exam_attempts DROP COLUMN course_id;

ALTER TABLE exams DROP CONSTRAINT exams_pkey, ADD PRIMARY KEY (id), ADD UNIQUE (course_id, format);
ALTER TABLE exam_questions ALTER exam_id SET NOT NULL, ADD FOREIGN KEY (exam_id) REFERENCES exams ON DELETE CASCADE;
ALTER TABLE exam_attempts ALTER exam_id SET NOT NULL, ADD FOREIGN KEY (exam_id) REFERENCES exams ON DELETE RESTRICT;
CREATE INDEX exam_questions_exam_position ON exam_questions (exam_id, position);
CREATE INDEX exam_attempts_learner_exam ON exam_attempts (learner_id, exam_id);
CREATE INDEX exam_attempts_exam ON exam_attempts (exam_id);

-- +goose Down
-- Only works while each Course has at most one Exam.
ALTER TABLE exam_questions ADD COLUMN course_id bigint;
UPDATE exam_questions q SET course_id = x.course_id FROM exams x WHERE x.id = q.exam_id;
ALTER TABLE exam_attempts ADD COLUMN course_id bigint;
UPDATE exam_attempts t SET course_id = x.course_id FROM exams x WHERE x.id = t.exam_id;
ALTER TABLE exam_questions DROP COLUMN exam_id;
ALTER TABLE exam_attempts DROP COLUMN exam_id;

ALTER TABLE exams DROP CONSTRAINT exams_pkey, DROP COLUMN id, DROP COLUMN format, ADD PRIMARY KEY (course_id),
    ALTER max_attempts DROP DEFAULT;
ALTER TABLE exam_questions ALTER course_id SET NOT NULL, ADD FOREIGN KEY (course_id) REFERENCES exams ON DELETE CASCADE;
ALTER TABLE exam_attempts ALTER course_id SET NOT NULL, ADD FOREIGN KEY (course_id) REFERENCES exams ON DELETE RESTRICT;
CREATE INDEX exam_questions_course_position ON exam_questions (course_id, position);
CREATE INDEX exam_attempts_learner_course ON exam_attempts (learner_id, course_id);
CREATE INDEX exam_attempts_course ON exam_attempts (course_id);
