# Running Cave LMS: rare procedures

Done once, or only when something goes wrong. Routine deploys, Staging, Promos, the Upload limit, the System log, Datasets in R2, the AI writer, CaveText and Email are in `docs/deploy.md`; the VPS layout is its first table. Run as root on the VPS unless a step says otherwise.

## First install

Once per VPS. Run as root on the VPS.

1. Point A records for `cavelms.com`, `www.cavelms.com` and `pages.cavelms.com` (the Deployments host, ADR 0024) at `13.140.65.183` (DNS is at NetKL) and wait until `dig +short cavelms.com` and `dig +short pages.cavelms.com` return it. Caddy can't get a certificate before that.
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
   CAVETEXT_EXTENSION_IDS=omcdclpagggfcjlhpkohpcgdmfbelohd,akkjngndkjadciijkjblnojfgbbhhmnl,ckbckpnpljpmcoeajhokjpombijphbkd
   EOF
   ```

   Then add `RESEND_API_KEY=re_...` with an editor (`nano /etc/cave-lms/env`) so the key stays out of shell history. Use a Resend API key with "Sending access" only. Without it the app starts but sends no email (see `docs/deploy.md`, Email).

   `PAGES_BASE_URL` (the Deployments host, ADR 0024) is optional: unset, it is `https://pages.` in front of `PUBLIC_BASE_URL`'s host, `https://pages.cavelms.com` here. Set it only to put Deployments on another host, which needs its own Caddy block and A record. It must be a host of its own, with no path.

   For CaveText Pro payments, add `BCL_WEBHOOK_KEY` the same way, set to a random key of at least 32 characters (`openssl rand -hex 24`). BCL's webhook address is then `https://cavelms.com/cavetext/bcl/<key>`; set it on the payment form under Advanced > Webhook Settings, with the Payment Success event. Only live orders on the form that Admin > CaveText settings > Payment link points to give Pro; Test Mode orders are only logged. The Test Mode copies of the forms send to staging instead (see `docs/deploy.md`, Staging). Without the key every webhook gets 404.

   BCL signs no webhook, so also add `BCL_API_TOKEN`: in BCL, Platform Setup > Integrations > Developer, create an API token with Read access, named for production. Each paid order is then read from BCL's order detail, `https://bcl.my/api/transactions/<order>` (live orders only), and gives nothing unless BCL has it successful (status 3), paid and live. Everything that decides the grant comes from that detail, not the webhook, whose body gives only the event and the order number: the payer email, the amounts (subtotal, discount, amount charged), the CAVELMS account email field and the form. The form is BCL's form id for the order (`formable_id`), matched to a product through `https://bcl.my/api/forms`, read on every order: the product is the one whose payment link (Admin > Prices) ends in the slug BCL lists with that id, so two forms with one title are told apart and renaming a form is safe. A webhook value that disagrees with BCL's (its `form_url`, payer email, amounts, or another CAVELMS account than BCL's order names) is noted in the order's System log line, as an error, without any email, and BCL's is used: a forged body racing BCL's own webhook gives what BCL's order bought to the account BCL's order names, and BCL's webhook after it is a repeat. An order paid on a form that is no product's, or that BCL charged under its subtotal less discount, may still be a real payment: it gives nothing, is answered 200, is kept under Payments not matched on Admin > CaveText Learners for an Admin to give by hand, and writes a System log error naming the order and why. An order BCL does not know yet (404) or has not marked successful, paid and live (any case), whose detail names no form, or BCL out of reach or failing (10 s timeout), is answered 500 and logged, and BCL retries: a real payment is given once BCL confirms it. A forged order number keeps failing that way, so many such System log errors for one order need a look. A revoked, expired or wrong token fails every order the same way, logged as `BCL API token refused (401): replace BCL_API_TOKEN` (or 403). The app also reads BCL's forms with the token at startup and once a day, and writes that same System log error when BCL refuses it, or one saying it could not check when BCL is out of reach; it starts either way. To replace the token: in BCL, Platform Setup > Integrations > Developer, create a new API token with Read access, named for production (or staging); set `BCL_API_TOKEN` to it in `/etc/cave-lms/env` (production) or `/etc/cave-lms/staging.env` (staging); restart with `systemctl restart cave-lms` (or `cave-lms-staging`); check the System log has no refused-token error after the start-up line; then delete the old token in BCL. Orders BCL retried meanwhile are given on their next retry. Without the token, orders are not confirmed, the app says so at startup, and anyone who learns the webhook address can forge one. `BCL_API_URL` (default `https://bcl.my/api`) is for tests only.

   For Datasets in R2, add the four `R2_` settings the same way (see `docs/deploy.md`, Datasets in R2). Without them Datasets are kept under `STORAGE_DIR`.

   For AI-written Exam questions and Lesson text, add the `OPENROUTER_` settings the same way (see `docs/deploy.md`, AI writer). Without them the Exam page and the Lesson form say the AI writer is not configured.

