#!/usr/bin/env bash
# Builds Cave LMS, ships it to the VPS with its systemd unit and Caddy site, and restarts it.
# Run from anywhere in the repo: deploy/deploy.sh. First install steps are in docs/deploy.md.
set -euo pipefail

host="${CAVE_HOST:-vps}"
cd "$(git rev-parse --show-toplevel)"

# Deploying a checkout that lacks origin/main would roll production back, so refuse it.
git fetch -q origin main
if ! git merge-base --is-ancestor origin/main HEAD; then
	echo "This checkout is behind origin/main; deploying it would roll production back. Merge or pull origin/main first." >&2
	exit 1
fi

build="$(mktemp -d)"
trap 'rm -rf "$build"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build/cave-lms" ./cmd/cave-lms

scp -q "$build/cave-lms" "$host:/usr/local/bin/cave-lms.new"
scp -q deploy/cave-lms.service "$host:/etc/systemd/system/cave-lms.service"
scp -q deploy/cave-lms.caddy "$host:/etc/caddy/sites/cave-lms.caddy.new"
scp -q deploy/backup.sh "$host:/usr/local/bin/cave-lms-backup"
scp -q deploy/backup-failed.sh "$host:/usr/local/bin/cave-lms-backup-failed"
scp -q deploy/cave-lms-backup.service deploy/cave-lms-backup.timer deploy/cave-lms-backup-failed.service "$host:/etc/systemd/system/"

ssh "$host" bash -s <<'REMOTE'
set -euo pipefail
chmod 755 /usr/local/bin/cave-lms.new
mv /usr/local/bin/cave-lms.new /usr/local/bin/cave-lms
systemctl daemon-reload
systemctl enable --quiet cave-lms
systemctl restart cave-lms

# Backups need restic and the R2 settings in /etc/cave-lms/backup.env (docs/deploy.md, Backups).
chmod 755 /usr/local/bin/cave-lms-backup /usr/local/bin/cave-lms-backup-failed
command -v restic >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq restic >/dev/null
if [ -f /etc/cave-lms/backup.env ] && [ -f /etc/cave-lms/restic-password ]; then
	systemctl enable --quiet --now cave-lms-backup.timer
else
	echo "backups not enabled yet: /etc/cave-lms/backup.env or restic-password is missing" >&2
fi

# Swap in the new Caddy site only if the whole Caddyfile still validates; otherwise keep the old one.
site=/etc/caddy/sites/cave-lms.caddy
if ! cmp -s "$site.new" "$site"; then
	[ -f "$site" ] && cp "$site" "$site.prev"
	mv "$site.new" "$site"
	if ! caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; then
		if [ -f "$site.prev" ]; then mv "$site.prev" "$site"; else rm "$site"; fi
		echo "Caddy config did not validate; kept the previous site file" >&2
		exit 1
	fi
	systemctl reload caddy
else
	rm "$site.new"
fi

# Migrations run before the app listens, so give it a few seconds.
for _ in $(seq 1 15); do
	if curl -fs -o /dev/null http://127.0.0.1:8088/; then
		echo "cave-lms is up"
		exit 0
	fi
	sleep 1
done
echo "cave-lms did not answer on 127.0.0.1:8088; see: journalctl -u cave-lms -n 50" >&2
exit 1
REMOTE
