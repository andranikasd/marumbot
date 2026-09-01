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

base="${1:?usage: smoke.sh <base-url> <expected-version>}"
expected_version="${2:?usage: smoke.sh <base-url> <expected-version>}"
deadline_s="${SMOKE_DEADLINE_S:-240}"

fail() { echo "::error::$*"; exit 1; }

# get retries a GET until it succeeds or the attempts run out, printing the body.
#
# Syncing secrets creates a new Worker version, which restarts the container, so
# a request issued immediately after can arrive mid-restart. Every assertion
# here has to tolerate that: a single-shot curl against a restarting container
# measures timing, not correctness -- which is the same mistake the cold start
# check made, and it cost a working deploy both times.
get() {
  local url="$1" attempt
  for attempt in 1 2 3 4 5; do
    if body=$(curl -sf --max-time 20 "$url" 2>/dev/null); then
      printf '%s' "$body"
      return 0
    fi
    sleep 4
  done
  return 1
}

# status retries until it sees the expected code, for the same reason.
status() {
  local url="$1" want="$2" method="${3:-GET}" attempt code
  for attempt in 1 2 3 4 5; do
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X "$method" \
      "$url" -H 'Content-Type: application/json' -d '{}' 2>/dev/null)
    [ "$code" = "$want" ] && { printf '%s' "$code"; return 0; }
    sleep 4
  done
  printf '%s' "$code"
}

echo "→ liveness (waiting up to ${deadline_s}s for the container to start)"
started=$(date +%s)
attempt=0
until health=$(curl -sf --max-time 15 "$base/healthz" 2>/dev/null) && \
      echo "$health" | grep -Fq "\"version\":\"$expected_version\""; do
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
echo "  application version $expected_version"

echo "→ readiness"
ready=$(get "$base/readyz") || fail "readyz did not answer"
echo "$ready" | grep -q '"database":true' || fail "database unreachable: $ready"

echo "→ schema version"
version=$(echo "$ready" | sed -n 's/.*"migration_version":\([0-9]*\).*/\1/p')
[ -n "$version" ] && [ "$version" -gt 0 ] || fail "no schema version reported"
echo "  schema at version $version"

echo "→ status endpoint"
state=$(get "$base/status") || fail "status did not answer"
echo "$state" | grep -q 'oldest_pending_command_s' || fail "status is incomplete: $state"

# The Mini App must be served by the container, not by the Worker's fallback.
# Checking only the status code would have passed while /app/ answered with the
# placeholder body -- which is exactly what happened the first time it shipped.
echo "→ mini app"
page=$(get "$base/app/") || fail "the mini app did not answer"
echo "$page" | grep -q 'telegram-web-app.js' || \
  fail "$base/app/ did not serve the form; got: $(echo "$page" | head -c 80)"

# The screens live in ES modules now; the shell is deliberately empty. So the
# shell is checked for its versioned entry script, and the modules for the
# markup they carry -- the budget form and the plan timeline.
echo "→ app shell and modules"
# The old container can still be answering while the new one rolls out, so
# the marker is polled to a deadline rather than asserted against whichever
# instance happened to answer first.
shell_ok=""
for attempt in 1 2 3 4 5 6 7 8; do
  if echo "$page" | grep -Fq "a/$expected_version/js/main.js"; then shell_ok=yes; break; fi
  sleep 8
  page=$(get "$base/app/") || true
done
[ -n "$shell_ok" ] || fail "the shell does not version its entry script after rollout"
budget=$(get "$base/app/js/screens/budget.js") || fail "the budget module did not answer"
echo "$budget" | grep -q 'budget-form' || fail "the budget module does not carry its form"
planmod=$(get "$base/app/js/screens/plan.js") || fail "the plan module did not answer"
echo "$planmod" | grep -q 'plan-goals' || fail "the plan module does not carry its screen"

echo "→ mini app rejects an unsigned call"
code=$(status "$base/app/api/loans" 401 POST)
[ "$code" = "401" ] || fail "an unsigned loan POST answered $code, want 401"

code=$(status "$base/app/api/budget" 401 POST)
[ "$code" = "401" ] || fail "an unsigned budget POST answered $code, want 401"

# What Telegram shows in the chat is not what the container serves; it is what
# the container last told Telegram. The global menu button carries the version
# in its URL precisely so the webview cannot reuse last week's cached app, and
# it is published from the container at startup. A container that starts
# without its Mini App URL publishes nothing, passes every check above, and
# leaves the chat opening the old URL -- which is exactly what shipped as
# v1.0.0: the deploy went green and Telegram kept serving the previous build.
#
# Publication is asynchronous and starts after the listener is up, so it is
# polled rather than asserted once. Skipped without a token: the check needs
# the bot's own credentials, and a self-hosted smoke may not have them.
if [ -n "${MARUM_BOT_TOKEN:-}" ]; then
  echo "→ telegram menu button"
  want="v=$(printf '%s' "$expected_version" | sed 's/[^A-Za-z0-9._-]/./g')"
  button=""
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
    button=$(curl -s --max-time 20 \
      "https://api.telegram.org/bot${MARUM_BOT_TOKEN}/getChatMenuButton" 2>/dev/null || true)
    if printf '%s' "$button" | grep -Eq "\"url\":\"[^\"]*[?&]${want}"; then
      echo "  menu button opens the app at $expected_version"
      break
    fi
    button=""
    sleep 5
  done
  [ -n "$button" ] || fail "the chat menu button does not point at $expected_version; the container is running without its Mini App URL, or publishing the menus failed"
fi

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
