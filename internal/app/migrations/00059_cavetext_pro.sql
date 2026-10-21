-- +goose Up
-- CaveText Pro periods: an Admin gives a Learner 12 months of Pro. Pro given while a period runs starts when that one
-- ends. Apart from Plans and Plan periods: they share no tables and no allowance.
CREATE TABLE cavetext_pro_periods (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	learner_id bigint NOT NULL REFERENCES cavetext_accounts ON DELETE CASCADE,
	starts_at timestamptz NOT NULL,
	ends_at timestamptz NOT NULL,
	ended_early_at timestamptz
);
CREATE INDEX cavetext_pro_periods_learner ON cavetext_pro_periods (learner_id);

-- Pro's price in sen and its payment link (empty: Learners are told to pay the Admin). Any Admin edits these.
ALTER TABLE cavetext_settings
	ADD COLUMN pro_price_sen integer NOT NULL DEFAULT 2500 CHECK (pro_price_sen >= 0),
	ADD COLUMN pro_payment_link text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE cavetext_settings DROP COLUMN pro_payment_link, DROP COLUMN pro_price_sen;
DROP TABLE cavetext_pro_periods;
