#!/usr/bin/env bash
# Builds Cave LMS, ships it to the VPS with its systemd unit and Caddy site, and restarts it.
# Run from anywhere in the repo: deploy/deploy.sh. First install steps are in docs/deploy.md.
set -euo pipefail

host="${CAVE_HOST:-vps}"
cd "$(git rev-parse --show-toplevel)"

build="$(mktemp -d)"
trap 'rm -rf "$build"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build/cave-lms" ./cmd/cave-lms

scp -q "$build/cave-lms" "$host:/usr/local/bin/cave-lms.new"
scp -q deploy/cave-lms.service "$host:/etc/systemd/system/cave-lms.service"
scp -q deploy/cave-lms.caddy "$host:/etc/caddy/sites/cave-lms.caddy.new"

ssh "$host" bash -s <<'REMOTE'
set -euo pipefail
chmod 755 /usr/local/bin/cave-lms.new
mv /usr/local/bin/cave-lms.new /usr/local/bin/cave-lms
systemctl daemon-reload
systemctl enable --quiet cave-lms
systemctl restart cave-lms

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
