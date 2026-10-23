# Staging runs beside production on the same VPS

The client needs somewhere to try new work, first the Written Exams of spec #140, before it goes live, so `staging.cavelms.com` runs a second copy of the app on the production VPS: its own systemd unit, port, system user and Postgres role and database (`cave_staging`), settings file and uploads directory, behind Caddy basic auth. A second VPS would cost money and drift from production; sharing the database or the service would let a staging mistake reach real Learners.

Staging is kept from acting on the real world by settings, not by a different build:

- It starts empty and is set up by hand. A copy of production would carry Learners' personal data and their CaveText Snippets, which no one reads (ADR 0006).
- `EMAIL_ALLOWLIST` limits whom it emails; every other message goes to the System log in full instead of out.
- `BCL_PAYMENTS=test` makes it count BCL Test Mode orders and ignore live ones. The TEST forms send their webhooks to staging, and production keeps ignoring Test Mode.
- Datasets stay on its disk, not in R2.
- It is not backed up.
- When AI marking lands it gets its own OpenRouter key with a low credit limit.

`CAVE_ENV=staging deploy/deploy.sh` ships any branch to staging and touches only staging's files; production deploys stay as they were.

Postgres's 100 connections are shared, but staging keeps the app's cap of 25: production, staging and the other apps together stay under 60.
