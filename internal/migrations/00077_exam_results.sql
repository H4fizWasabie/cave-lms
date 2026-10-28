-- +goose Up
-- A Result is released 10 minutes after its attempt is submitted, and only then saved: before release an attempt has
-- answers but no mark or pass or fail. Results so far were shown at once, so they count as released when submitted.
-- An attempt left open past its time limit (and its 30-second grace) was never marked; it is now submitted at its
-- time limit with no answers, so it counts as a released 0% fail, without an email about an old attempt.
ALTER TABLE exam_attempts ADD COLUMN released_at timestamptz;
ALTER TABLE exam_attempts DROP CONSTRAINT exam_attempts_check;
UPDATE exam_attempts SET released_at = finished_at WHERE finished_at IS NOT NULL;
UPDATE exam_attempts SET finished_at = started_at + make_interval(mins => time_limit_minutes),
    released_at = started_at + make_interval(mins => time_limit_minutes), score = 0, passed = false
    WHERE finished_at IS NULL AND now() > started_at + make_interval(mins => time_limit_minutes, secs => 30);
ALTER TABLE exam_attempts
    ADD CHECK (released_at IS NULL OR finished_at IS NOT NULL),
    ADD CHECK ((released_at IS NULL) = (score IS NULL) AND (score IS NULL) = (passed IS NULL));
-- The release job looks for these every minute.
CREATE INDEX exam_attempts_unreleased ON exam_attempts (id) WHERE released_at IS NULL;

-- +goose Down
-- A submitted attempt not yet released has no mark, so it goes back to being open.
UPDATE exam_attempts SET finished_at = NULL WHERE released_at IS NULL;
ALTER TABLE exam_attempts DROP COLUMN released_at;
ALTER TABLE exam_attempts ADD CHECK ((finished_at IS NULL) = (score IS NULL) AND (score IS NULL) = (passed IS NULL));
