-- +goose Up
-- Learner search over Courses and Lessons (spec #321). The simple configuration does no stemming and needs no Malay
-- dictionary, so English and Malay words match as typed. The columns are generated, so they never drift from the text.
ALTER TABLE courses ADD COLUMN search tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || description)) STORED;
ALTER TABLE lessons ADD COLUMN search tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || objective || ' ' || body)) STORED;
CREATE INDEX courses_search ON courses USING gin (search);
CREATE INDEX lessons_search ON lessons USING gin (search);

-- +goose Down
DROP INDEX lessons_search;
DROP INDEX courses_search;
ALTER TABLE lessons DROP COLUMN search;
ALTER TABLE courses DROP COLUMN search;
