-- +goose Up
-- A new edition of a DSKP is a DSKP of its own that replaces_id points at the one it replaces: it is read and checked as a
-- draft while Teachers keep the old one, and publishing it removes the old one. The upload holds the old DSKP while it
-- checks that no other edition is being prepared (one that failed does not count), so there is at most one.
ALTER TABLE dskps ADD COLUMN replaces_id bigint REFERENCES dskps ON DELETE SET NULL;
CREATE INDEX dskps_replaces ON dskps (replaces_id);

-- +goose Down
DROP INDEX dskps_replaces;
ALTER TABLE dskps DROP COLUMN replaces_id;
