-- +goose Up
-- Ebook access (ADR 0020): a Pro Plan Promo may come with Ebooks, and a year of CaveText Pro bought with its code, or
-- the COMBO Pro Plan given by hand, carries them. CAVE99, running at launch, is the first such Promo.
ALTER TABLE promos ADD COLUMN ebooks boolean NOT NULL DEFAULT false;
UPDATE promos SET ebooks = true WHERE upper(code) = 'CAVE99' AND product = 'pro-plan';
ALTER TABLE cavetext_pro_periods ADD COLUMN ebooks boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE cavetext_pro_periods DROP COLUMN ebooks;
ALTER TABLE promos DROP COLUMN ebooks;
