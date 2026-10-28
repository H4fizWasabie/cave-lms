# Production deploys pass staging and a smoke test first

CI proves the code, not the server it runs on: a missing setting, a migration on real data or a broken Caddy route only shows once deployed. So `deploy/deploy.sh` ends every deploy with a smoke test, `deploy/smoke.sh`, which requests a handful of public, signed-out pages over HTTPS and checks each answer. A production deploy also refuses a commit that has not passed that smoke test on staging (ADR 0010) first. If the smoke test fails on production, deploy puts the previous binary back. Staging is left broken, so the failure can be looked at.

A rollback swaps the binary and cannot undo a migration, so a production deploy takes a backup first whenever the commit adds a migration since the one production runs. After a migration `deploy.sh` therefore does not roll back a failed start or smoke test: the previous binary is likely broken against the migrated schema (on 2026-10-27 it ran for 2 minutes with `relation "datasets" does not exist`), so it leaves the new binary in place and the fix is forward or a restore of the pre-deploy snapshot.

Rejected: browser end-to-end tests, which cost far more to keep working than they would catch beyond CI's own tests; and checks behind a sign-in, which would need a standing test account on production.

`CAVE_SKIP_STAGING=1` lets an emergency fix skip staging.
