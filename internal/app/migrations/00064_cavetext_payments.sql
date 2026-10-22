-- +goose Up
-- Pro payments: each paid order on the CaveText Pro payment form, as BCL's webhook reports it. The order number is
-- kept once, so a webhook sent twice gives one year. granted is set when the order gave a Learner Pro; one that
-- named no Learner, or paid less than the price, waits on the Admin CaveText Learners page until an Admin marks it
-- done. Name and emails are kept so the Admin can find who paid.
CREATE TABLE cavetext_payments (
	order_number text PRIMARY KEY,
	received_at timestamptz NOT NULL DEFAULT now(),
	payer_name text NOT NULL,
	payer_email text NOT NULL,
	cavelms_email text NOT NULL,
	subtotal_sen integer NOT NULL,
	learner_id bigint REFERENCES learners ON DELETE SET NULL,
	granted boolean NOT NULL DEFAULT false,
	done_at timestamptz
);
CREATE INDEX cavetext_payments_learner ON cavetext_payments (learner_id);

-- +goose Down
DROP TABLE cavetext_payments;
