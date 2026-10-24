# Deploying Cave LMS

The app runs on the shared VPS (`ssh vps`, Ubuntu 24.04) as the `cave-lms` systemd service, listening on `127.0.0.1:8088`. The existing Caddy serves it over HTTPS at `cavelms.com` (`www.cavelms.com` and the old `lms.wasabietech.com` redirect to it) from `/etc/caddy/sites/cave-lms.caddy`. Other services share this VPS and this Caddy, so touch only the files named here.

| What | Where on the VPS |
|---|---|
| Binary | `/usr/local/bin/cave-lms` |
| Service | `/etc/systemd/system/cave-lms.service` (from `deploy/cave-lms.service`) |
| Caddy site | `/etc/caddy/sites/cave-lms.caddy` (from `deploy/cave-lms.caddy`) |
| Settings | `/etc/cave-lms/env`, root-owned, mode 0600 |
| Uploads | `/var/lib/cave-lms/uploads` |
| Database | `cave` on the local Postgres 16, peer auth over the Unix socket. Postgres allows 100 connections shared with the other apps on the VPS; the app holds at most 25 (`maxDBConns` in `internal/app/app.go`) |

Both the database and the uploads directory are backed up every night (see Backups).

## First install

Once per VPS. Run as root on the VPS.

1. Point A records for `cavelms.com` and `www.cavelms.com` at `169.58.63.110` (DNS is at NetKL) and wait until `dig +short cavelms.com` returns it. Caddy can't get a certificate before that.
2. Create the system user, database and settings:

   ```sh
   useradd --system --home /var/lib/cave-lms --shell /usr/sbin/nologin cave
   sudo -u postgres createuser cave
   sudo -u postgres createdb --owner cave cave
   install -d -m 0755 /etc/cave-lms
   install -m 0600 /dev/null /etc/cave-lms/env
   cat > /etc/cave-lms/env <<'EOF'
   DATABASE_URL=postgres:///cave?host=/var/run/postgresql
   PUBLIC_BASE_URL=https://cavelms.com
   EMAIL_FROM="Cave LMS <noreply@cavelms.com>"
   STORAGE_DIR=/var/lib/cave-lms/uploads
   LISTEN_ADDR=127.0.0.1:8088
   OWNER_EMAIL=hafizwasabie@gmail.com
   CAVETEXT_EXTENSION_IDS=omcdclpagggfcjlhpkohpcgdmfbelohd
   EOF
   ```

   Then add `RESEND_API_KEY=re_...` with an editor (`nano /etc/cave-lms/env`) so the key stays out of shell history. Use a Resend API key with "Sending access" only. Without it the app starts but sends no email (see Email below).

   For CaveText Pro payments, add `BCL_WEBHOOK_KEY` the same way, set to a random key of at least 32 characters (`openssl rand -hex 24`). BCL's webhook address is then `https://cavelms.com/cavetext/bcl/<key>`; set it on the payment form under Advanced > Webhook Settings, with the Payment Success event. Only live orders on the form that Admin > CaveText settings > Payment link points to give Pro; Test Mode orders are only logged. The Test Mode copies of the forms send to staging instead (see Staging). Without the key every webhook gets 404.

   BCL signs no webhook, so also add `BCL_API_TOKEN`: in BCL, Platform Setup > Integrations > Developer, create an API token with Read access, named for production. Each paid order is then read from BCL's order detail, `https://bcl.my/api/transactions/<order>` (live orders only), and gives nothing unless BCL has it successful (status 3), paid and live. Everything that decides the grant comes from that detail, not the webhook, whose body gives only the event and the order number: the payer email, the amounts (subtotal, discount, amount charged), the CAVELMS account email field and the form. The form is BCL's form id for the order (`formable_id`), matched to a product through `https://bcl.my/api/forms`, read on every order: the product is the one whose payment link (Admin > Prices) ends in the slug BCL lists with that id, so two forms with one title are told apart and renaming a form is safe. A webhook value that disagrees with BCL's (its `form_url`, payer email, amounts, or another CAVELMS account than BCL's order names) is noted in the order's System log line, as an error, without either CAVELMS email, and BCL's is used: a forged body racing BCL's own webhook gives what BCL's order bought to the account BCL's order names, and BCL's webhook after it is a repeat. An order paid on a form that is no product's, or that BCL charged under its subtotal less discount, may still be a real payment: it gives nothing, is answered 200, is kept under Payments not matched on Admin > CaveText Learners for an Admin to give by hand, and writes a System log error naming the order and why. An order BCL does not know yet (404) or has not marked successful, paid and live (any case), whose detail names no form, or BCL out of reach or failing (10 s timeout, or a wrong or revoked token), is answered 500 and logged, and BCL retries: a real payment is given once BCL confirms it. A forged order number keeps failing that way, so many such System log errors for one order, or every order failing (the token), need a look. Without the token, orders are not confirmed, the app says so at startup, and anyone who learns the webhook address can forge one. `BCL_API_URL` (default `https://bcl.my/api`) is for tests only.

   For Datasets in R2, add the four `R2_` settings the same way (see Datasets in R2 below). Without them Datasets are kept under `STORAGE_DIR`.

   For AI-written Exam questions, add the `OPENROUTER_` settings the same way (see AI writer below). Without them the Exam page says the AI writer is not configured.

