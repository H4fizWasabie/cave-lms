-- +goose Up
-- The DSKP library of the Teachers Portal (ADR 0029). A DSKP is one subject, year and language whose standards the AI read from
-- a PDF that is never kept: only what was read is. Its state goes reading, then draft or failed; the Admin publishes a draft.
-- year is a Teacher profile's code (Y1 to Y6, F1 to F5), so a Teacher's years can pick their DSKPs first. owner_id is NULL
-- for the library and the Teacher's account for a DSKP only that Teacher sees. failure names why a read failed: restart,
-- unreadable, unavailable, over-limit or not-configured.
CREATE TABLE dskps (
	id bigserial PRIMARY KEY,
	subject text NOT NULL,
	year text NOT NULL,
	language text NOT NULL CHECK (language IN ('ms', 'en')),
	state text NOT NULL DEFAULT 'reading' CHECK (state IN ('reading', 'failed', 'draft', 'published')),
	failure text NOT NULL DEFAULT '',
	owner_id bigint REFERENCES learners ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX dskps_owner ON dskps (owner_id);

-- A Standard Kandungan, in the order the DSKP gives them: grouping is its tema or bidang and topic its tajuk.
CREATE TABLE dskp_sks (
	id bigserial PRIMARY KEY,
	dskp_id bigint NOT NULL REFERENCES dskps ON DELETE CASCADE,
	position int NOT NULL,
	grouping text NOT NULL,
	topic text NOT NULL,
	code text NOT NULL,
	text text NOT NULL
);
CREATE INDEX dskp_sks_dskp ON dskp_sks (dskp_id, position);

-- A Standard Pembelajaran of an SK, in order.
CREATE TABLE dskp_sps (
	id bigserial PRIMARY KEY,
	sk_id bigint NOT NULL REFERENCES dskp_sks ON DELETE CASCADE,
	position int NOT NULL,
	code text NOT NULL,
	text text NOT NULL
);
CREATE INDEX dskp_sps_sk ON dskp_sps (sk_id, position);

-- +goose Down
DROP TABLE dskp_sps;
DROP TABLE dskp_sks;
DROP TABLE dskps;
