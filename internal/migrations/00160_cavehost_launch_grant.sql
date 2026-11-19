-- +goose Up
-- CaveHost comes with every Pro Plan (ADR 0030). Learners holding a Pro Plan at launch get a CaveHost Plan period ending
-- when their current Lab Plan period ends. Holding one is told by a granted Pro Plan Payment, a year of CaveText Pro still
-- running and a Lab Plan period running now: CaveText Pro and the Lab Plan bought apart leave no Pro Plan Payment. A Pro
-- Plan the Admin gave by hand leaves no record, so it is not found here; the Admin assigns that Learner CaveHost. The
-- period starts when any CaveHost Plan period the Learner already has ends, so running this again adds nothing.
INSERT INTO cavehost_plan_periods (learner_id, starts_at, ends_at)
SELECT l.learner_id, s.at, l.ends_at
FROM (
	SELECT pp.learner_id, pp.ends_at
	FROM plan_periods pp JOIN plans p ON p.id = pp.plan_id AND p.lab
	WHERE pp.starts_at <= now() AND pp.ends_at > now() AND pp.ended_early_at IS NULL
	  AND EXISTS (SELECT 1 FROM cavetext_payments pay WHERE pay.learner_id = pp.learner_id AND pay.product = 'pro-plan' AND pay.granted)
	  AND EXISTS (SELECT 1 FROM cavetext_pro_periods cp WHERE cp.learner_id = pp.learner_id
	              AND cp.starts_at <= now() AND cp.ends_at > now() AND cp.ended_early_at IS NULL)
) l,
LATERAL (SELECT GREATEST(now(), (
	SELECT max(LEAST(ends_at, COALESCE(ended_early_at, ends_at))) FROM cavehost_plan_periods WHERE learner_id = l.learner_id
)) AS at) s
WHERE s.at < l.ends_at;

-- +goose Down
-- The periods it gave stay: they are CaveHost Plan periods like any other.
SELECT 1;