3. From your machine, run `deploy/deploy.sh` (next section).
4. Create the Admin on the VPS. `systemd-run` runs it as `cave` with the service's own env file. `read -s` hides the password as you type, and passing it on stdin keeps it out of shell history and process lists:

   ```sh
   read -rsp 'Admin password: ' pw; echo
   printf '%s\n' "$pw" | systemd-run --quiet --pipe --wait --collect -p User=cave -p EnvironmentFile=/etc/cave-lms/env \
     /usr/local/bin/cave-lms create-admin --name "Abah" --email you@example.com
   unset pw
   ```

## Deploying an update

From a clean checkout of the commit to ship, staging first, then production (ADR 0011):

```sh
CAVE_ENV=staging deploy/deploy.sh
deploy/deploy.sh            # CAVE_HOST=other-host deploy/deploy.sh to target another SSH host
```

It refuses a checkout with uncommitted changes or a commit whose CI run (`.github/workflows/ci.yml`) has not passed, so `gh` must be signed in. A production deploy also refuses a commit that has not passed the smoke test on staging; for an emergency fix, `CAVE_SKIP_STAGING=1 deploy/deploy.sh` skips that check. When the commit adds a migration since the one production runs, or production's commit is not known, it runs the nightly backup first and waits for it. A merge makes a new commit, so after merging a pull request, deploy main to staging again before production.

Then it builds a Linux binary and copies the binary, unit, Caddy site and smoke test to the VPS. The Caddy site is replaced only if the whole Caddyfile still validates; otherwise the previous one is kept and the script fails before anything else changes. Then it swaps the binary, restarts `cave-lms` and waits for it to answer. Migrations run on start. Logs: `journalctl -u cave-lms -n 100`.

Last comes the smoke test, `deploy/smoke.sh` (installed as `cave-lms-smoke`, and as `cave-lms-staging-smoke` for staging): signed out, over the public HTTPS address, it requests the home page and its stylesheet, `/login`, `/courses`, `/gallery`, `/programmes`, `/plans` and `/certificates`, and checks that `/admin` redirects to `/login` and a wrong BCL key gets 404. If it fails on production, the previous binary goes back and the script fails; the binary is all a rollback restores, not the unit, the Caddy site or the database. Run it by hand with `ssh vps cave-lms-smoke production` or `ssh vps cave-lms-staging-smoke staging`. `/var/lib/cave-lms-deploys/` on the VPS holds the commit that last passed on staging and the one production runs.

## Staging

`staging.cavelms.com` is a second copy of the app on the same VPS, where the client tries new work before it goes live (`docs/adr/0010-staging-runs-beside-production.md`). It shares nothing with production but Postgres, Caddy and the Resend account, and it is not backed up.

| What | Where on the VPS |
|---|---|
| Binary | `/usr/local/bin/cave-lms-staging` |
| Service | `cave-lms-staging` (from `deploy/cave-lms-staging.service`), user `cave_staging`, `127.0.0.1:8089` |
| Caddy site | `/etc/caddy/sites/cave-lms-staging.caddy` (from `deploy/cave-lms-staging.caddy`) |
| Login | user `cave`; the hash in `/etc/caddy/cave-lms-staging-login`, the password in `/etc/cave-lms/staging-login-password` (root only), both kept off the public repo |
| Settings | `/etc/cave-lms/staging.env`, root-owned, mode 0600 |
| Uploads | `/var/lib/cave-lms-staging/uploads` (Datasets too: no R2) |
| Database | `cave_staging`, owned by the `cave_staging` role |

Deploy the current checkout, from any branch, once it is committed, pushed and CI has passed on it (open a pull request):

```sh
CAVE_ENV=staging deploy/deploy.sh
```

