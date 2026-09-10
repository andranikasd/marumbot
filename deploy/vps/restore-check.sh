#!/bin/sh
# Restores only into a NEW isolated disposable container, never a live database.
set -eu
if [ "$#" -ne 1 ]; then
  echo "usage: sh restore-check.sh /absolute/path/database.dump" >&2
  exit 2
fi
base=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$base/../.." && pwd)
dump=$1
test -r "$dump"
test -r "$dump.sha256"
(cd -- "$(dirname -- "$dump")" && sha256sum -c "$(basename -- "$dump").sha256" >&2)
name="marum-restore-check-$$"
container=$(docker run -d --name "$name" --network none \
  -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=marum_owner \
  -e POSTGRES_DB=marum "${MARUM_POSTGRES_IMAGE:-postgres:17-alpine}")
trap 'docker rm -fv "$container" >/dev/null' EXIT
trap 'exit 1' HUP INT TERM
attempt=0
until docker exec "$container" pg_isready -U marum_owner -d marum >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    echo "restore database did not start" >&2
    exit 1
  fi
  sleep 1
done
docker exec -i -e MARUM_DB_PASSWORD=restore-check-only "$container" psql -v ON_ERROR_STOP=1 -U marum_owner -d marum < "$repo/queries/provisioning/vps-role.sql"
docker exec -i "$container" pg_restore -U marum_owner -d marum \
  --exit-on-error --single-transaction --no-owner --no-acl < "$dump"
docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U marum_owner -d marum < "$repo/queries/provisioning/restrict-ledgers.sql"
docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U marum_owner -d marum < "$repo/queries/tests/ledger_guards.sql"
if [ -n "${MARUM_VERIFY_IMAGE:-}" ]; then
  : "${MARUM_IDENTITY_KEY:?set the original identity key for application verification}"
  : "${MARUM_ERASURE_JOURNAL_DIR:?provide the latest authoritative erasure journal}"
  export MARUM_IDENTITY_KEY
  docker run --rm --network "container:$container" --read-only --cap-drop ALL \
    --security-opt no-new-privileges --memory "${MARUM_APP_MEMORY:-6g}" --cpus "${MARUM_APP_CPUS:-4}" \
    --mount "type=bind,src=$MARUM_ERASURE_JOURNAL_DIR,dst=/journal" \
    -e MARUM_ENV=prod -e MARUM_MODE=polling -e MARUM_ERASURE_JOURNAL_DIR=/journal \
    -e GOMEMLIMIT="${MARUM_GOMEMLIMIT:-4GiB}" -e GOMAXPROCS="${MARUM_GOMAXPROCS:-4}" \
    -e MARUM_IDENTITY_KEY -e MARUM_BOT_TOKEN=restore-verification-no-delivery \
    -e MARUM_SERVICE_TOKEN=restore-verification-no-listener \
    -e 'MARUM_DATABASE_URL=host=127.0.0.1 user=marum_app dbname=marum sslmode=disable' \
    "$MARUM_VERIFY_IMAGE" -verify-restore
  echo "Application identity-key and original-plan replay verification passed."
fi
echo "Archive checksum, restore, restricted grants and ledger guards passed."
