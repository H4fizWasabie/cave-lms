-- +goose Up
-- pg_trgm is a trusted extension, so the cave role creates it as the database's owner. The Admin's searches by part
-- of a name or email match with LIKE (and, for Certificates, trigram similarity); these indexes serve both.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX certificates_learner_name_trgm ON certificates USING gin (lower(learner_name) gin_trgm_ops);
CREATE INDEX learners_name_trgm ON learners USING gin (lower(name) gin_trgm_ops);
CREATE INDEX learners_email_trgm ON learners USING gin (lower(email) gin_trgm_ops);

-- +goose Down
-- The extension stays: it belongs to the database, and dropping it would break any other index that uses it.
DROP INDEX learners_email_trgm;
DROP INDEX learners_name_trgm;
DROP INDEX certificates_learner_name_trgm;
