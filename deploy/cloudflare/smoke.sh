#!/usr/bin/env bash
# Post-deploy smoke. Asserts more than a 200: a deployment that answers but
# cannot reach its database, or whose schema is behind the binary, is not a
# successful deployment.
#
# The first request after a deploy is not a health check, it is a cold start.
# Cloudflare has to schedule the container, pull the image and wait for the Go
# binary to bind its port, and on a container application's first ever deploy it
# provisions the application too. A single ten-second attempt measures none of
# that -- it just fails, and the rollback that follows then destroys the version
# that would have become healthy a few seconds later.
#
# So liveness is polled to a deadline. Everything after it is asserted once,
# because by then the container is genuinely up and a failure is a real failure.
set -euo pipefail

base="${1:?usage: smoke.sh <base-url>}"
deadline_s="${SMOKE_DEADLINE_S:-240}"

fail() { echo "::error::$*"; exit 1; }

echo "→ liveness (waiting up to ${deadline_s}s for the container to start)"
started=$(date +%s)
attempt=0
until curl -sf --max-time 15 "$base/healthz" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  elapsed=$(( $(date +%s) - started ))
  if [ "$elapsed" -ge "$deadline_s" ]; then
    body=$(curl -s --max-time 15 "$base/healthz" 2>&1 | head -c 300)
    fail "healthz did not answer within ${deadline_s}s. Last response: ${body:-<none>}"
  fi
  # A container that is still scheduling returns quickly, so pace the polling
  # rather than hammering it.
  [ $((attempt % 4)) -eq 1 ] && echo "  still starting after ${elapsed}s"
  sleep 5
done
echo "  live after $(( $(date +%s) - started ))s"

echo "→ readiness"
ready=$(curl -sf --max-time 20 "$base/readyz") || fail "readyz did not answer"
echo "$ready" | grep -q '"database":true' || fail "database unreachable: $ready"

echo "→ schema version"
version=$(echo "$ready" | sed -n 's/.*"migration_version":\([0-9]*\).*/\1/p')
[ -n "$version" ] && [ "$version" -gt 0 ] || fail "no schema version reported"
echo "  schema at version $version"

echo "→ status endpoint"
status=$(curl -sf --max-time 20 "$base/status") || fail "status did not answer"
echo "$status" | grep -q 'oldest_pending_command_s' || fail "status is incomplete: $status"

# The Mini App must be served by the container, not by the Worker's fallback.
# Checking only the status code would have passed while /app/ answered with the
# placeholder body -- which is exactly what happened the first time it shipped.
echo "→ mini app"
page=$(curl -sf --max-time 20 "$base/app/") || fail "the mini app did not answer"
echo "$page" | grep -q 'telegram-web-app.js' || \
  fail "$base/app/ did not serve the form; got: $(echo "$page" | head -c 80)"

echo "→ budget form"
budget=$(curl -sf --max-time 20 "$base/app/?screen=budget") || fail "the budget screen did not answer"
echo "$budget" | grep -q 'budget-form' || fail "the budget screen did not serve its form"

echo "→ mini app rejects an unsigned call"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST \
  "$base/app/api/loans" -H 'Content-Type: application/json' -d '{}')
[ "$code" = "401" ] || fail "an unsigned loan POST answered $code, want 401"

code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST \
  "$base/app/api/budget" -H 'Content-Type: application/json' -d '{}')
[ "$code" = "401" ] || fail "an unsigned budget POST answered $code, want 401"

echo "→ cold start"
# The container sleeps when idle, so a request after a pause exercises the path
# a real user hits first thing in the morning. This is a MEASUREMENT, not an
# assertion: how long a wake takes is information, and a slow one is not a
# broken deploy.
#
# It was fatal once, and a slow wake then rolled back a release that was working
# -- the second time a check in this file destroyed something correct. A single
# attempt is also the wrong instrument for a wake: the first request is the one
# that triggers it, so failing on that request measures nothing but timing.
sleep 20
start=$(date +%s%3N)
woke=""
for attempt in 1 2 3; do
  if curl -sf --max-time 45 "$base/healthz" >/dev/null 2>&1; then
    woke="yes"
    break
  fi
  echo "  attempt $attempt did not answer; retrying"
  sleep 5
done
elapsed=$(( $(date +%s%3N) - start ))
if [ -n "$woke" ]; then
  echo "  cold response in ${elapsed} ms"
else
  # Liveness and readiness both passed above, so the deployment is serving.
  # Say the wake was slow and let the release stand.
  echo "::warning::the container did not answer a cold request within ${elapsed} ms across 3 attempts"
fi

echo "smoke passed"
