-- +goose Up
-- The Certificate PDF is printed on one background image, the client's artwork, which carries the organisation,
-- logos and signatures, so the separate logo, signature and names go. Their stored files are left behind.
ALTER TABLE certificate_settings ADD COLUMN background_key text,
    DROP COLUMN logo_key, DROP COLUMN signature_key, DROP COLUMN organisation_name, DROP COLUMN signatory_name;

-- +goose Down
ALTER TABLE certificate_settings DROP COLUMN background_key,
    ADD COLUMN organisation_name text NOT NULL DEFAULT '', ADD COLUMN signatory_name text NOT NULL DEFAULT '',
    ADD COLUMN logo_key text, ADD COLUMN signature_key text;
