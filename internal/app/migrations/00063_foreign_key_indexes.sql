-- +goose Up
-- Indexes for the foreign keys that had none, so lookups and cascading deletes by them don't read the whole table.
-- certificates_one_valid leads with learner_id but is partial, so it can't serve a plain lookup by Learner.
CREATE INDEX sessions_learner ON sessions (learner_id);
CREATE INDEX one_time_tokens_learner ON one_time_tokens (learner_id);
CREATE INDEX deployments_learner ON deployments (learner_id);
CREATE INDEX enrollments_course ON enrollments (course_id);
CREATE INDEX lesson_completions_lesson ON lesson_completions (lesson_id);
CREATE INDEX exam_attempts_course ON exam_attempts (course_id);
CREATE INDEX certificates_course ON certificates (course_id);
CREATE INDEX certificates_learner ON certificates (learner_id);
CREATE INDEX plan_periods_plan ON plan_periods (plan_id);

-- +goose Down
DROP INDEX sessions_learner;
DROP INDEX one_time_tokens_learner;
DROP INDEX deployments_learner;
DROP INDEX enrollments_course;
DROP INDEX lesson_completions_lesson;
DROP INDEX exam_attempts_course;
DROP INDEX certificates_course;
DROP INDEX certificates_learner;
DROP INDEX plan_periods_plan;
