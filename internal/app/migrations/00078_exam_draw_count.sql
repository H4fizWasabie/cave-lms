-- +goose Up
-- An Exam's draw count is how many of its published questions a Learner's first attempt asks, chosen at random.
-- Existing Exams take the Multiple-choice default of 30.
ALTER TABLE exams ADD COLUMN draw_count integer NOT NULL DEFAULT 30 CHECK (draw_count >= 1);

-- +goose Down
ALTER TABLE exams DROP COLUMN draw_count;
