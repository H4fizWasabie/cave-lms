#!/bin/sh
# Publish the mockup to the VPS behind Caddy (automatic HTTPS).
# Usage: ./deploy.sh                          (defaults below)
#        DOMAIN=demo.example.com ./deploy.sh  (move it to another domain; point its DNS A record at the VPS first)
set -eu
DOMAIN="${DOMAIN:-cave.wasabietech.com}"
HOST="${HOST:-vps}"                          # ssh alias
BIND="${BIND:-169.58.63.110}"                # public IP Caddy listens on (tailscale owns :443 on the other one)
ROOT="${ROOT:-/var/www/cave-mockup}"
SITE=/etc/caddy/sites/cave-mockup.caddy

cd "$(dirname "$0")"
rsync -az --delete --exclude deploy.sh --exclude run.sh --exclude README.md --exclude '*.blender.py' ./ "$HOST:$ROOT/"

ssh "$HOST" DOMAIN="$DOMAIN" BIND="$BIND" ROOT="$ROOT" SITE="$SITE" sh -s <<'REMOTE'
set -eu
mkdir -p /etc/caddy/sites
if ! grep -q '^import sites/\*\.caddy' /etc/caddy/Caddyfile; then
  cp /etc/caddy/Caddyfile "/etc/caddy/Caddyfile.bak-cave-mockup-$(date +%Y%m%d-%H%M%S)"
  printf '\nimport sites/*.caddy\n' >> /etc/caddy/Caddyfile
fi
[ -f "$SITE" ] && cp "$SITE" "$SITE.prev"
cat > "$SITE" <<EOF
# The Cave Consult LMS mockup. Written by prototype/mockup/deploy.sh; change the domain there.
$DOMAIN {
	bind $BIND
	root * $ROOT
	encode zstd gzip
	header X-Robots-Tag "noindex, nofollow"
	file_server
}
EOF
if caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/tmp/cave-mockup-validate.log 2>&1; then
  systemctl reload caddy
else
  cat /tmp/cave-mockup-validate.log
  if [ -f "$SITE.prev" ]; then mv "$SITE.prev" "$SITE"; else rm "$SITE"; fi
  echo "Caddy config invalid; restored the previous site file, nothing reloaded." >&2
  exit 1
fi
REMOTE
echo "Live: https://$DOMAIN/"
