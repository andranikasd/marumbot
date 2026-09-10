#!/bin/sh
# Run from any directory. stdout contains only the completed dump path.
set -eu
umask 077
base=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
env_file=${MARUM_COMPOSE_ENV:-"$base/.env"}
dest=${MARUM_BACKUP_DIR:-"$base/backups"}
retention_days=${MARUM_BACKUP_RETENTION_DAYS:-14}
case "$retention_days" in
  ''|*[!0-9]*|0*) echo "MARUM_BACKUP_RETENTION_DAYS must be a positive integer" >&2; exit 1 ;;
esac
mkdir -p "$dest"
# A unique directory avoids concurrent jobs overwriting each other's dumps.
job=$(mktemp -d "$dest/backup-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")
trap 'rm -f "$job/database.dump.partial"' EXIT
trap 'exit 1' HUP INT TERM
docker compose --env-file "$env_file" -f "$base/compose.yml" exec -T postgres \
  pg_dump -U marum_owner -d marum --format=custom --no-owner --no-acl \
  > "$job/database.dump.partial"
# Verify the archive directory before promoting it. A restore drill is still required.
docker compose --env-file "$env_file" -f "$base/compose.yml" exec -T postgres \
  pg_restore --list < "$job/database.dump.partial" > /dev/null
mv "$job/database.dump.partial" "$job/database.dump"
(cd "$job" && sha256sum database.dump > database.dump.sha256)
# Only age out completed, intact dumps created by this script, after this
# replacement is verified. Never traverse symlinks or recursively remove a
# directory: unexpected contents must survive for operator inspection.
for old in "$dest"/backup-*; do
  [ -d "$old" ] && [ ! -L "$old" ] || continue
  [ "$old" != "$job" ] || continue
  name=${old##*/}
  printf '%s\n' "$name" | grep -Eq '^backup-[0-9]{8}T[0-9]{6}Z-[[:alnum:]]{6}$' || continue
  find "$old" -maxdepth 0 -mtime "+$retention_days" -print | grep -q . || continue
  [ -f "$old/database.dump" ] && [ ! -L "$old/database.dump" ] || continue
  [ -f "$old/database.dump.sha256" ] && [ ! -L "$old/database.dump.sha256" ] || continue
  # Compare the entire expected record, so a modified checksum file cannot
  # direct checksum verification outside this directory.
  expected=$(cat "$old/database.dump.sha256")
  actual=$(cd "$old" && sha256sum database.dump)
  [ "$expected" = "$actual" ] || continue
  # Extra files mean this is no longer a directory owned solely by this job.
  entries=$(find "$old" -mindepth 1 -maxdepth 1 -printf x)
  [ "$entries" = xx ] || continue
  rm -- "$old/database.dump" "$old/database.dump.sha256"
  rmdir -- "$old"
done
printf '%s\n' "$job/database.dump"
