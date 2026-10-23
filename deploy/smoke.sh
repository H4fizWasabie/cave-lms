#!/usr/bin/env bash
# Smoke test of a deployed Cave LMS over its public HTTPS address: a few signed-out pages must answer as expected.
# deploy/deploy.sh installs it and runs it after every deploy (ADR 0011).
# Installed per site as /usr/local/bin/cave-lms-smoke and cave-lms-staging-smoke.
# By hand: cave-lms-smoke production, cave-lms-staging-smoke staging
# No set -e: every check runs and reports, and the script fails at the end if any did.
set -uo pipefail

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
case "${1:-}" in
production) base=https://cavelms.com ;;
staging)
	base=https://staging.cavelms.com
	login=/etc/cave-lms/staging-login-password
	[ -r "$login" ] || { echo "cannot read $login, the staging login" >&2; exit 2; }
	# The staging login goes in a curl config file, not the command line other users can see.
	(umask 077; printf 'user = "cave:%s"\n' "$(cat "$login")" >"$work/curlrc")
	;;
*)
	echo "usage: cave-lms-smoke production|staging" >&2
	exit 2
	;;
esac
# Connect straight to the address Caddy binds: the VPS's own DNS (Tailscale's resolver) sometimes fails to look up
# its own domain, and that is no fault of the site. Caddy, TLS and the app are still all tested.
echo "resolve = \"${base#https://}:443:169.58.63.110\"" >>"$work/curlrc"

failed=0
# check METHOD PATH STATUS [TEXT]: the answer must have STATUS and, when given, contain TEXT in its headers or body
# (ignoring case).
check() {
	local code
	: >"$work/head"
	: >"$work/body"
	code=$(curl -s -m 20 -K "$work/curlrc" -X "$1" -D "$work/head" -o "$work/body" -w '%{http_code}' "$base$2")
	if [ "$code" != "$3" ] || { [ -n "${4:-}" ] && ! cat "$work/head" "$work/body" | grep -qiF -- "$4"; }; then
		echo "FAIL $1 $2: got $code, want $3${4:+ containing \"$4\"}" >&2
		failed=1
	else
		echo "ok   $1 $2"
	fi
}

check GET / 200 '<a class="brand" href="/">'
stylesheet=$(grep -oE '/static/[^"]+\.css\?v=[0-9a-f]+' "$work/body" | head -n 1)
check GET "${stylesheet:-/static/missing.css}" 200
for page in /login /courses /gallery /programmes /plans /certificates; do
	check GET "$page" 200
done
check GET /admin 303 'location: /login'
check POST /cavetext/bcl/smoke-test-wrong-key 404

if [ "$failed" = 1 ]; then
	echo "smoke test FAILED on $base" >&2
	exit 1
fi
echo "smoke test passed on $base"
