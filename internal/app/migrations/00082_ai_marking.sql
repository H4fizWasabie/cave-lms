-- +goose Up
-- The AI marks Written answers after submit. An attempt records when the AI was last asked, so a failed mark is
-- retried every 5 minutes, and whether the System log has said it is still unmarked an hour after its release time.
ALTER TABLE exam_attempts ADD COLUMN marking_tried_at timestamptz,
    ADD COLUMN marking_late_logged boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE exam_attempts DROP COLUMN marking_late_logged, DROP COLUMN marking_tried_at;
