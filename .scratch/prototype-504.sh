#!/usr/bin/env bash
# PROTOTYPE (#504, throwaway): runs Cave LMS on :8504 against a throwaway Postgres, so
# http://localhost:8504/prototype/my-folder?variant=A|B|C works (add &empty=1 for the empty Folder).
# Postgres and the storage dir live and die with this script. Usage: .scratch/prototype-504.sh
set -euo pipefail
cd "$(dirname "$0")/.."

bindir=$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -n 1 || true)
[ -z "$bindir" ] && bindir=$(pg_config --bindir 2>/dev/null || true)
[ -x "$bindir/initdb" ] || { echo "Postgres server binaries not found (need initdb and pg_ctl)" >&2; exit 1; }

dir=$(mktemp -d)
trap '"$bindir/pg_ctl" -D "$dir/data" -m immediate stop >/dev/null 2>&1 || true; rm -rf "$dir"' EXIT

"$bindir/initdb" -D "$dir/data" -U cave -A trust -E UTF8 >/dev/null
"$bindir/pg_ctl" -D "$dir/data" -o "-k $dir -c listen_addresses='' -c fsync=off" -l "$dir/pg.log" -w start >/dev/null
"$bindir/createdb" -h "$dir" -U cave cave_lms_proto

mkdir -p "$dir/storage"
export DATABASE_URL="postgres://cave@/cave_lms_proto?host=$dir&sslmode=disable"
export CAVE_ENV=development
export PUBLIC_BASE_URL=http://localhost:8504
export PAGES_BASE_URL=https://pages.cavelms.com
export EMAIL_FROM="CAVELMS <no-reply@localhost.test>"
export STORAGE_DIR="$dir/storage"
export LISTEN_ADDR=:8504

echo "Prototype #504: http://localhost:8504/prototype/my-folder?variant=A  (B, C; &empty=1)"
# Not exec, so the trap above still stops Postgres when the server exits.
go run ./cmd/cave-lms
