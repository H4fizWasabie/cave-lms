#!/usr/bin/env bash
# Builds Cave LMS, ships it to the VPS with its systemd unit and Caddy site, restarts it and smoke-tests it (ADR 0011).
# Run from anywhere in the repo: deploy/deploy.sh for production, CAVE_ENV=staging deploy/deploy.sh for staging.
# First install steps are in docs/runbook.md.
set -euo pipefail

# vps: the SG box, where Cave runs with the other apps. The old FR box (vps-fr) is kept stopped as a fallback.
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
		[ -n "$(git diff --name-only --diff-filter=A "$running" HEAD -- internal/migrations)" ]; then
		backup=yes
	fi
fi

build="$(mktemp -d)"
trap 'rm -rf "$build"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build/cave-lms" ./cmd/cave-lms

scp -q "$build/cave-lms" "$host:/usr/local/bin/$name.new"
scp -q "deploy/$name.service" "$host:/etc/systemd/system/$name.service"
scp -q "deploy/$name.caddy" "$host:/etc/caddy/sites/$name.caddy.new"
# Each site has its own copy, so a staging branch cannot change production's smoke test.
scp -q deploy/smoke.sh "$host:/usr/local/bin/$name-smoke"
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
chmod 755 "$bin.new" "$bin-smoke"
install -d -m 755 "$records"
systemctl daemon-reload

# Everything that can fail goes before the binary swap, so a failure leaves the running site as it was.
if [ "$name" = cave-lms ]; then
	# Backups need restic, rclone and the R2 settings in /etc/cave-lms/backup.env (docs/runbook.md, Backups).
	chmod 755 /usr/local/bin/cave-lms-backup /usr/local/bin/cave-lms-backup-failed
	for tool in restic rclone; do
		command -v $tool >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $tool >/dev/null
	done
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

if [ "$backup" = yes ]; then
	if [ -f /etc/cave-lms/backup.env ]; then
		echo "the commit may add a migration: backing up first"
		# backup.sh reads the commit from the manager environment: it tags the snapshot pre-deploy, which the nightly pruning leaves alone.
		systemctl set-environment "CAVE_PREDEPLOY_SHA=$sha"
		rc=0
		systemctl start cave-lms-backup || rc=$?
		systemctl unset-environment CAVE_PREDEPLOY_SHA
		[ "$rc" = 0 ] || exit "$rc"
	else
		echo "the commit may add a migration, but backups are not set up: no backup taken" >&2
	fi
fi

# The previous binary stays as .prev, for the rollback below.
[ -f "$bin" ] && cp "$bin" "$bin.prev"
mv "$bin.new" "$bin"
systemctl enable --quiet "$name"
systemctl restart "$name"

# Migrations and startup work run before the app listens. After a migration, or on staging (which never rolls back), wait
# up to 120 s; otherwise 15 s. Either way give up at once when the unit is no longer starting or running (crashed, exited,
# waiting to be restarted).
up_and_smoke_tested() {
	limit=15
	if [ "$backup" = yes ] || [ "$env" = staging ]; then limit=120; fi
	for _ in $(seq 1 "$limit"); do
		if curl -fs -o /dev/null "http://127.0.0.1:$port/"; then
			"$bin-smoke" "$env"
			return
		fi
		case "$(systemctl show -p SubState --value "$name")" in
		running | start*) ;;
		*)
			echo "$name stopped while starting; see: journalctl -u $name -n 50" >&2
			return 1
			;;
		esac
		sleep 1
	done
	echo "$name did not answer on 127.0.0.1:$port within ${limit}s; see: journalctl -u $name -n 50" >&2
	return 1
}
if up_and_smoke_tested; then
	if [ "$env" = staging ]; then echo "$sha" >"$records/staging-passed"; else echo "$sha" >"$records/production"; fi
	echo "$name is up and passed the smoke test"
	exit 0
fi
# A rollback cannot undo a migration, and the previous binary likely fails against the new schema, so after one the new
# binary stays for a human; the pre-deploy snapshot is in docs/runbook.md, Backups.
if [ "$env" = production ] && [ "$backup" = yes ]; then
	echo "$name failed after a migration may have run. NOT rolling back: the previous binary ($bin.prev) is likely broken against the new schema. Fix forward, or restore the pre-deploy snapshot (docs/runbook.md, Backups)." >&2
	journalctl -u "$name" -n 50 --no-pager >&2 || true
	exit 1
fi
# Staging stays broken to be looked at; production without a migration goes back to the previous binary.
if [ "$env" = production ] && [ -f "$bin.prev" ]; then
	echo "rolling production back to the previous binary" >&2
	mv "$bin.prev" "$bin"
	systemctl restart "$name"
	up_and_smoke_tested || echo "the previous binary fails the smoke test too" >&2
fi
exit 1
REMOTE
