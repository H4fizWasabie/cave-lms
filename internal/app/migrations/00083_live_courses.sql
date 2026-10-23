-- +goose Up
-- A Live Course is a Course with an end date and a number of days (1 to 5), set together, and a start date (ADR 0012).
-- Its PCC Level follows from its days and is never stored. A Course the Admin duplicates starts as a Draft, hidden from
-- Learners until published; every Course so far is published.
ALTER TABLE courses
    ADD COLUMN end_date date,
    ADD COLUMN days integer CHECK (days BETWEEN 1 AND 5),
    ADD COLUMN published boolean NOT NULL DEFAULT true,
    ADD CHECK ((days IS NULL) = (end_date IS NULL)),
    ADD CHECK (end_date IS NULL OR (start_date IS NOT NULL AND end_date >= start_date));

-- +goose Down
ALTER TABLE courses DROP COLUMN published, DROP COLUMN days, DROP COLUMN end_date;
