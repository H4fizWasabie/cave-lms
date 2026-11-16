-- +goose Up
-- Teachers Portal Pro (ADR 0029) is sold by the month and by the year, so the Admin prices two more products and a Payment
-- may be kept on either one's form. The prices start at placeholders until the client approves them.
ALTER TABLE prices DROP CONSTRAINT prices_product_check;
ALTER TABLE prices ADD CONSTRAINT prices_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan', 'storage-3gb', 'storage-5gb', 'teachers-month', 'teachers-year'));
INSERT INTO prices (product, price_sen, normal_sen, payment_link) VALUES
	('teachers-month', 1500, NULL, ''),
	('teachers-year', 15000, NULL, '');
ALTER TABLE cavetext_payments DROP CONSTRAINT cavetext_payments_product_check;
ALTER TABLE cavetext_payments ADD CONSTRAINT cavetext_payments_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan', 'teachers-month', 'teachers-year'));

-- A Teachers Portal Pro period is a month or a year an account may use the portal, from a Payment or given by the Admin.
-- Each starts when the account's Teacher trial or last period ends, so renewing early loses no days. It is on the account
-- (learners), not on a CaveText account: a Teacher may have none. Apart from CaveText Pro periods and Plan periods.
CREATE TABLE teacher_pro_periods (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
	product text NOT NULL CHECK (product IN ('teachers-month', 'teachers-year')),
	given_by text NOT NULL CHECK (given_by IN ('payment', 'admin')),
	starts_at timestamptz NOT NULL,
	ends_at timestamptz NOT NULL
);
CREATE INDEX teacher_pro_periods_learner ON teacher_pro_periods (learner_id);

-- +goose Down
DROP TABLE teacher_pro_periods;
DELETE FROM promos WHERE product IN ('teachers-month', 'teachers-year');
DELETE FROM cavetext_payments WHERE product IN ('teachers-month', 'teachers-year');
DELETE FROM prices WHERE product IN ('teachers-month', 'teachers-year');
ALTER TABLE cavetext_payments DROP CONSTRAINT cavetext_payments_product_check;
ALTER TABLE cavetext_payments ADD CONSTRAINT cavetext_payments_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan'));
ALTER TABLE prices DROP CONSTRAINT prices_product_check;
ALTER TABLE prices ADD CONSTRAINT prices_product_check
	CHECK (product IN ('cavetext-pro', 'lab-plan', 'pro-plan', 'storage-3gb', 'storage-5gb'));
