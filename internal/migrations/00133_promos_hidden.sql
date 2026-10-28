-- +goose Up
-- A Hidden Promo is accepted as a payment and gives Ebooks like any Promo, but is never shown: not in a price, on the
-- COMBO Pro Plan card or in a locked ebook's pointer. An Admin uses one to check payments.
ALTER TABLE promos ADD COLUMN hidden boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE promos DROP COLUMN hidden;
