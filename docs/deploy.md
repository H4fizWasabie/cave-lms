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

Back up both the database and the uploads directory (#14).

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
   EMAIL_FROM=noreply@lms.wasabietech.com
   STORAGE_DIR=/var/lib/cave-lms/uploads
   LISTEN_ADDR=127.0.0.1:8088
   EOF
   ```

3. From your machine, run `deploy/deploy.sh` (next section).
4. Create the Admin on the VPS. `read -s` hides the password as you type, and passing it on stdin keeps it out of shell history and process lists:

   ```sh
   read -rsp 'Admin password: ' pw; echo
   printf '%s\n' "$pw" | sudo -u cave env $(grep -v '^#' /etc/cave-lms/env | xargs) \
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
5. Email: the sender domain must be re-verified with the email service (see #12).

**Standing rule: an old domain keeps redirecting forever.** Never remove an old domain's redirect block, and keep renewing the old domain. A printed or saved Certificate carries its verification link on the domain that was current when the PDF was downloaded. See `docs/adr/0002-old-domains-redirect-forever.md`.

## Upload limit

The app refuses an upload over 2 GB with its own message. Caddy's `request_body` cap (2100 MiB, about 2.2 GB) sits just above it to leave room for multipart framing. Caddy reads `MB` as 10^6 bytes, so keep `MiB`. Caddy streams the body to the app, so the app's cap trips first and even a 2.2 GB upload gets the app's message (tested 2026-10-20: 2047 MiB stored; 2050 MiB and 2200 MiB refused with 413 and nothing stored). Caddy's cap is a backstop.
