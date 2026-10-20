#!/usr/bin/env bash
# Nightly backup of Cave LMS: a dump of the cave database plus the uploads directory, written with restic
# to a repository on the VPS and to one in Cloudflare R2. Installed to /usr/local/bin/cave-lms-backup by
# deploy/deploy.sh and run by cave-lms-backup.timer. See docs/deploy.md, Backups.
set -euo pipefail

# backup.env holds R2_ENDPOINT, R2_BUCKET, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, ALERT_EMAIL and,
# optionally, HEALTHCHECK_URL.
set -a; . /etc/cave-lms/backup.env; set +a
export RESTIC_PASSWORD_FILE=/etc/cave-lms/restic-password

staging=/var/backups/cave-lms/staging
local_repo=/var/backups/cave-lms/restic
r2_repo="s3:$R2_ENDPOINT/$R2_BUCKET"
install -d -m 700 /var/backups/cave-lms "$staging"

runuser -u postgres -- pg_dump --format=custom cave > "$staging/cave.dump.tmp"
mv "$staging/cave.dump.tmp" "$staging/cave.dump"

for repo in "$local_repo" "$r2_repo"; do
	restic -r "$repo" cat config >/dev/null 2>&1 || restic -r "$repo" init
	restic -r "$repo" backup --quiet --tag nightly "$staging/cave.dump" /var/lib/cave-lms/uploads
	restic -r "$repo" forget --quiet --tag nightly --keep-daily 7 --keep-weekly 4 --keep-monthly 6 --prune
	restic -r "$repo" check --quiet
done

if [ -n "${HEALTHCHECK_URL:-}" ]; then
	curl -fsS -m 10 --retry 3 -o /dev/null "$HEALTHCHECK_URL"
fi
echo "backup done"
