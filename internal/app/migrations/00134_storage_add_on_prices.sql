-- +goose Up
-- Storage add-ons (3 GB and 5 GB) get a price row each, so the Admin can price them on the Prices page. Neither is sold
-- yet: a price of 0 means no price set, and they have no Normal price or payment link.
ALTER TABLE prices DROP CONSTRAINT prices_product_check;
ALTER TABLE prices ADD CONSTRAINT prices_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan', 'storage-3gb', 'storage-5gb'));
INSERT INTO prices (product, price_sen, normal_sen, payment_link) VALUES
	('storage-3gb', 0, NULL, ''),
	('storage-5gb', 0, NULL, '');

-- +goose Down
DELETE FROM prices WHERE product IN ('storage-3gb', 'storage-5gb');
ALTER TABLE prices DROP CONSTRAINT prices_product_check;
ALTER TABLE prices ADD CONSTRAINT prices_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan'));
