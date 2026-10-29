# Deploying Cave LMS

The app runs on the SG VPS (`ssh vps`, Ubuntu 24.04, with the other apps; the old FR box, `ssh vps-fr`, is kept stopped as a fallback) as the `cave-lms` systemd service, listening on `127.0.0.1:8088`. The existing Caddy serves it over HTTPS at `cavelms.com` (`www.cavelms.com` and the old `lms.wasabietech.com` redirect to it) from `/etc/caddy/sites/cave-lms.caddy`. Learners' Deployments are served by the same app on `pages.cavelms.com` (`docs/adr/0024-deployments-move-to-the-pages-subdomain.md`), from the same Caddy file; its A record must exist before a deploy that adds or changes that block, or Caddy cannot get the certificate. Other services share this VPS and this Caddy, so touch only the files named here.

| What | Where on the VPS |
|---|---|
| Binary | `/usr/local/bin/cave-lms` |
| Service | `/etc/systemd/system/cave-lms.service` (from `deploy/cave-lms.service`) |
| Caddy site | `/etc/caddy/sites/cave-lms.caddy` (from `deploy/cave-lms.caddy`) |
| Settings | `/etc/cave-lms/env`, root-owned, mode 0600 |
| Uploads | `/var/lib/cave-lms/uploads` |
| Database | `cave` on the local Postgres 16, peer auth over the Unix socket. Postgres allows 100 connections shared with the other apps on the VPS; the app holds at most 25 (`maxDBConns` in `internal/app/app.go`) |

Both the database and the uploads directory are backed up every night. First install, changing the domain, backups and restores, data requests and breaches are in `docs/runbook.md`.

## Deploying an update

From a clean checkout of the commit to ship, staging first, then production (ADR 0011):

```sh
CAVE_ENV=staging deploy/deploy.sh
deploy/deploy.sh            # CAVE_HOST=other-host deploy/deploy.sh to target another SSH host
```

It refuses a checkout with uncommitted changes or a commit whose CI run (`.github/workflows/ci.yml`) has not passed, so `gh` must be signed in. A production deploy also refuses a commit that has not passed the smoke test on staging; for an emergency fix, `CAVE_SKIP_STAGING=1 deploy/deploy.sh` skips that check. When the commit adds a migration since the one production runs, or production's commit is not known, it runs a backup first and waits for it; that snapshot is tagged `pre-deploy` and kept out of the nightly pruning (`docs/runbook.md`, Backups). A merge makes a new commit, so after merging a pull request, deploy main to staging again before production.

Then it builds a Linux binary and copies the binary, unit, Caddy site and smoke test to the VPS. The Caddy site is replaced only if the whole Caddyfile still validates; otherwise the previous one is kept and the script fails before anything else changes. Then it swaps the binary, restarts `cave-lms` and waits for it to answer. Migrations and any startup work run before it listens, so after a migration (and always on staging) it waits up to 120 seconds, and 15 seconds otherwise; it gives up at once if the service crashes or exits while starting. Migrations run on start. Logs: `journalctl -u cave-lms -n 100`.

Last comes the smoke test, `deploy/smoke.sh` (installed as `cave-lms-smoke`, and as `cave-lms-staging-smoke` for staging): signed out, over the public HTTPS address, it requests the home page and its stylesheet, `/login`, `/courses`, `/gallery`, `/programmes`, `/pricing` and `/certificates`, and checks that `/plans` redirects to `/pricing`, `/admin` redirects to `/login` and a wrong BCL key gets 404. If it fails on production, the previous binary goes back and the script fails; the binary is all a rollback restores, not the unit, the Caddy site or the database. **After a migration there is no rollback**: when the commit may have added one (the backup above was due), a failed start or smoke test leaves the new binary in place, prints `journalctl -u cave-lms -n 50` and fails, because the previous binary is likely broken against the migrated schema. Fix forward and deploy again, or restore the pre-deploy snapshot (`docs/runbook.md`, Restoring a pre-deploy snapshot); the old binary is `/usr/local/bin/cave-lms.prev` if it is wanted. Run it by hand with `ssh vps cave-lms-smoke production` or `ssh vps cave-lms-staging-smoke staging`. `/var/lib/cave-lms-deploys/` on the VPS holds the commit that last passed on staging and the one production runs.

## Promos

A Promo needs its coupon in BCL as well as a row on Admin > Promos. In BCL, create the coupon, then add it to each payment form's Allowed Coupons (Edit Payment Form > Form Configuration > Coupon Settings). When that list is not empty, BCL refuses every code not on it with "This coupon is not available for this form".

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

