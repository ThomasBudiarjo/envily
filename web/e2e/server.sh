#!/bin/sh
# Starts a throwaway Envily server for Playwright with a fresh database and
# master key. The server replaces this shell (exec) so Playwright's stop
# signal reaches it directly; the data directory is reset on every start.
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
port=${E2E_PORT:-18091}
dir="${TMPDIR:-/tmp}/envily-e2e-$port"
rm -rf "$dir"
mkdir -p "$dir"
ENVILY_DB_DSN="file:$dir/envily.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)" \
ENVILY_PUBLIC_URL="http://localhost:$port" \
ENVILY_ADDR="127.0.0.1:$port" \
ENVILY_MASTER_KEY="$("$root/bin/envily" keygen)" \
exec "$root/bin/envily" server
