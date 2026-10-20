#!/usr/bin/env bash
# Emails ALERT_EMAIL through Resend when cave-lms-backup fails. Installed to /usr/local/bin/cave-lms-backup-failed.
set -euo pipefail
set -a; . /etc/cave-lms/env; . /etc/cave-lms/backup.env; set +a

log=$(journalctl -u cave-lms-backup -n 40 --no-pager -o cat)
body=$(jq -n --arg from "$EMAIL_FROM" --arg to "$ALERT_EMAIL" --arg host "$(hostname)" --arg log "$log" \
	'{from: $from, to: [$to], subject: "Cave LMS backup FAILED on \($host)", text: "The Cave LMS backup failed. The last log lines:\n\n\($log)\n\nRun it again by hand: systemctl start cave-lms-backup"}')
curl -fsS -m 20 -o /dev/null https://api.resend.com/emails \
	-H "Authorization: Bearer $RESEND_API_KEY" -H "Content-Type: application/json" -d "$body"
