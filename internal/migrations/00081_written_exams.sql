-- +goose Up
-- A Written question has a model answer instead of options, in an Exam and in the Question bank; Multiple-choice
-- questions keep it empty. An attempt records whether an Admin set any of its Written marks.
ALTER TABLE exam_questions ADD COLUMN model_answer text NOT NULL DEFAULT '';
ALTER TABLE bank_questions ADD COLUMN model_answer text NOT NULL DEFAULT '';
ALTER TABLE exam_attempts ADD COLUMN admin_marked boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE exam_attempts DROP COLUMN admin_marked;
ALTER TABLE bank_questions DROP COLUMN model_answer;
ALTER TABLE exam_questions DROP COLUMN model_answer;
