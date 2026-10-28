-- +goose Up
-- Each Payment records which product's payment form it was paid on, keyed as prices keys products, or NULL when the
-- form was no current payment link. Payments kept so far were all on the CaveText Pro form when they named a
-- Learner; one that named none may have come from any form, so it stays NULL.
ALTER TABLE cavetext_payments ADD COLUMN product text CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan'));
UPDATE cavetext_payments SET product = 'cavetext-pro' WHERE granted OR learner_id IS NOT NULL;

-- +goose Down
ALTER TABLE cavetext_payments DROP COLUMN product;
