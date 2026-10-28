-- +goose Up
-- The storage key of the Organisation's logo when the Certificate was issued, which it prints top left; NULL for
-- none. A later logo change leaves it as issued.
ALTER TABLE certificates ADD COLUMN logo_key text;

-- +goose Down
ALTER TABLE certificates DROP COLUMN logo_key;
