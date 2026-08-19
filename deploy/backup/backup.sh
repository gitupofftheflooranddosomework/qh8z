#!/bin/sh
set -u

umask 077

: "${RESTIC_REPOSITORY:?RESTIC_REPOSITORY is required}"
: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=qh8z}"
: "${POSTGRES_DB:=qh8z}"
: "${BACKUP_INTERVAL_SECONDS:=21600}"

read_secret() {
  path="$1"
  if [ ! -r "$path" ]; then
    echo "required secret is unreadable: $path" >&2
    return 1
  fi
  cat "$path"
}

export PGPASSWORD="$(read_secret /run/secrets/postgres_password)"
export RESTIC_PASSWORD_FILE=/run/secrets/restic_password
export AWS_ACCESS_KEY_ID="$(read_secret /run/secrets/restic_s3_access_key)"
export AWS_SECRET_ACCESS_KEY="$(read_secret /run/secrets/restic_s3_secret_key)"

mkdir -p /backup-work /backup-status

ensure_repository() {
  if restic snapshots --no-cache >/dev/null 2>&1; then
    return 0
  fi
  echo "restic repository is not initialized; attempting initialization" >&2
  restic init
}

backup_once() {
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  dump="/backup-work/qh8z-${timestamp}.dump"
  rm -f /backup-work/*.dump

  echo "creating PostgreSQL backup at ${timestamp}"
  if ! pg_dump \
    --host "$POSTGRES_HOST" \
    --port "$POSTGRES_PORT" \
    --username "$POSTGRES_USER" \
    --dbname "$POSTGRES_DB" \
    --format custom \
    --compress 6 \
    --no-owner \
    --no-privileges \
    --file "$dump"; then
    rm -f "$dump"
    return 1
  fi

  if ! restic backup --no-cache --tag qh8z-postgres "$dump"; then
    rm -f "$dump"
    return 1
  fi
  rm -f "$dump"

  restic forget --no-cache --prune \
    --keep-hourly 24 \
    --keep-daily 14 \
    --keep-weekly 8 \
    --keep-monthly 12

  date -u +%s > /backup-status/last_success_epoch
  echo "backup completed successfully"
}

if ! ensure_repository; then
  echo "restic repository initialization failed" >&2
  if [ "${BACKUP_ONCE:-0}" = "1" ]; then
    exit 1
  fi
fi

if [ "${BACKUP_ONCE:-0}" = "1" ]; then
  backup_once
  exit $?
fi

while :; do
  if ! backup_once; then
    echo "backup attempt failed" >&2
  fi
  sleep "$BACKUP_INTERVAL_SECONDS"
done
