-- +goose Up
-- Renewal reminders (ADR 0026) also cover the Teacher trial ('teachers-trial', reminded 3 days before it ends and once it
-- has ended) and Teachers Portal Pro ('teachers-pro': a year 30 and 7 days before and once ended, a month 7 days before
-- and once ended).
ALTER TABLE renewal_reminders DROP CONSTRAINT renewal_reminders_item_check;
ALTER TABLE renewal_reminders ADD CONSTRAINT renewal_reminders_item_check
	CHECK (item IN ('lab-plan', 'cavetext-pro', 'lab-data', 'teachers-trial', 'teachers-pro'));
ALTER TABLE renewal_reminders DROP CONSTRAINT renewal_reminders_stage_check;
ALTER TABLE renewal_reminders ADD CONSTRAINT renewal_reminders_stage_check
	CHECK (stage IN ('30-days', '7-days', '3-days', 'ended', 'deletion'));

-- +goose Down
DELETE FROM renewal_reminders WHERE item IN ('teachers-trial', 'teachers-pro');
ALTER TABLE renewal_reminders DROP CONSTRAINT renewal_reminders_stage_check;
ALTER TABLE renewal_reminders ADD CONSTRAINT renewal_reminders_stage_check
	CHECK (stage IN ('30-days', '7-days', 'ended', 'deletion'));
ALTER TABLE renewal_reminders DROP CONSTRAINT renewal_reminders_item_check;
ALTER TABLE renewal_reminders ADD CONSTRAINT renewal_reminders_item_check
	CHECK (item IN ('lab-plan', 'cavetext-pro', 'lab-data'));
