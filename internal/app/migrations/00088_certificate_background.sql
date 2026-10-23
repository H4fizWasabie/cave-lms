-- +goose Up
-- The Certificate PDF is printed on one background image, the client's artwork, which carries the organisation,
-- logos and signatures. The old logo, signature and names are no longer used; their columns and files stay.
ALTER TABLE certificate_settings ADD COLUMN background_key text;

-- +goose Down
ALTER TABLE certificate_settings DROP COLUMN background_key;
