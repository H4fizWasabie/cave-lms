# Deploying Cave LMS

The app runs on the shared VPS (`ssh vps`, Ubuntu 24.04) as the `cave-lms` systemd service, listening on `127.0.0.1:8088`. The existing Caddy serves it over HTTPS at `lms.wasabietech.com` from `/etc/caddy/sites/cave-lms.caddy`. Other services share this VPS and this Caddy, so touch only the files named here.

| What | Where on the VPS |
|---|---|
| Binary | `/usr/local/bin/cave-lms` |
| Service | `/etc/systemd/system/cave-lms.service` (from `deploy/cave-lms.service`) |
| Caddy site | `/etc/caddy/sites/cave-lms.caddy` (from `deploy/cave-lms.caddy`) |
| Settings | `/etc/cave-lms/env`, root-owned, mode 0600 |
| Uploads | `/var/lib/cave-lms/uploads` |
| Database | `cave` on the local Postgres 16, peer auth over the Unix socket |

Both the database and the uploads directory are backed up every night (see Backups).

## First install

Once per VPS. Run as root on the VPS.

1. Point an A record for the domain at `169.58.63.110` and wait until `dig +short lms.wasabietech.com` returns it. Caddy can't get a certificate before that.
2. Create the system user, database and settings:

   ```sh
   useradd --system --home /var/lib/cave-lms --shell /usr/sbin/nologin cave
   sudo -u postgres createuser cave
   sudo -u postgres createdb --owner cave cave
   install -d -m 0755 /etc/cave-lms
   install -m 0600 /dev/null /etc/cave-lms/env
   cat > /etc/cave-lms/env <<'EOF'
   DATABASE_URL=postgres:///cave?host=/var/run/postgresql
   PUBLIC_BASE_URL=https://lms.wasabietech.com
   EMAIL_FROM="Cave LMS <noreply@lms.wasabietech.com>"
   STORAGE_DIR=/var/lib/cave-lms/uploads
   LISTEN_ADDR=127.0.0.1:8088
   OWNER_EMAIL=hafizwasabie@gmail.com
   EOF
   ```

   Then add `RESEND_API_KEY=re_...` with an editor (`nano /etc/cave-lms/env`) so the key stays out of shell history. Use a Resend API key with "Sending access" only. Without it the app starts but sends no email (see Email below).

3. From your machine, run `deploy/deploy.sh` (next section).
4. Create the Admin on the VPS. `systemd-run` runs it as `cave` with the service's own env file. `read -s` hides the password as you type, and passing it on stdin keeps it out of shell history and process lists:

   ```sh
   read -rsp 'Admin password: ' pw; echo
   printf '%s\n' "$pw" | systemd-run --quiet --pipe --wait --collect -p User=cave -p EnvironmentFile=/etc/cave-lms/env \
     /usr/local/bin/cave-lms create-admin --name "Abah" --email you@example.com
   unset pw
   ```

## Deploying an update

From a clean checkout of the commit to ship:

```sh
deploy/deploy.sh            # CAVE_HOST=other-host deploy/deploy.sh to target another SSH host
```

It builds a Linux binary, copies the binary, unit and Caddy site to the VPS, restarts `cave-lms`, and waits for it to answer. Migrations run on start. The Caddy site is replaced only if the whole Caddyfile still validates; otherwise the previous one is kept and the script fails. Logs: `journalctl -u cave-lms -n 100`.

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

**Standing rule: an old domain keeps redirecting forever.** Never remove an old domain's redirect block, and keep renewing the old domain. A printed or saved Certificate carries its verification link on the domain that was current when the PDF was downloaded. See `docs/adr/0002-old-domains-redirect-forever.md`.

## Upload limit

The app refuses an upload over 2 GB with its own message. Caddy's `request_body` cap (2100 MiB, about 2.2 GB) sits just above it to leave room for multipart framing. Caddy reads `MB` as 10^6 bytes, so keep `MiB`. Caddy streams the body to the app, so the app's cap trips first and even a 2.2 GB upload gets the app's message (tested 2026-10-20: 2047 MiB stored; 2050 MiB and 2200 MiB refused with 413 and nothing stored). Caddy's cap is a backstop.

## System log

Errors and start-ups go to journald and to the `system_log` table, which keeps 30 days. The Owner (`OWNER_EMAIL` in `/etc/cave-lms/env`, an Admin account with that email) reads them at `/admin/log`; every other visitor gets a 404 there. A line that could not reach the database, for example while Postgres is down, is only in journald: `journalctl -u cave-lms -n 100`.

## Email

Verification and password reset emails go through [Resend](https://resend.com) when `RESEND_API_KEY` is set; the start-up log says which. Free plan as of 2026-10-20: 3000 emails a month, 100 a day, 3 domains. The sending domain `lms.wasabietech.com` is verified in Resend through DNS records at Hostinger (DKIM `resend._domainkey.lms`, plus `send.lms` and `rsend.lms`). Resend's dashboard lists the exact records.

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
