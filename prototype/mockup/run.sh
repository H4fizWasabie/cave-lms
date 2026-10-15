#!/bin/sh
# One command to view the prototype. Usage: ./run.sh [port]  (default 4173)
cd "$(dirname "$0")" || exit 1
echo "Open http://localhost:${1:-4173}/  (variants: ?variant=A | B | C, ← → keys cycle)"
exec python3 -m http.server "${1:-4173}"