3. From your machine, run `deploy/deploy.sh` (`docs/deploy.md`, Deploying an update).
4. Create the Admin on the VPS. `systemd-run` runs it as `cave` with the service's own env file. `read -s` hides the password as you type, and passing it on stdin keeps it out of shell history and process lists:

   ```sh
   read -rsp 'Admin password: ' pw; echo
   printf '%s\n' "$pw" | systemd-run --quiet --pipe --wait --collect -p User=cave -p EnvironmentFile=/etc/cave-lms/env \
     /usr/local/bin/cave-lms create-admin --name "Abah" --email you@example.com
   unset pw
   ```

   Running it again for an email that already has an account replaces that password, makes the account an Admin and signs it out everywhere: every session and CaveText token ends, as with a password reset. Use it to recover an Admin account.

## First install of staging

Run as root on the VPS unless a step says otherwise.

1. At NetKL, add A records `staging.cavelms.com` and `pages.staging.cavelms.com` (staging's Deployments host) → `13.140.65.183`, then wait until `dig +short staging.cavelms.com` and `dig +short pages.staging.cavelms.com` return it. Staging's `PAGES_BASE_URL` is left unset: it defaults to `https://pages.staging.cavelms.com`.
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

   Add the client's addresses to `EMAIL_ALLOWLIST`. Then, with an editor, add `RESEND_API_KEY` (a new Resend key with Sending access only, named for staging, so it can be revoked alone; it shares production's 100 emails a day), `BCL_WEBHOOK_KEY` (a new one, `openssl rand -hex 24`, not production's) and `BCL_API_TOKEN` (a new BCL API token with Read access, from Platform Setup > Integrations > Developer, named for staging so it can be revoked alone). With `BCL_PAYMENTS=test` orders are looked up at `https://bcl.my/api/transaction/<order>`, which also finds Test Mode orders; production's `/transactions/<order>` answers 404 for them. This lookup gives only the order's amount charged, payer email and status: no form and no custom fields. BCL's API has no other record of a Test Mode order (its order detail, list and customer records leave them out), so the form and the CAVELMS account email can only come from the webhook. Staging therefore checks the form only by amount and price: a dearer Test Mode order can be claimed as a cheaper TEST product, never the reverse. It binds the account to BCL's payer (see `docs/deploy.md`, Staging, the BCL bullet under What keeps staging off the real world). A CAVELMS account email that is not BCL's payer email (ignoring case and spaces) gives nothing and is answered 200. The order is kept under Payments not matched for an Admin, and a System log error notes it without either email. A webhook without the field gives the order to the payer. An order BCL reports for another amount or payer email, or charged under the webhook's subtotal less discount, is answered 200, gives nothing and is only logged as an error.
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

The domain lives in two places: `deploy/cave-lms.caddy` and `PUBLIC_BASE_URL` in `/etc/cave-lms/env`. Deployments are on `pages.` in front of it (ADR 0024), so they move with it unless `PAGES_BASE_URL` is set.

1. Point the new domain's A record, and the `pages.` one, at the VPS (as in First install, step 1).
2. In `deploy/cave-lms.caddy`, change the site address and the `pages.` block's to the new domain and add a block that redirects the old one, keeping the path:

   ```caddy
   old.example.com {
   	bind 13.140.65.183
   	redir https://new.example.com{uri} 308
   }
   ```

3. Set `PUBLIC_BASE_URL` to the new domain in `/etc/cave-lms/env`.
4. Run `deploy/deploy.sh`, then open a Certificate verification link that uses the old domain and check that it lands on the same Certificate on the new one. A Deployment link shared at the old domain's `pages.` host is not redirected by Cave: keep a `redir` block for it too, or those links die.
5. Email: add the new domain in Resend, copy the DNS records it lists into the new domain's DNS, wait for Verified, then set the new address in `EMAIL_FROM` (see `docs/deploy.md`, Email). Once nothing sends from the old domain, delete it in Resend: the free plan allows 3 domains.

A redirect drops a request's sign-in header and webhook senders may not follow it, so when callers outside the browser use the old domain, serve both domains from one site block first (`new.example.com, old.example.com {`), move the callers, then switch the old one to the redirect.

**Standing rule: an old domain keeps redirecting forever.** Never remove an old domain's redirect block, and keep renewing the old domain. A printed or saved Certificate carries its verification link on the domain that was current when the PDF was downloaded. See `docs/adr/0002-old-domains-redirect-forever.md`.

## Backups

Every night at 03:00 (VPS time, MYT) `cave-lms-backup.timer` runs `/usr/local/bin/cave-lms-backup` (`deploy/backup.sh`). It dumps the `cave` database with `pg_dump`, mirrors the Datasets bucket `cave-lms-datasets` (ADR 0008, Datasets and Assignment Submission files) into `/var/backups/cave-lms/datasets` with `rclone sync` using the app's R2 settings in `/etc/cave-lms/env`, and backs up the dump, `/var/lib/cave-lms/uploads` and that mirror with [restic](https://restic.net) to two encrypted repositories:

| Where | Repository |
|---|---|
| On the VPS | `/var/backups/cave-lms/restic` (quick restores after a mistake) |
| Cloudflare R2 | bucket `cave-lms-backups` (survives losing the VPS) |

Each keeps the last 7 days, 4 weeks and 6 months of the nightly snapshots (tag `nightly`), and is checked after every run. The backup `deploy/deploy.sh` takes before a commit that adds a migration is a separate snapshot tagged `pre-deploy` and `commit-<first 12 characters of the commit>`; the nightly pruning never touches it, and its own policy keeps the last 10 in each repository. It also leaves a copy of its dump in `/var/backups/cave-lms/pre-deploy/` (last 10), which later runs do not overwrite, and it does not ping `HEALTHCHECK_URL`. It runs through `systemctl start cave-lms-backup` with `CAVE_PREDEPLOY_SHA` set in systemd's environment for that call. A nightly run still in progress when the deploy starts would be joined, not repeated, and would leave no `pre-deploy` snapshot. If the VPS was off at 03:00, the backup runs at boot.

Each snapshot therefore holds the database rows and the Dataset objects from the same moment. A Learner's Delete still removes the object from R2 at once, and it leaves the backups with the last snapshot that holds it: within 6 months, as the privacy notice promises. Backup copies do not count against the Storage allowance. A Dataset created and deleted between two nightly runs is never backed up. The mirror is a full copy on the VPS disk (112 GB free in October 2026); once the bucket passes about 50 GB, switch to a bucket-to-bucket copy or a soft delete in the app.

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
restic -r /var/backups/cave-lms/restic snapshots --tag pre-deploy    # only the ones taken before a migration
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

The Datasets are at `/tmp/cave-restore/var/backups/cave-lms/datasets`; copy them back with `rclone copy` (below).

**A backup from before #411 (2026-10-27)** holds Datasets in the shape from before Parts, which only a release from before the Parts-conversion removal (#466) rewrites at start-up. Boot such a release once on the restored database and Datasets, let it finish starting, stop it, and only then start the current release. The current release does not convert them.

Then point `DATABASE_URL` at that database and `STORAGE_DIR` at the restored uploads directory and start the app (on a new VPS: follow First install, restore into the `cave` database as the `cave` user, and copy the uploads to `/var/lib/cave-lms/uploads` owned by `cave`). After a mistake on the live VPS, restore from the local repository instead and stop `cave-lms` while swapping the database. Once Query statistics is on (`docs/deploy.md`), the dump also holds `CREATE EXTENSION pg_stat_statements`: a restore as `cave` reports errors for that and loads everything else; as `postgres` on Postgres 16 it restores in full.

### Restoring a pre-deploy snapshot

After a bad migration, restore the snapshot taken just before it. Its dump, uploads and Datasets mirror are from the same moment. `restic snapshots --tag pre-deploy` lists them with their time and `commit-<sha>` tag; take the last one before the deploy that failed. The dump is at the same path as in any snapshot, and a copy sits in `/var/backups/cave-lms/pre-deploy/`:

```sh
set -a; . /etc/cave-lms/backup.env; set +a; export RESTIC_PASSWORD_FILE=/etc/cave-lms/restic-password
repo=/var/backups/cave-lms/restic            # or "s3:$R2_ENDPOINT/$R2_BUCKET"
restic -r $repo snapshots --tag pre-deploy
systemctl stop cave-lms
restic -r $repo restore <snapshot> --target /tmp/r
runuser -u postgres -- dropdb cave && runuser -u postgres -- createdb --owner cave cave
runuser -u postgres -- pg_restore --no-owner --role=cave --dbname=cave /tmp/r/var/backups/cave-lms/staging/cave.dump
mv /usr/local/bin/cave-lms.prev /usr/local/bin/cave-lms   # the binary that matches this schema, if the deploy swapped it out
systemctl start cave-lms
```

The Datasets objects are in `/tmp/r/var/backups/cave-lms/datasets`; put back only the ones the migration or conversion removed or rewrote, with `rclone copy` as in the next section. Drop `/tmp/r` afterwards.

### Restoring one deleted Dashboard's Datasets

On the VPS, from the local repository (use the R2 one if the VPS repository is gone). Pick the last snapshot taken before the delete:

```sh
set -a; . /etc/cave-lms/backup.env; set +a; export RESTIC_PASSWORD_FILE=/etc/cave-lms/restic-password
repo=/var/backups/cave-lms/restic
restic -r $repo snapshots
# the database rows: restore that snapshot's dump into a scratch database and read the Dashboard's Dataset keys
restic -r $repo restore <snapshot> --include /var/backups/cave-lms/staging/cave.dump --target /tmp/r
runuser -u postgres -- createdb cave_restore
runuser -u postgres -- pg_restore --no-owner --dbname=cave_restore /tmp/r/var/backups/cave-lms/staging/cave.dump
# the objects: one --include per key
restic -r $repo restore <snapshot> --include /var/backups/cave-lms/datasets/<key> --target /tmp/r
# put them back in the Datasets bucket with the app's R2 settings
(set -a; . /etc/cave-lms/env; set +a
 export RCLONE_CONFIG_R2_TYPE=s3 RCLONE_CONFIG_R2_PROVIDER=Cloudflare RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true \
   RCLONE_CONFIG_R2_ENDPOINT="https://$R2_ACCOUNT_ID.r2.cloudflarestorage.com" \
   RCLONE_CONFIG_R2_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
 rclone copy /tmp/r/var/backups/cave-lms/datasets "r2:$R2_BUCKET")
```

Then copy the Dashboard's rows from `cave_restore` into `cave` by hand with SQL, and drop `cave_restore` and `/tmp/r`.

Tested 2026-10-25 on the VPS with a scratch restic repository: a Dataset object mirrored, backed up, deleted locally and restored came back byte-identical to the live object. The final `rclone copy` into the bucket was not run, to leave production data untouched.

## Data requests

The privacy notice (`/privacy`, see `docs/adr/0006-the-client-is-the-data-user-and-no-one-reads-snippets.md`) promises a copy, a correction or a deletion within 21 days of an email to the contact. Act only on a request sent from the account's own email address. On the VPS, `sudo -u cave psql cave`, then:

```sql
-- find the account
SELECT id, name, email, verified_at, created_at FROM learners WHERE lower(email) = lower('ada@example.com');
-- correct a name (it changes on every Certificate too)
UPDATE learners SET name = 'Ada Lovelace' WHERE id = 42;
-- delete the account: everything on it goes with it, Certificates and Submissions included
-- (Submission files leave R2 at the next daily prune)
DELETE FROM learners WHERE id = 42;
```

For a copy, `SELECT *` from `learners` and from each table with a `learner_id` column for that id, leaving out `password_hash` and the token tables (`sessions`, `one_time_tokens`, `cavetext_tokens`), and send the result. Deleted data leaves the backups as they expire, within 6 months.

## Personal data breach

A breach is personal data seen, copied, changed or lost by someone who should not have it: a leaked key, a stolen backup, a bug that showed one Learner's data to another. The PDPA sets the deadlines below. Keep notes with times as you go.

1. **Find and contain.** Work out what was reached and since when (`journalctl -u cave-lms`, `/admin/log`, the VPS's auth log). Stop it: fix or roll back the bug, or stop the site (`systemctl stop cave-lms`) while it still leaks.
2. **Tell the client the same day.** The Cave Consult is the data user; Wasabie Tech, its processor, tells them what happened, whose data and what kind, and what has been done.
3. **The client notifies the Personal Data Protection Commissioner within 72 hours** of the breach being found, when it is likely to cause significant harm (for example password hashes, many accounts, or Snippets and master prompts). Give them the facts for the form.
4. **Tell the affected Learners without delay, within 7 days** of the notification to the Commissioner: what was reached, what was done, and what they should do (for example change a password they reuse elsewhere).
5. **Rotate whatever key was exposed.** `RESEND_API_KEY`: see `docs/deploy.md`, Email. R2: create a new token in Cloudflare with Object Read & Write on `cave-lms-backups` only, put it in `/etc/cave-lms/backup.env`, run a backup, then delete the old token. restic password: for each repository `restic key add` with the new password, update `/etc/cave-lms/restic-password` and the password manager, then `restic key list` and `restic key remove <old id>`.
6. **Revoke every login**, so a stolen cookie or token stops working (Learners log in again, CaveText signs in again, unused verify and reset links die):

   ```sh
   sudo -u cave psql cave -c 'DELETE FROM sessions; DELETE FROM cavetext_tokens; DELETE FROM one_time_tokens;'
   ```

7. **Restore if data was changed or lost**: from the local repository after a mistake, from R2 when the VPS itself is not trusted (see Restoring). After a break-in, restore onto a fresh VPS and rotate every key above, since the old server held them all.
8. **Record what happened**: when it started and was found, what was reached, who was told and when, and what now stops it happening again. Keep the record even when no one had to be notified.
