-- +goose Up
-- The emails of a released Result and of a Certificate issued with it go out in the background, after the release. These
-- flags record, in the statement that releases the Result or issues the Certificate, which of them still owe their
-- email, so a restart cannot lose one: the sender clears a flag once it has tried the send. Every row that exists now
-- counts as emailed.
ALTER TABLE exam_attempts ADD COLUMN result_email_owed boolean NOT NULL DEFAULT false;
ALTER TABLE certificates ADD COLUMN email_owed boolean NOT NULL DEFAULT false;
CREATE INDEX exam_attempts_result_email_owed ON exam_attempts (id) WHERE result_email_owed;

-- +goose Down
DROP INDEX exam_attempts_result_email_owed;
ALTER TABLE certificates DROP COLUMN email_owed;
ALTER TABLE exam_attempts DROP COLUMN result_email_owed;
