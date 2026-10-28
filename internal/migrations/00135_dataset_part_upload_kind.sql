-- +goose Up
-- What an upload does to the Dataset once confirmed (ADR 0022) is a real column on its Part, not a field of the preview's
-- cleanup JSON: 'replace-data', 'add' or 'replace-part', and for a Replace of one Part, the Part it replaces. Only a
-- pending Part's kind matters; a current one keeps the default. Every pending Part takes its kind from its JSON.
ALTER TABLE dataset_parts
    ADD COLUMN upload_kind text NOT NULL DEFAULT 'replace-data' CHECK (upload_kind IN ('replace-data', 'add', 'replace-part')),
    ADD COLUMN replaces_part_id bigint,
    ADD CONSTRAINT dataset_parts_replaces_part CHECK ((upload_kind = 'replace-part') = (replaces_part_id IS NOT NULL));
UPDATE dataset_parts SET upload_kind = 'add' WHERE state = 'pending' AND cleanup ? 'added';
UPDATE dataset_parts SET upload_kind = 'replace-part', replaces_part_id = (cleanup->'replaces'->>'part')::bigint
    WHERE state = 'pending' AND cleanup ? 'replaces';

-- +goose Down
ALTER TABLE dataset_parts DROP CONSTRAINT dataset_parts_replaces_part, DROP COLUMN replaces_part_id, DROP COLUMN upload_kind;
