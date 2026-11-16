-- +goose Up
-- The DSKP uploads a Teacher has started (ADR 0029), one row each, kept when the Teacher deletes the DSKP they uploaded, so
-- the limit of 5 a calendar month, Malaysia time, counts uploads started and deleting one gives no upload back.
CREATE TABLE teacher_dskp_uploads (
	id bigserial PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
	started_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX teacher_dskp_uploads_learner ON teacher_dskp_uploads (learner_id, started_at);

-- +goose Down
DROP TABLE teacher_dskp_uploads;
