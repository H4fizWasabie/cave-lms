-- +goose Up
-- Prices: the three products the Admin sells, each with a price in sen, an optional Normal price shown crossed out
-- beside it, and its payment link (empty: Learners are told to contact us). Any Admin edits them. CaveText Pro keeps
-- today's price and link; the Lab Plan and the Pro Plan start at the client's prices with no link.
CREATE TABLE prices (
	product text PRIMARY KEY CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan')),
	price_sen integer NOT NULL CHECK (price_sen >= 0),
	normal_sen integer CHECK (normal_sen > price_sen),
	payment_link text NOT NULL DEFAULT ''
);
INSERT INTO prices (product, price_sen, normal_sen, payment_link) VALUES
	('cavetext-pro', COALESCE((SELECT pro_price_sen FROM cavetext_settings WHERE id = 1), 3500), NULL,
		COALESCE((SELECT pro_payment_link FROM cavetext_settings WHERE id = 1), '')),
	('lab-plan', 8900, NULL, ''),
	('pro-plan', 9900, 11900, '');
ALTER TABLE cavetext_settings DROP COLUMN pro_price_sen, DROP COLUMN pro_payment_link;

-- Only the Lab Plan is sold, so Plans lose their own payment link, and the Pro Plan mark becomes the Lab Plan mark.
ALTER TABLE plans DROP COLUMN payment_link;
ALTER TABLE plans RENAME COLUMN pro TO lab;
ALTER INDEX plans_one_pro RENAME TO plans_one_lab;

-- +goose Down
ALTER INDEX plans_one_lab RENAME TO plans_one_pro;
ALTER TABLE plans RENAME COLUMN lab TO pro;
ALTER TABLE plans ADD COLUMN payment_link text NOT NULL DEFAULT '';
ALTER TABLE cavetext_settings
	ADD COLUMN pro_price_sen integer NOT NULL DEFAULT 2500 CHECK (pro_price_sen >= 0),
	ADD COLUMN pro_payment_link text NOT NULL DEFAULT '';
UPDATE cavetext_settings SET pro_price_sen = p.price_sen, pro_payment_link = p.payment_link
	FROM prices p WHERE p.product = 'cavetext-pro';
DROP TABLE prices;
