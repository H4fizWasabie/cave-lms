-- +goose Up
-- The lesson details a Teacher last typed for an RPH prompt (ADR 0029), kept on their Teacher profile to prefill the next
-- form: the time or duration, the class and the number of murid. Notes and the date are for one lesson and are not kept.
-- Empty and 0 mean none yet.
ALTER TABLE teacher_profiles
	ADD COLUMN last_time text NOT NULL DEFAULT '',
	ADD COLUMN last_class text NOT NULL DEFAULT '',
	ADD COLUMN last_students integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE teacher_profiles DROP COLUMN last_time, DROP COLUMN last_class, DROP COLUMN last_students;
