#!/usr/bin/env bash
# Builds Cave LMS, ships it to the VPS with its systemd unit and Caddy site, restarts it and smoke-tests it (ADR 0011).
# Run from anywhere in the repo: deploy/deploy.sh for production, CAVE_ENV=staging deploy/deploy.sh for staging.
# First install steps are in docs/deploy.md.
set -euo pipefail

host="${CAVE_HOST:-vps}"
env="${CAVE_ENV:-production}"
case "$env" in
production) name=cave-lms port=8088 ;;
staging) name=cave-lms-staging port=8089 ;;
*)
	echo "CAVE_ENV must be production or staging" >&2
	exit 1
	;;
esac
cd "$(git rev-parse --show-toplevel)"

# Deploying a checkout that lacks origin/main would roll production back, so refuse it. Staging takes any branch.
if [ "$name" = cave-lms ]; then
	git fetch -q origin main
	if ! git merge-base --is-ancestor origin/main HEAD; then
		echo "This checkout is behind origin/main; deploying it would roll production back. Merge or pull origin/main first." >&2
		exit 1
	fi
fi

# The build uses the working tree, so ship only a clean commit that CI passed.
if ! git diff --quiet HEAD || [ -n "$(git ls-files --others --exclude-standard -- '*.go' go.mod go.sum deploy)" ]; then
	echo "This checkout has uncommitted changes; CI never tested them. Commit, push and wait for CI first." >&2
	exit 1
fi
sha="$(git rev-parse HEAD)"
if [ "$(gh run list --workflow ci.yml --commit "$sha" --status success --limit 1 --json databaseId --jq length)" != 1 ]; then
	echo "CI has not passed on $sha; see: gh run list --commit $sha" >&2
	exit 1
fi

# The VPS keeps the commit that last passed the smoke test on staging, and the one production runs.
records=/var/lib/cave-lms-deploys
backup=no
if [ "$env" = production ]; then
	if [ "${CAVE_SKIP_STAGING:-}" = 1 ]; then
		echo "WARNING: CAVE_SKIP_STAGING=1: shipping $sha to production without it passing staging" >&2
	elif [ "$(ssh "$host" cat "$records/staging-passed" 2>/dev/null)" != "$sha" ]; then
		echo "$sha has not passed the smoke test on staging. Ship it there first: CAVE_ENV=staging deploy/deploy.sh" >&2
		exit 1
	fi
	# A rollback cannot undo a migration, so back up first when this commit adds one, or production's commit is unknown.
	running="$(ssh "$host" cat "$records/production" 2>/dev/null || true)"
	if [ -z "$running" ] || ! git cat-file -e "$running^{commit}" 2>/dev/null ||
		[ -n "$(git diff --name-only --diff-filter=A "$running" HEAD -- internal/app/migrations)" ]; then
		backup=yes
	fi
fi

build="$(mktemp -d)"
trap 'rm -rf "$build"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build/cave-lms" ./cmd/cave-lms

scp -q "$build/cave-lms" "$host:/usr/local/bin/$name.new"
scp -q "deploy/$name.service" "$host:/etc/systemd/system/$name.service"
scp -q "deploy/$name.caddy" "$host:/etc/caddy/sites/$name.caddy.new"
scp -q deploy/smoke.sh "$host:/usr/local/bin/cave-lms-smoke"
# Backups cover production only, and a staging branch must not change them.
if [ "$name" = cave-lms ]; then
	scp -q deploy/backup.sh "$host:/usr/local/bin/cave-lms-backup"
	scp -q deploy/backup-failed.sh "$host:/usr/local/bin/cave-lms-backup-failed"
	scp -q deploy/cave-lms-backup.service deploy/cave-lms-backup.timer deploy/cave-lms-backup-failed.service "$host:/etc/systemd/system/"
fi

ssh "$host" bash -s -- "$env" "$name" "$port" "$sha" "$backup" "$records" <<'REMOTE'
set -euo pipefail
env=$1 name=$2 port=$3 sha=$4 backup=$5 records=$6
bin="/usr/local/bin/$name"
chmod 755 "$bin.new" /usr/local/bin/cave-lms-smoke
install -d -m 755 "$records"
if [ "$backup" = yes ]; then
	echo "this commit adds a migration: backing up first"
	systemctl start cave-lms-backup
fi
# The previous binary stays as .prev, for the rollback below.
[ -f "$bin" ] && cp "$bin" "$bin.prev"
mv "$bin.new" "$bin"
systemctl daemon-reload
systemctl enable --quiet "$name"
systemctl restart "$name"

if [ "$name" = cave-lms ]; then
	# Backups need restic and the R2 settings in /etc/cave-lms/backup.env (docs/deploy.md, Backups).
	chmod 755 /usr/local/bin/cave-lms-backup /usr/local/bin/cave-lms-backup-failed
	command -v restic >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq restic >/dev/null
	if [ -f /etc/cave-lms/backup.env ] && [ -f /etc/cave-lms/restic-password ]; then
		systemctl enable --quiet --now cave-lms-backup.timer
	else
		echo "backups not enabled yet: /etc/cave-lms/backup.env or restic-password is missing" >&2
	fi
fi

# Swap in the new Caddy site only if the whole Caddyfile still validates; otherwise keep the old one.
site="/etc/caddy/sites/$name.caddy"
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

# Migrations run before the app listens, so give it a few seconds; then the smoke test must pass.
up_and_smoke_tested() {
	for _ in $(seq 1 15); do
		if curl -fs -o /dev/null "http://127.0.0.1:$port/"; then
			cave-lms-smoke "$env"
			return
		fi
		sleep 1
	done
	echo "$name did not answer on 127.0.0.1:$port; see: journalctl -u $name -n 50" >&2
	return 1
}
if up_and_smoke_tested; then
	if [ "$env" = staging ]; then echo "$sha" >"$records/staging-passed"; else echo "$sha" >"$records/production"; fi
	echo "$name is up and passed the smoke test"
	exit 0
fi
# Staging stays broken to be looked at; production goes back to the previous binary.
if [ "$env" = production ] && [ -f "$bin.prev" ]; then
	echo "rolling production back to the previous binary" >&2
	mv "$bin.prev" "$bin"
	systemctl restart "$name"
	up_and_smoke_tested || echo "the previous binary fails the smoke test too" >&2
fi
exit 1
REMOTE
