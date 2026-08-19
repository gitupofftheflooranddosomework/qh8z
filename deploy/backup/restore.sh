#!/bin/sh
set -eu

umask 077

: "${RESTIC_REPOSITORY:?RESTIC_REPOSITORY is required}"
: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=qh8z}"
: "${POSTGRES_DB:=qh8z}"
: "${RESTORE_SNAPSHOT:=latest}"

if [ "${CONFIRM_RESTORE:-}" != "$POSTGRES_DB" ]; then
  echo "refusing restore: set CONFIRM_RESTORE=$POSTGRES_DB" >&2
  exit 2
fi

read_secret() {
  path="$1"
  if [ ! -r "$path" ]; then
    echo "required secret is unreadable: $path" >&2
    exit 1
  fi
  cat "$path"
}

export PGPASSWORD="$(read_secret /run/secrets/postgres_password)"
export RESTIC_PASSWORD_FILE=/run/secrets/restic_password
export AWS_ACCESS_KEY_ID="$(read_secret /run/secrets/restic_s3_access_key)"
export AWS_SECRET_ACCESS_KEY="$(read_secret /run/secrets/restic_s3_secret_key)"

work=/backup-work/restore
rm -rf "$work"
mkdir -p "$work"

restic restore --no-cache "$RESTORE_SNAPSHOT" --target "$work" --tag qh8z-postgres

dump="$(find "$work" -type f -name 'qh8z-*.dump' | sort | tail -n 1)"
if [ -z "$dump" ]; then
  echo "no qh8z PostgreSQL dump found in restored snapshot" >&2
  exit 1
fi

pg_restore \
  --host "$POSTGRES_HOST" \
  --port "$POSTGRES_PORT" \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  "$dump"

echo "restore completed from snapshot $RESTORE_SNAPSHOT"
