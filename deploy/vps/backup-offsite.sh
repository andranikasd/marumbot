#!/bin/sh
# RESTIC_REPOSITORY and RESTIC_PASSWORD_FILE belong in a root-readable env file.
set -eu
umask 077
base=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
: "${RESTIC_REPOSITORY:?configure an off-host restic repository}"
: "${RESTIC_PASSWORD_FILE:?configure the restic encryption password file}"
command -v restic > /dev/null
command -v flock > /dev/null
# Hold one host-local lock across dump, upload and pruning. Never unlink it:
# replacing its inode would let another invocation bypass an active lock.
exec 9>>"${MARUM_BACKUP_LOCK:-/run/lock/marum-backup.lock}"
if ! flock -n 9; then
  echo "another offsite backup is running (or the backup lock is unavailable)" >&2
  exit 1
fi
daily=${MARUM_RESTIC_KEEP_DAILY:-14}
weekly=${MARUM_RESTIC_KEEP_WEEKLY:-8}
monthly=${MARUM_RESTIC_KEEP_MONTHLY:-12}
for keep in "$daily" "$weekly" "$monthly"; do
  case "$keep" in
    ''|*[!0-9]*|0*) echo "MARUM_RESTIC_KEEP_* values must be positive integers" >&2; exit 1 ;;
  esac
done
backup_host=$(hostname)
# Resolve the journal actually mounted by this deployment. Compose's .env and
# systemd's EnvironmentFile are different sources; never guess a default here.
env_file=${MARUM_COMPOSE_ENV:-"$base/.env"}
container=$(docker compose --env-file "$env_file" -f "$base/compose.yml" ps --all --quiet marum)
case "$container" in
  ''|*[!a-f0-9]*) echo "expected exactly one application container" >&2; exit 1 ;;
esac
journal=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/marum/erasures"}}{{if eq .Type "bind"}}{{.Source}}{{end}}{{end}}{{end}}' "$container")
case "$journal" in
  /*) ;;
  *) echo "application erasure journal bind mount is missing" >&2; exit 1 ;;
esac
if [ ! -d "$journal" ] || [ -L "$journal" ]; then
  echo "application erasure journal directory is unavailable" >&2
  exit 1
fi
dump=$(sh "$base/backup.sh")
restic backup --host "$backup_host" --tag marum-postgres "$dump" "$dump.sha256" "$journal"
# Dump paths are unique per run. Group by host and tag rather than path so
# retention applies across runs; filters exclude other hosts and workloads.
# set -e ensures a failed upload never proceeds to retention or pruning.
restic forget --host "$backup_host" --tag marum-postgres --group-by host,tags \
  --keep-daily "$daily" --keep-weekly "$weekly" --keep-monthly "$monthly" --prune