It ships only the staging binary, unit and Caddy site, and leaves production and the backups alone. A failed smoke test leaves staging as it is, to be looked at. Logs: `journalctl -u cave-lms-staging -n 100`. Setting staging up from nothing is `docs/runbook.md`, First install of staging.

### What keeps staging off the real world

- **Email** goes only to the addresses in `EMAIL_ALLOWLIST` (comma-separated; `@example.com` allows a whole domain). Any other message is not sent: it goes to the System log (and journald) in full, link included, so the Owner can read it at `staging.cavelms.com/admin/log`. Production leaves `EMAIL_ALLOWLIST` unset and emails everyone.
- **BCL**: `BCL_PAYMENTS=test` counts Test Mode orders as paid and ignores live ones. Each is confirmed with BCL's API when `BCL_API_TOKEN` is set, as on production (which requires it; staging alone may run without, and then takes orders as the webhook sends them), though BCL gives nothing for a Test Mode order but its amount, payer and status, so the form is checked only by amount and price, and the CAVELMS account email on the form must be the payer's email (`docs/runbook.md`, First install of staging, step 2). Testers pay with the email of their staging account, or an Admin gives the product by hand. Production leaves it unset (`live`), so it keeps ignoring Test Mode. The TEST forms send their webhooks to staging.
- **Deployments** are on `pages.staging.cavelms.com`, a second Caddy block in the staging site file. It has no login, unlike `staging.cavelms.com`: a sandboxed page cannot send one with its data requests. Staging holds test data only.
- **Placeholders**: the staging unit sets `CAVE_ENV=staging`, so the home page shows its marked placeholders for numbers the client has not supplied yet. Production leaves `CAVE_ENV` unset (production) and never shows them. Run locally with `CAVE_ENV=development` to see them.
- **Data** starts empty. Set up Courses, Exams and prices by hand in staging's Admin.
- **CaveText** can't sign in to staging: `CAVETEXT_EXTENSION_IDS` is unset and the extension would trip over the login. A Test Mode payment still gives Pro, and the System log says so.

## Upload limit

The app refuses an upload over 2 GB with its own message. Caddy's `request_body` cap (2100 MiB, about 2.2 GB) sits just above it to leave room for multipart framing. Caddy reads `MB` as 10^6 bytes, so keep `MiB`. Caddy streams the body to the app, so the app's cap trips first and even a 2.2 GB upload gets the app's message (tested 2026-10-20: 2047 MiB stored; 2050 MiB and 2200 MiB refused with 413 and nothing stored). Caddy's cap is a backstop. An Assignment upload has its own smaller cap: 5 files of 20 MB, refused above about 101 MB.

