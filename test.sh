#!/usr/bin/env bash
# Runs the Go tests against a throwaway Postgres that lives and dies with this script.
# Usage: ./test.sh [go test args]   (default: ./...)
set -euo pipefail

bindir=$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -n 1 || true)
[ -z "$bindir" ] && bindir=$(pg_config --bindir 2>/dev/null || true)
[ -x "$bindir/initdb" ] || { echo "Postgres server binaries not found (need initdb and pg_ctl)" >&2; exit 1; }

dir=$(mktemp -d)
trap '"$bindir/pg_ctl" -D "$dir/data" -m immediate stop >/dev/null 2>&1 || true; rm -rf "$dir"' EXIT

"$bindir/initdb" -D "$dir/data" -U cave -A trust -E UTF8 >/dev/null
# Unix socket only, inside the temp dir: nothing is exposed on the network.
"$bindir/pg_ctl" -D "$dir/data" -o "-k $dir -c listen_addresses='' -c max_connections=500 -c fsync=off" -l "$dir/pg.log" -w start >/dev/null
"$bindir/createdb" -h "$dir" -U cave cave_lms_test

export TEST_DATABASE_URL="postgres://cave@/cave_lms_test?host=$dir&sslmode=disable"
# Tests copy a migrated template database and run in parallel; eight at once peaks under 1 GB and the app package takes
# about a minute. A later -parallel or -timeout overrides these.
go test -count=1 -parallel 8 "${@:-./...}"
