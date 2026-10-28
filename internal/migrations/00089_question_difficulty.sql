-- +goose Up
-- A question's Difficulty: what the Admin asked the AI for, or set by hand. Existing questions stay without one.
-- An Exam copy carries its bank question's Difficulty as it carries its prompt.
ALTER TABLE bank_questions ADD COLUMN difficulty text CHECK (difficulty IN ('easy', 'medium', 'hard'));
ALTER TABLE exam_questions ADD COLUMN difficulty text CHECK (difficulty IN ('easy', 'medium', 'hard'));

-- +goose Down
ALTER TABLE exam_questions DROP COLUMN difficulty;
ALTER TABLE bank_questions DROP COLUMN difficulty;