It ships only the staging binary, unit and Caddy site, and leaves production and the backups alone. A failed smoke test leaves staging as it is, to be looked at. Logs: `journalctl -u cave-lms-staging -n 100`.

### What keeps staging off the real world

- **Email** goes only to the addresses in `EMAIL_ALLOWLIST` (comma-separated; `@example.com` allows a whole domain). Any other message is not sent: it goes to the System log (and journald) in full, link included, so the Owner can read it at `staging.cavelms.com/admin/log`. Production leaves `EMAIL_ALLOWLIST` unset and emails everyone.
- **BCL**: `BCL_PAYMENTS=test` counts Test Mode orders as paid and ignores live ones. Each is confirmed with BCL's API when `BCL_API_TOKEN` is set, as on production, though without the form or CAVELMS email check (see step 2 below). Production leaves it unset (`live`), so it keeps ignoring Test Mode. The TEST forms send their webhooks to staging.
- **Data** starts empty. Set up Courses, Exams and prices by hand in staging's Admin.
- **CaveText** can't sign in to staging: `CAVETEXT_EXTENSION_IDS` is unset and the extension would trip over the login. A Test Mode payment still gives Pro, and the System log says so.

### First install of staging

Run as root on the VPS unless a step says otherwise.

1. At NetKL, add an A record `staging.cavelms.com` → `169.58.63.110`, then wait until `dig +short staging.cavelms.com` returns it.
2. Create the system user, database and settings:

   ```sh
   useradd --system --home /var/lib/cave-lms-staging --shell /usr/sbin/nologin cave_staging
   sudo -u postgres createuser cave_staging
   sudo -u postgres createdb --owner cave_staging cave_staging
   install -m 0600 /dev/null /etc/cave-lms/staging.env
   cat > /etc/cave-lms/staging.env <<'EOF'
   DATABASE_URL=postgres:///cave_staging?host=/var/run/postgresql
   PUBLIC_BASE_URL=https://staging.cavelms.com
   EMAIL_FROM="Cave LMS Staging <noreply@cavelms.com>"
   EMAIL_ALLOWLIST=hafizwasabie@gmail.com
   STORAGE_DIR=/var/lib/cave-lms-staging/uploads
   LISTEN_ADDR=127.0.0.1:8089
   OWNER_EMAIL=hafizwasabie@gmail.com
   BCL_PAYMENTS=test
   EOF
   ```

   Add the client's addresses to `EMAIL_ALLOWLIST`. Then, with an editor, add `RESEND_API_KEY` (a new Resend key with Sending access only, named for staging, so it can be revoked alone; it shares production's 100 emails a day), `BCL_WEBHOOK_KEY` (a new one, `openssl rand -hex 24`, not production's) and `BCL_API_TOKEN` (a new BCL API token with Read access, from Platform Setup > Integrations > Developer, named for staging so it can be revoked alone). With `BCL_PAYMENTS=test` orders are looked up at `https://bcl.my/api/transaction/<order>`, which also finds Test Mode orders; production's `/transactions/<order>` answers 404 for them. This lookup gives only the order's amount charged, payer email and status, no form and no custom fields, so staging does not check the form and still takes the CAVELMS account email from the webhook: an order BCL reports for another amount or payer email, or charged under the webhook's subtotal less discount, is answered 200, gives nothing and is only logged as an error.
3. Create the login, user `cave`. The password is generated into a root-only file; read it there (`cat /etc/cave-lms/staging-login-password`) and give it to the client out of band:

   ```sh
   (umask 077; openssl rand -base64 18 > /etc/cave-lms/staging-login-password)
   pw=$(cat /etc/cave-lms/staging-login-password)
   echo "cave $(printf '%s\n%s\n' "$pw" "$pw" | caddy hash-password 2>/dev/null)" > /etc/caddy/cave-lms-staging-login
   unset pw; chmod 644 /etc/caddy/cave-lms-staging-login
   ```

   To change it, rerun these lines, then `systemctl reload caddy`.
4. From your machine, run `CAVE_ENV=staging deploy/deploy.sh`.
5. Create the Admin as in First install, step 4, with `-p User=cave_staging -p EnvironmentFile=/etc/cave-lms/staging.env /usr/local/bin/cave-lms-staging`.
6. In BCL, set each TEST form (19556, 19624, 19625) under Advanced > Webhook Settings to `https://staging.cavelms.com/cavetext/bcl/<staging key>` with the Payment Success event. In staging's Admin > Prices, set each payment link to the matching TEST form's address (`https://ideaniagaventures.bcl.my/form/cavetext-pro-test`, `lab-plan-test`, `pro-plan-test`) and each price to no more than that form charges, or a TEST payment is kept for an Admin instead of giving the product.

## Changing the domain

The domain lives in two places: `deploy/cave-lms.caddy` and `PUBLIC_BASE_URL` in `/etc/cave-lms/env`.

1. Point the new domain's A record at the VPS (as in First install, step 1).
2. In `deploy/cave-lms.caddy`, change the site address to the new domain and add a block that redirects the old one, keeping the path:

   ```caddy
   old.example.com {
   	bind 169.58.63.110
   	redir https://new.example.com{uri} 308
   }
   ```

3. Set `PUBLIC_BASE_URL` to the new domain in `/etc/cave-lms/env`.
4. Run `deploy/deploy.sh`, then open a Certificate verification link that uses the old domain and check that it lands on the same Certificate on the new one.
5. Email: add the new domain in Resend, copy the DNS records it lists into the new domain's DNS, wait for Verified, then set the new address in `EMAIL_FROM` (see Email below). Once nothing sends from the old domain, delete it in Resend: the free plan allows 3 domains.

A redirect drops a request's sign-in header and webhook senders may not follow it, so when callers outside the browser use the old domain, serve both domains from one site block first (`new.example.com, old.example.com {`), move the callers, then switch the old one to the redirect.

**Standing rule: an old domain keeps redirecting forever.** Never remove an old domain's redirect block, and keep renewing the old domain. A printed or saved Certificate carries its verification link on the domain that was current when the PDF was downloaded. See `docs/adr/0002-old-domains-redirect-forever.md`.

## Upload limit

The app refuses an upload over 2 GB with its own message. Caddy's `request_body` cap (2100 MiB, about 2.2 GB) sits just above it to leave room for multipart framing. Caddy reads `MB` as 10^6 bytes, so keep `MiB`. Caddy streams the body to the app, so the app's cap trips first and even a 2.2 GB upload gets the app's message (tested 2026-10-20: 2047 MiB stored; 2050 MiB and 2200 MiB refused with 413 and nothing stored). Caddy's cap is a backstop.

## System log

Errors and start-ups go to journald and to the `system_log` table, which keeps 30 days. The Owner (`OWNER_EMAIL` in `/etc/cave-lms/env`, an Admin account with that email) reads them at `/admin/log`; every other visitor gets a 404 there. A line that could not reach the database, for example while Postgres is down, is only in journald: `journalctl -u cave-lms -n 100`.

## Datasets in R2

Lab Datasets live in a private Cloudflare R2 bucket (ADR 0008); Postgres keeps only which Dataset belongs to which Dashboard. Only the app holds the bucket's credentials.

**Put the four `R2_` settings in `/etc/cave-lms/env` before the first start of this release.** That start copies every existing Dataset, once, out of the database into whichever store is configured. If R2 is not set yet they land on the disk, and switching to R2 later leaves those Dashboards and their Deployments failing, because the app then looks for their data in the bucket.

1. Cloudflare dashboard > R2 > Create bucket. Name it `cave-lms-datasets`, Location: Automatic with the hint Asia-Pacific (APAC), default storage class Standard. Leave Public access off: no `r2.dev` address, no custom domain.
2. R2 > Manage API tokens > Create API token: permission Object Read & Write, applied to `cave-lms-datasets` only, no expiry (or note when it expires). Copy the Access Key ID and Secret Access Key; the secret is shown once. Use a new token, not the backup one.
3. Add to `/etc/cave-lms/env` with an editor, then `systemctl restart cave-lms` (or deploy):

   ```sh
   R2_ACCOUNT_ID=...            # Cloudflare account ID, on the R2 overview page
   R2_BUCKET=cave-lms-datasets
   R2_ACCESS_KEY_ID=...
   R2_SECRET_ACCESS_KEY=...
   ```

All four or none: with some set and others missing the app stops at start naming the missing ones (`journalctl -u cave-lms -n 20`); with none set Datasets stay on the disk under `STORAGE_DIR`. Failed reads and writes go to the System log; a failed upload tells the Learner and keeps their previous Dataset. The nightly backup does not cover this bucket.

## AI writer

From an Exam's Admin page, Generate questions takes matching published Question bank questions first, then asks the AI to write the rest through [OpenRouter](https://openrouter.ai) (ADR 0009). Only the topic, format, language and count are sent. The AI's questions arrive as Drafts in the Question bank and the Exam. One generation asks for at most 50 questions, with a reply capped at 16000 tokens and a 60-second timeout; a reply in the wrong shape is asked for once more, then reported, and nothing is saved.

The AI also marks each Written answer 0 to 10 with a one-line reason, before the Result is released. Only the question, its model answer and the Learner's answer are sent, nothing about the Learner (ADR 0009). A mark in the wrong shape is asked for once more; an answer still unmarked is asked about again every 5 minutes, and an hour past its release time the System log says so, for the Admin to mark it on Admin > Results.

1. On OpenRouter, create an API key for Cave LMS and **set a monthly credit limit on it**, so a runaway month cannot overspend. Over the limit, the Exam page says so and nothing is saved.
2. Add to `/etc/cave-lms/env` with an editor, then `systemctl restart cave-lms` (or deploy):

   ```sh
   OPENROUTER_API_KEY=sk-or-...
   OPENROUTER_MODEL=...          # an OpenRouter model slug with structured outputs, e.g. openai/gpt-4o-mini
   OPENROUTER_WEB_SEARCH=true    # optional: true (the default) or false
   ```

There is no built-in model: with the key or model unset the app starts, the Exam page says the AI writer is not configured, and Written attempts with non-blank answers wait for the Admin to mark them on Admin > Results; the start-up log says which. `OPENROUTER_WEB_SEARCH` turns OpenRouter's web search on each request, which costs extra; anything but `true` or `false` stops the app at start. Failed AI calls go to the System log with OpenRouter's reason. The instructions sent with each request are `internal/aiwriter/write_questions.txt` for questions and `internal/aiwriter/mark_answer.txt` for marks, built into the binary: edit them and deploy to change the wording.

## Email

Verification and password reset emails go through [Resend](https://resend.com) when `RESEND_API_KEY` is set; the start-up log says which. Free plan as of 2026-10-20: 3000 emails a month, 100 a day, 3 domains. The sending domain `cavelms.com` is verified in Resend through DNS records at NetKL (DKIM `resend._domainkey`, CNAMEs `send` and `rsend`, and `_dmarc`). Resend's dashboard lists the exact records.

A send that fails is logged (`journalctl -u cave-lms | grep "send .* email"`) with Resend's reason, for example an unverified domain or the daily limit. To rotate the key, create a new one in Resend, replace `RESEND_API_KEY` in `/etc/cave-lms/env`, `systemctl restart cave-lms`, then delete the old key.

## Backups

Every night at 03:00 (VPS time, MYT) `cave-lms-backup.timer` runs `/usr/local/bin/cave-lms-backup` (`deploy/backup.sh`). It dumps the `cave` database with `pg_dump` and backs up the dump plus `/var/lib/cave-lms/uploads` with [restic](https://restic.net) to two encrypted repositories:

| Where | Repository |
|---|---|
| On the VPS | `/var/backups/cave-lms/restic` (quick restores after a mistake) |
| Cloudflare R2 | bucket `cave-lms-backups` (survives losing the VPS) |

Each keeps the last 7 days, 4 weeks and 6 months, and is checked after every run. If the VPS was off at 03:00, the backup runs at boot.

| File on the VPS | Holds |
|---|---|
| `/etc/cave-lms/backup.env` | `R2_ENDPOINT`, `R2_BUCKET`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (an R2 token with Object Read & Write on the bucket only), `ALERT_EMAIL`, optional `HEALTHCHECK_URL` |
| `/etc/cave-lms/restic-password` | the encryption password for both repositories |

Both are root-only (mode 0600). **Keep a copy of the restic password in a password manager**: without it the R2 backups cannot be opened, and if the VPS is lost so is the file.

**A failed backup is noticed.** `cave-lms-backup-failed.service` emails `ALERT_EMAIL` through Resend with the last log lines. If `HEALTHCHECK_URL` is set (a free healthchecks.io check with a 1-day period and grace), a missed or silently skipped backup also raises an email.

Useful commands on the VPS:

```sh
systemctl start cave-lms-backup            # run a backup now
journalctl -u cave-lms-backup -n 50        # what the last run did
systemctl list-timers cave-lms-backup      # when the next run is
set -a; . /etc/cave-lms/backup.env; set +a; export RESTIC_PASSWORD_FILE=/etc/cave-lms/restic-password
restic -r /var/backups/cave-lms/restic snapshots
restic -r "s3:$R2_ENDPOINT/$R2_BUCKET" snapshots
```

First install: `install -m 600 /dev/null /etc/cave-lms/backup.env` and fill it in, create the password with `(umask 077; openssl rand -base64 32 > /etc/cave-lms/restic-password)`, copy the password into a password manager, then run `deploy/deploy.sh`, which installs restic and enables the timer once both files exist.

### Restoring

On any machine with restic, Postgres and the R2 settings and restic password (from the password manager):

```sh
export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... RESTIC_PASSWORD=...
repo="s3:https://<account id>.r2.cloudflarestorage.com/cave-lms-backups"
restic -r "$repo" snapshots                       # pick one, or use latest
restic -r "$repo" restore latest --target /tmp/cave-restore
# the dump is at /tmp/cave-restore/var/backups/cave-lms/staging/cave.dump
# the uploads are at /tmp/cave-restore/var/lib/cave-lms/uploads
createdb cave && pg_restore --no-owner --dbname=cave /tmp/cave-restore/var/backups/cave-lms/staging/cave.dump
```

Tested 2026-10-20 on a laptop as the fresh machine: the latest R2 snapshot restored the Admin account and all 10 Gallery items with their files, and the app served them.

Then point `DATABASE_URL` at that database and `STORAGE_DIR` at the restored uploads directory and start the app (on a new VPS: follow First install, restore into the `cave` database as the `cave` user, and copy the uploads to `/var/lib/cave-lms/uploads` owned by `cave`). After a mistake on the live VPS, restore from the local repository instead and stop `cave-lms` while swapping the database.

## Data requests

The privacy notice (`/privacy`, see `docs/adr/0006-the-client-is-the-data-user-and-no-one-reads-snippets.md`) promises a copy, a correction or a deletion within 21 days of an email to the contact. Act only on a request sent from the account's own email address. On the VPS, `sudo -u cave psql cave`, then:

```sql
-- find the account
SELECT id, name, email, verified_at, created_at FROM learners WHERE lower(email) = lower('ada@example.com');
-- correct a name (it changes on every Certificate too)
UPDATE learners SET name = 'Ada Lovelace' WHERE id = 42;
-- delete the account: everything on it goes with it, Certificates included
DELETE FROM learners WHERE id = 42;
```

For a copy, `SELECT *` from `learners` and from each table with a `learner_id` column for that id, leaving out `password_hash` and the token tables (`sessions`, `one_time_tokens`, `cavetext_tokens`), and send the result. Deleted data leaves the backups as they expire, within 6 months.

## Personal data breach

A breach is personal data seen, copied, changed or lost by someone who should not have it: a leaked key, a stolen backup, a bug that showed one Learner's data to another. The PDPA sets the deadlines below. Keep notes with times as you go.

1. **Find and contain.** Work out what was reached and since when (`journalctl -u cave-lms`, `/admin/log`, the VPS's auth log). Stop it: fix or roll back the bug, or stop the site (`systemctl stop cave-lms`) while it still leaks.
2. **Tell the client the same day.** The Cave Consult is the data user; Wasabie Tech, its processor, tells them what happened, whose data and what kind, and what has been done.
3. **The client notifies the Personal Data Protection Commissioner within 72 hours** of the breach being found, when it is likely to cause significant harm (for example password hashes, many accounts, or Snippets and master prompts). Give them the facts for the form.
4. **Tell the affected Learners without delay, within 7 days** of the notification to the Commissioner: what was reached, what was done, and what they should do (for example change a password they reuse elsewhere).
5. **Rotate whatever key was exposed.** `RESEND_API_KEY`: see Email. R2: create a new token in Cloudflare with Object Read & Write on `cave-lms-backups` only, put it in `/etc/cave-lms/backup.env`, run a backup, then delete the old token. restic password: for each repository `restic key add` with the new password, update `/etc/cave-lms/restic-password` and the password manager, then `restic key list` and `restic key remove <old id>`.
6. **Revoke every login**, so a stolen cookie or token stops working (Learners log in again, CaveText signs in again, unused verify and reset links die):

   ```sh
   sudo -u cave psql cave -c 'DELETE FROM sessions; DELETE FROM cavetext_tokens; DELETE FROM one_time_tokens;'
   ```

7. **Restore if data was changed or lost**: from the local repository after a mistake, from R2 when the VPS itself is not trusted (see Restoring). After a break-in, restore onto a fresh VPS and rotate every key above, since the old server held them all.
8. **Record what happened**: when it started and was found, what was reached, who was told and when, and what now stops it happening again. Keep the record even when no one had to be notified.
