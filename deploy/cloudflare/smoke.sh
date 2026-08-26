#!/usr/bin/env bash
# Post-deploy smoke. Asserts more than a 200: a deployment that answers but
# cannot reach its database, or whose schema is behind the binary, is not a
# successful deployment.
set -euo pipefail

base="${1:?usage: smoke.sh <base-url>}"
fail() { echo "::error::$*"; exit 1; }

echo "→ liveness"
curl -sf --max-time 10 "$base/healthz" >/dev/null || fail "healthz did not answer"

echo "→ readiness"
ready=$(curl -sf --max-time 15 "$base/readyz") || fail "readyz did not answer"
echo "$ready" | grep -q '"database":true' || fail "database unreachable: $ready"

echo "→ schema version"
version=$(echo "$ready" | sed -n 's/.*"migration_version":\([0-9]*\).*/\1/p')
[ -n "$version" ] && [ "$version" -gt 0 ] || fail "no schema version reported"
echo "  schema at version $version"

echo "→ status endpoint"
status=$(curl -sf --max-time 15 "$base/status") || fail "status did not answer"
echo "$status" | grep -q 'oldest_pending_command_s' || fail "status is incomplete: $status"

echo "→ cold start"
# The container sleeps when idle. A request after a pause is the one that
# exercises the path a real user hits first thing in the morning.
sleep 20
start=$(date +%s%3N)
curl -sf --max-time 30 "$base/healthz" >/dev/null || fail "cold request failed"
echo "  cold response in $(( $(date +%s%3N) - start )) ms"

echo "smoke passed"