**A stalled upload.** Caddy aborts a request whose body sends nothing for `read_body_idle`, which defaults to 1 minute. A class on one weak school connection, all uploading at once, can stall that long. The 200-Learner staging run of 2026-10-27 lost 9 Assignment uploads this way: a 502 after exactly 60 s, with Caddy logging `readfrom tcp ...->127.0.0.1:8089: i/o timeout` (8089 is staging's port).

- **Not the cause**: the app has no body timeout (only `ReadHeaderTimeout`), and a steady upload of 80 s or more passes. Only silence counts, so a long steady upload of any size, a 2 GB Gallery file included, is unaffected.
- **The fix**: raise the limit to 5 minutes. It's the one change outside the files named above, a global option in the shared `/etc/caddy/Caddyfile`, so it applies to every site on this Caddy. Not yet applied: as of 2026-10-27 the VPS still runs the 1-minute default. If the file has a global block (a `{ ... }` before the first site), add the `servers` part inside it. A second global block makes the Caddyfile invalid. If it has none, add this as its first block:

  ```
  {
  	servers {
  		timeouts {
  			read_body_idle 5m
  		}
  	}
  }
  ```

  Then, as root:

  ```sh
  cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak-read-body-idle-$(date +%Y%m%d)
  # edit /etc/caddy/Caddyfile as above
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile && systemctl reload caddy
  ```

- **Why not per route**: Caddy's `timeouts` directive (experimental) can't do it. Tested on the VPS's Caddy v2.11.7 on 2026-10-27:
  - A route's `read_timeout` only takes effect once the global `read_body_idle` is off (`-1s`), and that would leave every other site on this Caddy with no body timeout at all.
  - Its `min_rate` argument didn't abort an upload trickling at 100 B/s.
- **What the raised limit costs**:
  - A completely silent body is now held 5 minutes, not 1, on every site here, before Caddy drops it.
  - A body that trickles a byte more often than that was never cut off, under 1 minute or 5. Nothing here limits a slow sender's rate.
- **To check it**: with the same Caddy on a spare local port in front of a small upstream, send a fixed-length body that pauses 70 s halfway. Under the default it fails after 60 s; with `5m` it gets 200.

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

Assignment Submission files (spec #196) live in the same bucket, under keys starting `submission-<learner id>-`, with the same fallback to the disk. A deleted file or Assignment leaves the bucket at once; a deleted account's files leave it at the next daily prune (or restart). A file the bucket fails to delete stays queued in `stored_file_deletions` and is tried again then.

## AI writer

From an Exam's Admin page, Generate questions takes matching published Question bank questions first, then asks the AI to write the rest through [OpenRouter](https://openrouter.ai) (ADR 0009). Only the topic, format, language and count are sent. The AI's questions arrive as Drafts in the Question bank and the Exam. One generation asks for at most 50 questions, with a reply capped at 16000 tokens and a 60-second timeout; a reply in the wrong shape is asked for once more, then reported, and nothing is saved.

The AI also marks each Written answer 0 to 10 with a one-line reason, before the Result is released. Only the question, its model answer and the Learner's answer are sent, nothing about the Learner (ADR 0009). A mark in the wrong shape is asked for once more; an answer still unmarked is asked about again every 5 minutes, and an hour past its release time the System log says so, for the Admin to mark it on Admin > Results.

On the Lesson form, Generate text has the AI write the Lesson's text, about 500 to 900 words of Markdown, into the Text box; nothing is saved until the Admin clicks Save. Only the Course title, the Lesson's title and Objectives, the other Lessons' titles and the chosen language are sent, nothing about a Learner. A reply that is not 300 to 1500 words, or is over a Lesson's 20000 characters, is asked for once more, then reported.

1. On OpenRouter, create an API key for Cave LMS and **set a monthly credit limit on it**, so a runaway month cannot overspend. Over the limit, the Exam page says so and nothing is saved.
2. Add to `/etc/cave-lms/env` with an editor, then `systemctl restart cave-lms` (or deploy):

   ```sh
   OPENROUTER_API_KEY=sk-or-...
   OPENROUTER_MODEL=...          # an OpenRouter model slug with structured outputs, e.g. openai/gpt-4o-mini
   OPENROUTER_WEB_SEARCH=true    # optional: true (the default) or false
   ```

There is no built-in model: with the key or model unset the app starts, the Exam page and the Lesson form say the AI writer is not configured, and Written attempts with non-blank answers wait for the Admin to mark them on Admin > Results; the start-up log says which. `OPENROUTER_WEB_SEARCH` turns OpenRouter's web search on each request, which costs extra; anything but `true` or `false` stops the app at start. Failed AI calls go to the System log with OpenRouter's reason. The instructions sent with each request are `internal/aiwriter/write_questions.txt` for questions, `internal/aiwriter/mark_answer.txt` for marks and `internal/aiwriter/write_lesson.txt` for Lesson text, built into the binary: edit them and deploy to change the wording.

## CaveText

`CAVETEXT_EXTENSION_IDS` is the comma-separated Chrome extension IDs that may sign in to CaveText through `/cavetext/connect`; unset, sign-in is refused (staging leaves it unset). Production lists three:

- `omcdclpagggfcjlhpkohpcgdmfbelohd`: the unpacked development build.
- `akkjngndkjadciijkjblnojfgbbhhmnl`: the zip shared before store approval, built by CaveText's `package.sh` with a fixed key.
- `ckbckpnpljpmcoeajhokjpombijphbkd`: the Chrome Web Store item.

## Email

Verification and password reset emails go through [Resend](https://resend.com) when `RESEND_API_KEY` is set; the start-up log says which. Free plan as of 2026-10-20: 3000 emails a month, 100 a day, 3 domains. The sending domain `cavelms.com` is verified in Resend through DNS records on Cloudflare (DKIM `resend._domainkey`, CNAMEs `send` and `rsend`, and `_dmarc`). Resend's dashboard lists the exact records.

A send that fails is logged (`journalctl -u cave-lms | grep "send .* email"`) with Resend's reason, for example an unverified domain or the daily limit. To rotate the key, create a new one in Resend, replace `RESEND_API_KEY` in `/etc/cave-lms/env`, `systemctl restart cave-lms`, then delete the old key.

## Query statistics

`pg_stat_statements` is installed on production and staging (#321).
