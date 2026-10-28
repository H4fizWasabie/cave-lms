-- +goose Up
-- Learner search over Courses and Lessons (spec #321). The simple configuration does no stemming and needs no Malay
-- dictionary, so English and Malay words match as typed. The columns are generated, so they never drift from the text.
-- A Lesson's body has its own column: only a Learner enrolled in the Course may match on it, since a word found in a
-- body the searcher cannot open would leak what the body says.
ALTER TABLE courses ADD COLUMN search tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || description)) STORED;
ALTER TABLE lessons ADD COLUMN search tsvector GENERATED ALWAYS AS (to_tsvector('simple', title || ' ' || objective)) STORED;
ALTER TABLE lessons ADD COLUMN body_search tsvector GENERATED ALWAYS AS (to_tsvector('simple', body)) STORED;
CREATE INDEX courses_search ON courses USING gin (search);
CREATE INDEX lessons_search ON lessons USING gin (search);
CREATE INDEX lessons_body_search ON lessons USING gin (body_search);

-- +goose Down
DROP INDEX lessons_body_search;
DROP INDEX lessons_search;
DROP INDEX courses_search;
ALTER TABLE lessons DROP COLUMN body_search;
ALTER TABLE lessons DROP COLUMN search;
ALTER TABLE courses DROP COLUMN search;
