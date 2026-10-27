-- +goose Up
-- Built Dashboards are back (ADR 0019): the Dashboards production converted to pages on 2026-10-27 (#375) still have
-- their engine settings, a config object, and the converted page as their only page. Deleting that page makes each a
-- built Dashboard again at the same link, in its Theme, with its Dataset; the trigger queues the page's file for
-- deletion from the store. One whose owner has since uploaded a page of their own has more than one and stays a page
-- Dashboard; a pasted one has no config and is left alone (#389).
DELETE FROM dashboard_pages pg USING dashboards d
WHERE d.id = pg.dashboard_id AND jsonb_typeof(d.config) = 'object'
    AND (SELECT count(*) FROM dashboard_pages one WHERE one.dashboard_id = d.id) = 1;

-- +goose Down
-- Nothing: a converted page cannot be rebuilt in SQL, and the conversion code is gone.
SELECT 1;
